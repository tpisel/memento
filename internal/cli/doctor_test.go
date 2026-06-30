package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

// These tests cover the ADR-0032 liveness nodes retrofitted onto the check engine:
// each failing case asserts the exact wire-value of the canonical token it emits, and
// the no-vault case proves grant-fresh SKIPS (blocked-by vault-discoverable) rather than
// reporting a dishonest ok.

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
const benignScriptBody = "#!/usr/bin/env bash\necho noop\n"

// hookSpec is one (event, matcher, command) settings entry.
type hookSpec struct {
	event   string
	matcher string
	command string
}

// writeClaudeSettingsFile writes a named .claude settings file wiring the given hooks.
func writeClaudeSettingsFile(t *testing.T, repoRoot, name string, specs ...hookSpec) {
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
	writeDoctorScript(t, repoRoot, ".claude/"+name, string(data))
}

func writeClaudeSettings(t *testing.T, repoRoot string, specs ...hookSpec) {
	t.Helper()
	writeClaudeSettingsFile(t, repoRoot, "settings.json", specs...)
}

func writeClaudeLocalSettings(t *testing.T, repoRoot string, specs ...hookSpec) {
	t.Helper()
	writeClaudeSettingsFile(t, repoRoot, "settings.local.json", specs...)
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

// codexRepo builds a repo whose codex gate is fully wired and live.
func codexRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
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

// realBin sets MEMENTO_BIN to a binary exec.LookPath resolves on every platform — the
// test binary itself (absolute, with the Windows .exe extension already on it).
func realBin(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	t.Setenv("MEMENTO_BIN", exe)
}

// stubGateSchema makes binary-schema-compatible see a gate binary reporting schema,
// restoring the real exec-based probe when the test ends. The in-process live tests use
// it so traversing the node does not exec the test binary itself — which, invoked with
// `schema`, would re-enter TestMain and re-run the whole suite. The real exec+parse path
// is covered separately against a freshly built binary (TestGateSchemaProbeRealBinary).
func stubGateSchema(t *testing.T, schema int) {
	t.Helper()
	orig := gateSchemaProbe
	gateSchemaProbe = func(string) (int, bool) { return schema, true }
	t.Cleanup(func() { gateSchemaProbe = orig })
}

// --- finding assertions --------------------------------------------------

func sole(t *testing.T, fs []finding) finding {
	t.Helper()
	if len(fs) != 1 {
		t.Fatalf("want exactly one finding, got %d: %+v", len(fs), fs)
	}
	return fs[0]
}

func assertOK(t *testing.T, fs []finding) {
	t.Helper()
	f := sole(t, fs)
	if f.severity != sevOK || f.token != "" {
		t.Fatalf("want ok finding (empty token), got %+v", f)
	}
}

func findToken(t *testing.T, fs []finding, token string) finding {
	t.Helper()
	for _, f := range fs {
		if f.token == token {
			return f
		}
	}
	t.Fatalf("no finding with token %q in %+v", token, fs)
	return finding{}
}

// --- live-fire -----------------------------------------------------------

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

func TestLiveFireFindingsOK(t *testing.T) {
	assertOK(t, liveFireFindings())
}

// --- gate-committed-config ----------------------------------------------

func TestGateCommittedFindingVerdicts(t *testing.T) {
	good := &resolvedHook{command: "guard.sh", exists: true, executable: true, content: preWriteScriptBody}
	const fullMatcher = "Write|Edit|MultiEdit|Bash"
	cases := []struct {
		name      string
		scan      gateScan
		wantToken string
		wantSev   severity
	}{
		{"no gate", gateScan{family: "claude", covers: claudeMatcherCovers}, tokGateMissing, sevError},
		{"unresolved command", gateScan{family: "claude", gate: &resolvedHook{command: "x", exists: false}, gateMatcher: fullMatcher, covers: claudeMatcherCovers}, tokGateUnresolved, sevError},
		{"not executable", gateScan{family: "claude", gate: &resolvedHook{command: "x", exists: true, executable: false}, gateMatcher: fullMatcher, covers: claudeMatcherCovers}, tokGateUnresolved, sevError},
		{"matcher misses file tools", gateScan{family: "claude", gate: good, gateMatcher: "Bash", covers: claudeMatcherCovers}, tokGateMissing, sevError},
		{"matcher partial (no Bash)", gateScan{family: "claude", gate: good, gateMatcher: "Write|Edit|MultiEdit", covers: claudeMatcherCovers}, tokGateMatcherPartial, sevWarning},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := gateCommittedFinding(c.scan)
			if f.token != c.wantToken || f.severity != c.wantSev {
				t.Fatalf("got token=%q sev=%v, want token=%q sev=%v", f.token, f.severity, c.wantToken, c.wantSev)
			}
		})
	}
	if f := gateCommittedFinding(gateScan{family: "claude", gate: good, gateMatcher: fullMatcher, covers: claudeMatcherCovers}); f.severity != sevOK || f.token != "" {
		t.Fatalf("full matcher want ok finding, got %+v", f)
	}
}

func TestGateCommittedFindingsLive(t *testing.T) {
	assertOK(t, gateCommittedFindings(liveClaudeRepo(t)))
}

func TestGateCommittedFindingsNoFamily(t *testing.T) {
	f := sole(t, gateCommittedFindings(t.TempDir()))
	if f.token != tokGateMissing || f.severity != sevError {
		t.Fatalf("no wired family: got %+v, want gate-missing error", f)
	}
}

// --- gate-effective-local ------------------------------------------------

func TestGateEffectiveLocalOverride(t *testing.T) {
	repoRoot := liveClaudeRepo(t)
	noop := writeDoctorScript(t, repoRoot, ".claude/local-noop.sh", benignScriptBody)
	// settings.local.json replaces the committed PreToolUse gate with a non-gate command.
	writeClaudeLocalSettings(t, repoRoot, hookSpec{"PreToolUse", "Write|Edit|MultiEdit|Bash", noop})

	f := findToken(t, gateEffectiveLocalFindings(repoRoot), tokGateLocallyOverridden)
	if f.severity != sevError {
		t.Fatalf("gate-locally-overridden severity = %v, want error", f.severity)
	}
	// The committed-config node must stay ok: it reads settings.json only.
	assertOK(t, gateCommittedFindings(repoRoot))
}

func TestGateEffectiveLocalNoLocalOK(t *testing.T) {
	assertOK(t, gateEffectiveLocalFindings(liveClaudeRepo(t)))
}

func TestGateEffectiveLocalCodexNoLayer(t *testing.T) {
	// codex has no machine-local layer, so the node is trivially ok.
	assertOK(t, gateEffectiveLocalFindings(codexRepo(t)))
}

// --- postwrite-hook-live -------------------------------------------------

func TestPostwriteFindingsLive(t *testing.T) {
	assertOK(t, postwriteFindings(liveClaudeRepo(t)))
}

func TestPostwriteFindingsMissing(t *testing.T) {
	repoRoot := t.TempDir()
	pre := writeDoctorScript(t, repoRoot, ".claude/memento-pre-write-vault-guard.sh", preWriteScriptBody)
	writeClaudeSettings(t, repoRoot, hookSpec{"PreToolUse", "Write|Edit|MultiEdit|Bash", pre}) // no PostToolUse
	f := sole(t, postwriteFindings(repoRoot))
	if f.token != tokPostwriteHookMissing || f.severity != sevWarning {
		t.Fatalf("missing post hook: got %+v, want postwrite-hook-missing warning", f)
	}
}

// --- no-legacy-broad-deny ------------------------------------------------

func TestLegacyFindingsFails(t *testing.T) {
	repoRoot := t.TempDir()
	legacy := writeDoctorScript(t, repoRoot, ".claude/memento-pre-write-vault-guard.sh", legacyGuardBody)
	writeClaudeSettings(t, repoRoot, hookSpec{"PreToolUse", "Write|Edit|MultiEdit|Bash", legacy})

	f := findToken(t, legacyFindings(repoRoot), tokLegacyBroadDenyWired)
	if f.severity != sevError {
		t.Fatalf("legacy guard severity = %v, want error", f.severity)
	}
	// The legacy script lacks check-write, so it is not also counted as a live gate.
	g := sole(t, gateCommittedFindings(repoRoot))
	if g.token != tokGateMissing {
		t.Fatalf("with only a legacy guard, committed gate token = %q, want gate-missing", g.token)
	}
}

func TestLegacyFindingsClean(t *testing.T) {
	assertOK(t, legacyFindings(liveClaudeRepo(t)))
}

func TestIsLegacyBroadDeny(t *testing.T) {
	if !isLegacyBroadDeny(legacyGuardBody) {
		t.Fatalf("legacy guard body not recognised as broad-deny")
	}
	if isLegacyBroadDeny(preWriteScriptBody) {
		t.Fatalf("ADR-0031 check-write gate misclassified as legacy broad-deny")
	}
}

// --- binary-on-path ------------------------------------------------------

func TestBinaryOnPathMissing(t *testing.T) {
	t.Setenv("MEMENTO_BIN", filepath.Join(t.TempDir(), "does-not-exist-memento"))
	f := sole(t, binaryOnPathFindings())
	if f.token != tokBinaryNotOnPath || f.severity != sevError {
		t.Fatalf("missing binary: got %+v, want binary-not-on-path error", f)
	}
}

func TestBinaryOnPathLive(t *testing.T) {
	realBin(t)
	assertOK(t, binaryOnPathFindings())
}

// --- binary-schema-compatible --------------------------------------------

// TestSchemaNodesDiverge is the split's reason for being: the two schema nodes read
// distinct data sources and so disagree. With a manifest at the binary's own schema,
// manifest-schema-readable (keyed on doctor's compiled-in version) stays ok, while a
// gate binary reporting an older schema makes binary-schema-compatible emit
// binary-schema-too-old.
func TestSchemaNodesDiverge(t *testing.T) {
	realBin(t) // binary-on-path resolves; the probe below is stubbed, so nothing execs
	stubGateSchema(t, manifest.CurrentSchemaVersion-1)
	v := doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion)

	assertOK(t, manifestSchemaReadableFindings(v))

	f := findToken(t, binarySchemaCompatFindings(v, nil), tokBinarySchemaTooOld)
	if f.severity != sevError {
		t.Fatalf("binary-schema-too-old severity = %v, want error", f.severity)
	}
}

func TestBinarySchemaCompatLive(t *testing.T) {
	realBin(t)
	stubGateSchema(t, manifest.CurrentSchemaVersion)
	assertOK(t, binarySchemaCompatFindings(doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion), nil))
}

// A gate binary that does not report a schema (old binary, exec error) is not judged on
// what cannot be determined.
func TestBinarySchemaCompatProbeUnknownOK(t *testing.T) {
	realBin(t)
	orig := gateSchemaProbe
	gateSchemaProbe = func(string) (int, bool) { return 0, false }
	t.Cleanup(func() { gateSchemaProbe = orig })
	assertOK(t, binarySchemaCompatFindings(doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion), nil))
}

// With no resolved vault there is no manifest to be incompatible with; the node is ok
// (vault-discoverable owns the no-vault error).
func TestBinarySchemaCompatNoVaultOK(t *testing.T) {
	assertOK(t, binarySchemaCompatFindings(vault.Vault{}, vault.ErrVaultNotFound))
}

// TestGateSchemaProbeRealBinary exercises the real exec+parse path: a freshly built
// memento binary answers `schema` with its compiled-in CurrentSchemaVersion.
func TestGateSchemaProbeRealBinary(t *testing.T) {
	schema, ok := gateSchemaProbe(mementoBinary(t))
	if !ok || schema != manifest.CurrentSchemaVersion {
		t.Fatalf("gateSchemaProbe(real) = (%d, %v), want (%d, true)", schema, ok, manifest.CurrentSchemaVersion)
	}
}

// --- manifest-schema-readable --------------------------------------------

// TestManifestSchemaReadableTooNew re-homes the dropped v1 TestBinaryReachableSchemaTooNew:
// a manifest newer than this binary's schema cannot be decoded.
func TestManifestSchemaReadableTooNew(t *testing.T) {
	v := doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion+1)
	f := findToken(t, manifestSchemaReadableFindings(v), tokManifestSchemaUnread)
	if f.severity != sevError {
		t.Fatalf("manifest-schema-unreadable severity = %v, want error", f.severity)
	}
}

func TestManifestSchemaReadableOK(t *testing.T) {
	assertOK(t, manifestSchemaReadableFindings(doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion)))
}

// A malformed on-disk manifest is undecodable, so the node reports it unreadable.
func TestManifestSchemaReadableMalformed(t *testing.T) {
	v := doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion)
	if err := os.WriteFile(v.ManifestPath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := findToken(t, manifestSchemaReadableFindings(v), tokManifestSchemaUnread)
	if f.severity != sevError {
		t.Fatalf("malformed manifest severity = %v, want error", f.severity)
	}
}

// --- grant-fresh ---------------------------------------------------------

func TestGrantStaleWarns(t *testing.T) {
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
	f := findToken(t, grantFreshFindings(v), tokGrantStale)
	if f.severity != sevWarning || !strings.Contains(f.detail, "frozen.md") {
		t.Fatalf("stale grant: got %+v, want grant-stale warning mentioning frozen.md", f)
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
	assertOK(t, grantFreshFindings(v))
}

func TestNoGrantsOK(t *testing.T) {
	assertOK(t, grantFreshFindings(doctorVault(t, t.TempDir(), manifest.CurrentSchemaVersion)))
}

// --- vault-discoverable --------------------------------------------------

func TestVaultDiscoverableOK(t *testing.T) {
	assertOK(t, vaultDiscoverableFindings(nil))
}

func TestVaultDiscoverableAbsent(t *testing.T) {
	f := sole(t, vaultDiscoverableFindings(vault.ErrVaultNotFound))
	if f.token != tokVaultAbsent || f.severity != sevError {
		t.Fatalf("absent vault: got %+v, want vault-absent error", f)
	}
}

func TestVaultDiscoverableAmbiguous(t *testing.T) {
	f := sole(t, vaultDiscoverableFindings(vault.ErrMultipleVaults))
	if f.token != tokVaultAmbiguous || f.severity != sevError {
		t.Fatalf("ambiguous vault: got %+v, want vault-ambiguous error", f)
	}
}

// --- full DAG ------------------------------------------------------------

// TestGrantFreshSkipsWithNoVault is the dishonest-OK fix: with no vault, grant-fresh
// must SKIP blocked-by vault-discoverable, not report ok.
func TestGrantFreshSkipsWithNoVault(t *testing.T) {
	outcomes, err := runChecks(doctorNodes(t.TempDir(), vault.Vault{}, vault.ErrVaultNotFound))
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}
	gf := outcomeFor(t, outcomes, nodeGrantFresh)
	if !gf.skipped || gf.blockedBy != nodeVaultDiscoverable {
		t.Fatalf("grant-fresh outcome = %+v, want skipped blocked-by %s", gf, nodeVaultDiscoverable)
	}
	if gf.passed() {
		t.Fatal("a skipped grant-fresh must not count as passed (not green)")
	}
}

func TestDoctorDAGLive(t *testing.T) {
	repoRoot := liveClaudeRepo(t)
	realBin(t)
	stubGateSchema(t, manifest.CurrentSchemaVersion)
	v := doctorVault(t, repoRoot, manifest.CurrentSchemaVersion)
	outcomes, err := runChecks(doctorNodes(repoRoot, v, nil))
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}
	for _, o := range outcomes {
		if o.skipped || !o.passed() {
			t.Fatalf("node %q should have run and passed, got skipped=%v findings=%+v", o.node.name, o.skipped, o.findings)
		}
	}
	if code := computeExitCode(outcomes, ctxSession, false); code != 0 {
		t.Fatalf("live DAG exit = %d, want 0", code)
	}
}

// --- runDoctor (headline + exit) ----------------------------------------

func TestRunDoctorExitLive(t *testing.T) {
	t.Setenv("CI", "")
	repoRoot := liveClaudeRepo(t)
	realBin(t)
	stubGateSchema(t, manifest.CurrentSchemaVersion)
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
	t.Setenv("CI", "")
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

func TestRunDoctorCodexCaveat(t *testing.T) {
	t.Setenv("CI", "")
	repoRoot := codexRepo(t)
	realBin(t)
	stubGateSchema(t, manifest.CurrentSchemaVersion)
	doctorVault(t, repoRoot, manifest.CurrentSchemaVersion)
	chdirCLI(t, repoRoot)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("codex doctor exit = %d, want 0; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.HasPrefix(out, "vault write enforcement: LIVE") || !strings.Contains(out, "apply_patch") {
		t.Fatalf("codex headline = %q, want LIVE with apply_patch caveat", out)
	}
}

func TestRunDoctorRejectsArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor", "extra"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("doctor extra-arg exit = %d, want 2", code)
	}
}
