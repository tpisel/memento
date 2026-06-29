package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

// --- fixtures ------------------------------------------------------------

// writeDoctorScript writes an executable hook script and returns its absolute path.
func writeDoctorScript(t *testing.T, repoRoot, rel, content string) string {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return path
}

const preWriteScriptBody = "#!/usr/bin/env bash\n\"$memento_bin\" check-write\n"
const postWriteScriptBody = "#!/usr/bin/env bash\n\"$memento_bin\" compile\n"
const legacyGuardBody = "#!/usr/bin/env bash\n# routes vault writes through memento write\npermission_decision deny\n"

// writeClaudeSettings writes a .claude/settings.json wiring the given hook entries.
type hookSpec struct {
	event   string
	matcher string
	command string
}

func writeClaudeSettings(t *testing.T, repoRoot string, specs ...hookSpec) {
	t.Helper()
	hooks := map[string][]map[string]any{}
	for _, s := range specs {
		hooks[s.event] = append(hooks[s.event], map[string]any{
			"matcher": s.matcher,
			"hooks":   []map[string]any{{"type": "command", "command": s.command}},
		})
	}
	data, err := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	writeDoctorScript(t, repoRoot, ".claude/settings.json", string(data))
}

// liveClaudeRepo builds a repo whose Claude gate is fully wired and live.
func liveClaudeRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	pre := writeDoctorScript(t, repoRoot, ".claude/memento-pre-write-vault-guard.sh", preWriteScriptBody)
	post := writeDoctorScript(t, repoRoot, ".claude/memento-post-write-compile.sh", postWriteScriptBody)
	writeClaudeSettings(t, repoRoot,
		hookSpec{"PreToolUse", "Write|Edit|MultiEdit|Bash", pre},
		hookSpec{"PostToolUse", "Write|Edit|MultiEdit|Bash", post},
	)
	return repoRoot
}

// doctorVault creates a vault with a manifest at the given schema version.
func doctorVault(t *testing.T, repoRoot string, schemaVersion int) vault.Vault {
	t.Helper()
	root := filepath.Join(repoRoot, "memory")
	marker := filepath.Join(root, vault.MarkerDirName)
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	v := vault.Vault{Root: root, MarkerDir: marker, ManifestPath: filepath.Join(marker, vault.ManifestFileName)}
	manifestJSON := []byte(`{"schema_version":` + strconv.Itoa(schemaVersion) + `,"entries":[]}`)
	if err := os.WriteFile(v.ManifestPath, manifestJSON, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return v
}

// realBin sets MEMENTO_BIN to a binary exec.LookPath resolves on every platform —
// the test binary itself (absolute, with the Windows .exe extension already on it).
func realBin(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	t.Setenv("MEMENTO_BIN", exe)
}

func findResult(t *testing.T, results []checkResult, nameSubstr string) checkResult {
	t.Helper()
	for _, r := range results {
		if strings.Contains(r.name, nameSubstr) {
			return r
		}
	}
	t.Fatalf("no check result matching %q in %+v", nameSubstr, results)
	return checkResult{}
}

// --- live-fire self-test -------------------------------------------------

func TestLiveFireReadOnlyProbeDenies(t *testing.T) {
	denied, reasonCode, err := liveFireReadOnlyProbe()
	if err != nil {
		t.Fatalf("liveFireReadOnlyProbe error: %v", err)
	}
	if !denied {
		t.Fatalf("probe denied = false, want true (read-only overwrite must be denied)")
	}
	if reasonCode != enforce.ReasonReadOnly {
		t.Fatalf("probe reasonCode = %q, want %q", reasonCode, enforce.ReasonReadOnly)
	}
}

func TestLiveFireCheckLeavesNoResidue(t *testing.T) {
	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "memento-doctor-probe-*"))
	if _, _, err := liveFireReadOnlyProbe(); err != nil {
		t.Fatalf("probe error: %v", err)
	}
	after, _ := filepath.Glob(filepath.Join(os.TempDir(), "memento-doctor-probe-*"))
	if len(after) > len(before) {
		t.Fatalf("probe left a temp vault behind: before=%d after=%d", len(before), len(after))
	}
}

func TestLiveFireCheckReportsOK(t *testing.T) {
	r := liveFireCheck()
	if r.status != statusOK {
		t.Fatalf("liveFireCheck status = %v, want ok; detail = %q", r.status, r.detail)
	}
}

// --- gate check (#1) -----------------------------------------------------

func TestGateCheckLive(t *testing.T) {
	repoRoot := liveClaudeRepo(t)
	results := gateChecks(repoRoot, "claude", readClaudeHooks(repoRoot), claudeMatcherCovers)
	if r := findResult(t, results, "PreToolUse gate"); r.status != statusOK {
		t.Fatalf("gate status = %v, want ok; detail = %q", r.status, r.detail)
	}
	if r := findResult(t, results, "PostToolUse"); r.status != statusOK {
		t.Fatalf("post hook status = %v, want ok; detail = %q", r.status, r.detail)
	}
	if r := findResult(t, results, "legacy"); r.status != statusOK {
		t.Fatalf("legacy status = %v, want ok; detail = %q", r.status, r.detail)
	}
}

func TestGateCheckMissing(t *testing.T) {
	repoRoot := t.TempDir()
	writeClaudeSettings(t, repoRoot) // hooks present but empty
	results := gateChecks(repoRoot, "claude", readClaudeHooks(repoRoot), claudeMatcherCovers)
	if r := findResult(t, results, "PreToolUse gate"); r.status != statusFail {
		t.Fatalf("gate status = %v, want fail", r.status)
	}
}

func TestGateCheckCommandUnresolved(t *testing.T) {
	repoRoot := t.TempDir()
	missing := filepath.Join(repoRoot, ".claude", "memento-pre-write-vault-guard.sh")
	writeClaudeSettings(t, repoRoot, hookSpec{"PreToolUse", "Write|Edit|MultiEdit|Bash", missing})
	results := gateChecks(repoRoot, "claude", readClaudeHooks(repoRoot), claudeMatcherCovers)
	r := findResult(t, results, "PreToolUse gate")
	if r.status != statusFail {
		t.Fatalf("gate status = %v, want fail (command does not resolve)", r.status)
	}
	if !strings.Contains(r.reason, "resolve") {
		t.Fatalf("gate reason = %q, want it to mention resolve", r.reason)
	}
}

func TestGateCheckNotExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no execute bit on Windows")
	}
	repoRoot := t.TempDir()
	pre := filepath.Join(repoRoot, ".claude", "memento-pre-write-vault-guard.sh")
	if err := os.MkdirAll(filepath.Dir(pre), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pre, []byte(preWriteScriptBody), 0o644); err != nil { // no exec bit
		t.Fatal(err)
	}
	writeClaudeSettings(t, repoRoot, hookSpec{"PreToolUse", "Write|Edit|MultiEdit|Bash", pre})
	results := gateChecks(repoRoot, "claude", readClaudeHooks(repoRoot), claudeMatcherCovers)
	if r := findResult(t, results, "PreToolUse gate"); r.status != statusFail || !strings.Contains(r.reason, "executable") {
		t.Fatalf("gate = %+v, want fail mentioning executable", r)
	}
}

func TestGateCheckMatcherMissesWriteTools(t *testing.T) {
	repoRoot := t.TempDir()
	pre := writeDoctorScript(t, repoRoot, ".claude/memento-pre-write-vault-guard.sh", preWriteScriptBody)
	writeClaudeSettings(t, repoRoot, hookSpec{"PreToolUse", "Bash", pre})
	results := gateChecks(repoRoot, "claude", readClaudeHooks(repoRoot), claudeMatcherCovers)
	if r := findResult(t, results, "PreToolUse gate"); r.status != statusFail {
		t.Fatalf("gate status = %v, want fail (matcher misses write tools)", r.status)
	}
}

func TestGateCheckMatcherPartialWarns(t *testing.T) {
	repoRoot := t.TempDir()
	pre := writeDoctorScript(t, repoRoot, ".claude/memento-pre-write-vault-guard.sh", preWriteScriptBody)
	writeClaudeSettings(t, repoRoot, hookSpec{"PreToolUse", "Write|Edit|MultiEdit", pre}) // no Bash
	results := gateChecks(repoRoot, "claude", readClaudeHooks(repoRoot), claudeMatcherCovers)
	if r := findResult(t, results, "PreToolUse gate"); r.status != statusWarn {
		t.Fatalf("gate status = %v, want warn (Bash uncovered, file tools covered)", r.status)
	}
}

// --- legacy broad-deny guard (#4) ---------------------------------------

func TestLegacyBroadDenyGuardFails(t *testing.T) {
	repoRoot := t.TempDir()
	legacy := writeDoctorScript(t, repoRoot, ".claude/memento-pre-write-vault-guard.sh", legacyGuardBody)
	writeClaudeSettings(t, repoRoot, hookSpec{"PreToolUse", "Write|Edit|MultiEdit|Bash", legacy})
	results := gateChecks(repoRoot, "claude", readClaudeHooks(repoRoot), claudeMatcherCovers)
	r := findResult(t, results, "legacy")
	if r.status != statusFail {
		t.Fatalf("legacy guard status = %v, want fail", r.status)
	}
	// The legacy script lacks check-write, so it is not also counted as a live gate.
	if g := findResult(t, results, "PreToolUse gate"); g.status != statusFail {
		t.Fatalf("with only a legacy guard, gate status = %v, want fail", g.status)
	}
}

func TestIsLegacyBroadDeny(t *testing.T) {
	if !isLegacyBroadDeny(legacyGuardBody) {
		t.Fatalf("legacy guard body not recognised as broad-deny")
	}
	if isLegacyBroadDeny(preWriteScriptBody) {
		t.Fatalf("ADR-0031 check-write gate misclassified as legacy broad-deny")
	}
}

// --- binary reachable + schema (#3) -------------------------------------

func TestBinaryReachableMissing(t *testing.T) {
	t.Setenv("MEMENTO_BIN", filepath.Join(t.TempDir(), "does-not-exist-memento"))
	v := doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion)
	r := binaryReachableCheck(v, nil)
	if r.status != statusFail {
		t.Fatalf("status = %v, want fail (binary not reachable)", r.status)
	}
}

func TestBinaryReachableSchemaTooNew(t *testing.T) {
	realBin(t)
	v := doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion+1)
	r := binaryReachableCheck(v, nil)
	if r.status != statusFail || !strings.Contains(r.reason, "schema") {
		t.Fatalf("status/reason = %+v, want fail mentioning schema", r)
	}
}

func TestBinaryReachableLive(t *testing.T) {
	realBin(t)
	v := doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion)
	r := binaryReachableCheck(v, nil)
	if r.status != statusOK {
		t.Fatalf("status = %v, want ok; detail = %q", r.status, r.detail)
	}
}

// --- stale unlock grants (#5) -------------------------------------------

func TestStaleGrantWarns(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(repoRoot, "memory")
	marker := filepath.Join(root, vault.MarkerDirName)
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatal(err)
	}
	v := vault.Vault{Root: root, MarkerDir: marker, ManifestPath: filepath.Join(marker, vault.ManifestFileName)}
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n")
	initCLIGit(t, root)
	commitCLIGit(t, root) // frozen.md is committed and clean -> a grant for it is stale
	if err := enforce.AddGrant(v, "frozen.md", "test", time.Now()); err != nil {
		t.Fatalf("add grant: %v", err)
	}
	r := staleGrantCheck(v, nil)
	if r.status != statusWarn || !strings.Contains(r.detail, "frozen.md") {
		t.Fatalf("stale grant check = %+v, want warn mentioning frozen.md", r)
	}
}

func TestActiveGrantWithEditNotStale(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(repoRoot, "memory")
	marker := filepath.Join(root, vault.MarkerDirName)
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatal(err)
	}
	v := vault.Vault{Root: root, MarkerDir: marker, ManifestPath: filepath.Join(marker, vault.ManifestFileName)}
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n")
	initCLIGit(t, root)
	commitCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nEdited.\n") // uncommitted edit
	if err := enforce.AddGrant(v, "frozen.md", "test", time.Now()); err != nil {
		t.Fatalf("add grant: %v", err)
	}
	r := staleGrantCheck(v, nil)
	if r.status != statusOK {
		t.Fatalf("grant with uncommitted edit = %+v, want ok (not stale)", r)
	}
}

func TestNoGrantsOK(t *testing.T) {
	v := doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion)
	r := staleGrantCheck(v, nil)
	if r.status != statusOK {
		t.Fatalf("no grants = %+v, want ok", r)
	}
}

// --- full report + exit codes -------------------------------------------

func TestRunDoctorChecksLive(t *testing.T) {
	repoRoot := liveClaudeRepo(t)
	realBin(t)
	v := doctorVault(t, repoRoot, manifest.CurrentSchemaVersion)
	results, caveat := runDoctorChecks(repoRoot, v, nil)
	if !isLive(results) {
		t.Fatalf("expected LIVE, got results %+v", results)
	}
	if caveat != "" {
		t.Fatalf("claude-only repo should have no caveat, got %q", caveat)
	}
}

func TestRunDoctorChecksOffNoGate(t *testing.T) {
	repoRoot := t.TempDir()
	realBin(t)
	v := doctorVault(t, repoRoot, manifest.CurrentSchemaVersion)
	results, _ := runDoctorChecks(repoRoot, v, nil)
	if isLive(results) {
		t.Fatalf("expected OFF with no gate, got LIVE: %+v", results)
	}
	if got := firstFailReason(results); !strings.Contains(got, "no memento gate") {
		t.Fatalf("first fail reason = %q, want it to mention no memento gate", got)
	}
}

func TestRunDoctorChecksCodexCaveat(t *testing.T) {
	repoRoot := t.TempDir()
	realBin(t)
	pre := writeDoctorScript(t, repoRoot, ".codex/memento-pre-write-vault-guard.sh", preWriteScriptBody)
	post := writeDoctorScript(t, repoRoot, ".codex/memento-post-write-compile.sh", postWriteScriptBody)
	config := strings.Join([]string{
		"# memento:start",
		"[[hooks.PreToolUse]]",
		`matcher = "apply_patch"`,
		"[[hooks.PreToolUse.hooks]]",
		`type = "command"`,
		`command = "` + pre + `"`,
		"[[hooks.PostToolUse]]",
		`matcher = "apply_patch"`,
		"[[hooks.PostToolUse.hooks]]",
		`type = "command"`,
		`command = "` + post + `"`,
		"# memento:end",
	}, "\n")
	writeDoctorScript(t, repoRoot, ".codex/config.toml", config)
	v := doctorVault(t, repoRoot, manifest.CurrentSchemaVersion)
	results, caveat := runDoctorChecks(repoRoot, v, nil)
	if !isLive(results) {
		t.Fatalf("expected LIVE codex, got %+v", results)
	}
	if !strings.Contains(caveat, "apply_patch") {
		t.Fatalf("codex caveat = %q, want it to mention apply_patch", caveat)
	}
}

func TestRunDoctorExitLive(t *testing.T) {
	repoRoot := liveClaudeRepo(t)
	realBin(t)
	doctorVault(t, repoRoot, manifest.CurrentSchemaVersion)
	chdirCLI(t, repoRoot)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit = %d, want 0; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "vault write enforcement: LIVE") {
		t.Fatalf("headline = %q, want LIVE prefix", stdout.String())
	}
}

func TestRunDoctorExitOff(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("MEMENTO_BIN", filepath.Join(repoRoot, "no-memento"))
	chdirCLI(t, repoRoot)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doctor exit = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "OFF (") {
		t.Fatalf("headline = %q, want OFF reason", stdout.String())
	}
}

func TestRunDoctorRejectsArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor", "extra"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("doctor extra-arg exit = %d, want 2", code)
	}
}
