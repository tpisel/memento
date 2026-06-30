package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

// writeClaudeLocalDisableAllHooks writes a .claude/settings.local.json that sets
// disableAllHooks:true — the one machine-local vector that switches off the committed gate.
func writeClaudeLocalDisableAllHooks(t *testing.T, repoRoot string) {
	t.Helper()
	data, err := json.MarshalIndent(map[string]any{"disableAllHooks": true}, "", "  ")
	if err != nil {
		t.Fatalf("marshal local settings: %v", err)
	}
	writeDoctorScript(t, repoRoot, ".claude/settings.local.json", string(data))
}

// liveClaudeRepo builds a repo whose Claude gate is fully wired and live: a git tree with
// the memento pre-commit anchor installed at the default .git/hooks location, so every
// doctor node (git-repo and precommit-anchor-live included) runs and passes.
func liveClaudeRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	pre := writeDoctorScript(t, repoRoot, ".claude/memento-pre-write-vault-guard.sh", preWriteScriptBody)
	post := writeDoctorScript(t, repoRoot, ".claude/memento-post-write-compile.sh", postWriteScriptBody)
	writeClaudeSettings(t, repoRoot,
		hookSpec{"PreToolUse", "Write|Edit|MultiEdit|Bash", pre},
		hookSpec{"PostToolUse", "Write|Edit|MultiEdit|Bash", post},
	)
	initCLIGit(t, repoRoot)
	writeMementoPreCommitHook(t, repoRoot)
	return repoRoot
}

// mementoPreCommitBody is a pre-commit hook carrying memento's sentinel-bracketed step,
// shaped like what init writes.
const mementoPreCommitBody = "#!/bin/sh\nset -eu\n\n# memento:start\nif command -v memento >/dev/null 2>&1; then\nmemento compile\nmemento clear-grants\nfi\n# memento:end\n"

// writeMementoPreCommitHook installs the memento anchor at the default .git/hooks location.
func writeMementoPreCommitHook(t *testing.T, repoRoot string) {
	t.Helper()
	writeDoctorScript(t, repoRoot, ".git/hooks/pre-commit", mementoPreCommitBody)
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
	// Claude merges hooks additively, so a local PreToolUse hook ADDS alongside the
	// committed gate — it cannot replace or remove it. The local layer disables the gate
	// only via disableAllHooks:true.
	t.Run("local hook is additive, gate stays live", func(t *testing.T) {
		repoRoot := liveClaudeRepo(t)
		noop := writeDoctorScript(t, repoRoot, ".claude/local-noop.sh", benignScriptBody)
		// An unrelated local PreToolUse hook: it runs alongside the committed gate, it
		// does not displace it, so the effective gate stays live.
		writeClaudeLocalSettings(t, repoRoot, hookSpec{"PreToolUse", "Write|Edit|MultiEdit|Bash", noop})
		assertOK(t, gateEffectiveLocalFindings(repoRoot))
	})

	t.Run("disableAllHooks:true disables the gate", func(t *testing.T) {
		repoRoot := liveClaudeRepo(t)
		writeClaudeLocalDisableAllHooks(t, repoRoot)

		f := findToken(t, gateEffectiveLocalFindings(repoRoot), tokGateLocallyOverridden)
		if f.severity != sevError {
			t.Fatalf("gate-locally-overridden severity = %v, want error", f.severity)
		}
		// The committed-config node must stay ok: it reads settings.json only.
		assertOK(t, gateCommittedFindings(repoRoot))
	})
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

// --- manifest-present & manifest-fresh -----------------------------------

// freshVault builds a vault with one note and a freshly compiled, on-disk manifest, so a
// manifest-fresh check over it reports ok until a note is edited out of band.
func freshVault(t *testing.T) vault.Vault {
	t.Helper()
	root := t.TempDir()
	marker := filepath.Join(root, vault.MarkerDirName)
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	v := vault.Vault{Root: root, MarkerDir: marker, ManifestPath: filepath.Join(marker, vault.ManifestFileName)}
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("---\ntitle: Note\n---\n# Note\n\nBody.\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	if err := manifest.Write(v); err != nil {
		t.Fatalf("compile manifest: %v", err)
	}
	return v
}

func TestManifestPresentOK(t *testing.T) {
	assertOK(t, manifestPresentFindings(freshVault(t)))
}

func TestManifestPresentMissing(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, vault.MarkerDirName)
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatal(err)
	}
	v := vault.Vault{Root: root, MarkerDir: marker, ManifestPath: filepath.Join(marker, vault.ManifestFileName)}
	f := findToken(t, manifestPresentFindings(v), tokManifestNotFound)
	if f.severity != sevWarning {
		t.Fatalf("manifest-not-found severity = %v, want warning", f.severity)
	}
}

// A freshly compiled-and-committed vault is fresh.
func TestManifestFreshOK(t *testing.T) {
	assertOK(t, manifestFreshFindings(freshVault(t)))
}

// Editing a note without recompiling makes the on-disk manifest stale: the authoritative
// in-buffer recompile diverges from the artifact.
func TestManifestFreshStaleAfterEdit(t *testing.T) {
	v := freshVault(t)
	if err := os.WriteFile(filepath.Join(v.Root, "note.md"), []byte("---\ntitle: Note\n---\n# Note\n\nEdited body, never recompiled.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := findToken(t, manifestFreshFindings(v), tokManifestStale)
	if f.severity != sevWarning {
		t.Fatalf("manifest-stale severity = %v, want warning", f.severity)
	}
}

// A re-serialized but semantically equivalent on-disk manifest must NOT trip
// manifest-stale: the diff runs over the canonical decoded projection, not raw bytes, so
// whitespace and serialization differences are invisible to it.
func TestManifestFreshIgnoresReserialization(t *testing.T) {
	v := freshVault(t)
	m, err := manifest.Load(v)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// Compact re-encoding differs byte-for-byte from memento's indented canonical Marshal
	// while decoding to the identical model; a raw-byte diff would call this stale.
	scrambled, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Equal(scrambled, mustReadFile(t, v.ManifestPath)) {
		t.Fatal("re-encoding did not change the bytes; the test proves nothing")
	}
	if err := os.WriteFile(v.ManifestPath, scrambled, 0o644); err != nil {
		t.Fatal(err)
	}
	assertOK(t, manifestFreshFindings(v))
}

// An internal Marshal failure while canonicalising the recompile is undiagnosable, not
// evidence of staleness: it must yield a plain warning WITHOUT the manifest-stale token,
// matching the sibling Compile/Load failure branches. Mislabeling it manifest-stale would
// tell the user a note changed when the corpus is actually fresh.
func TestManifestFreshMarshalErrorIsUndiagnosable(t *testing.T) {
	v := freshVault(t)
	orig := manifestProjectionMarshal
	manifestProjectionMarshal = func(manifest.Manifest) ([]byte, error) {
		return nil, errors.New("injected marshal failure")
	}
	t.Cleanup(func() { manifestProjectionMarshal = orig })

	findings := manifestFreshFindings(v)
	for _, f := range findings {
		if f.token == tokManifestStale {
			t.Fatal("a Marshal failure was mislabeled manifest-stale; it must be an undiagnosable warning")
		}
	}
	if len(findings) != 1 || findings[0].severity != sevWarning || findings[0].token != "" {
		t.Fatalf("want one untokened warning, got %+v", findings)
	}
}

// manifest-fresh is side-effect-free: it recompiles to a buffer and must not touch the
// on-disk manifest. Writing would race the PostToolUse compile hook — a diagnostic must
// not mutate what it diagnoses.
func TestManifestFreshWritesNothing(t *testing.T) {
	v := freshVault(t)
	// Make it stale so the check does its most work (recompile, diff, and emit a finding).
	if err := os.WriteFile(filepath.Join(v.Root, "note.md"), []byte("---\ntitle: Note\n---\n# Note\n\nEdited.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(v.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeBytes := mustReadFile(t, v.ManifestPath)

	_ = manifestFreshFindings(v)

	after, err := os.Stat(v.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("manifest mtime changed %v -> %v; doctor must not write", before.ModTime(), after.ModTime())
	}
	if !bytes.Equal(beforeBytes, mustReadFile(t, v.ManifestPath)) {
		t.Fatal("manifest bytes changed; doctor must not write the manifest")
	}
}

// With a resolvable vault but no compiled manifest, manifest-present emits
// manifest-not-found and manifest-fresh SKIPS through the manifest chain rather than
// recompiling against an artifact that is not there.
func TestManifestFreshSkipsWithNoManifest(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, vault.MarkerDirName)
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatal(err)
	}
	v := vault.Vault{Root: root, MarkerDir: marker, ManifestPath: filepath.Join(marker, vault.ManifestFileName)}
	outcomes, err := runChecks(doctorNodes(t.TempDir(), v, nil, false))
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}
	mp := outcomeFor(t, outcomes, nodeManifestPresent)
	if f := findToken(t, mp.findings, tokManifestNotFound); f.severity != sevWarning {
		t.Fatalf("manifest-present finding = %+v, want manifest-not-found warning", f)
	}
	mf := outcomeFor(t, outcomes, nodeManifestFresh)
	if !mf.skipped || mf.blockedBy != nodeManifestSchemaRead {
		t.Fatalf("manifest-fresh outcome = %+v, want skipped blocked-by %s", mf, nodeManifestSchemaRead)
	}
	if mf.passed() {
		t.Fatal("a skipped manifest-fresh must not count as passed")
	}
}

// In the session cadence (the orient hook's --session), the corpus-scaling checks are
// DEFERRED: manifest-fresh's full in-buffer recompile and the live-fire temp-vault probe
// never run on the hot path. The proof is behavioral — a vault made stale would trip
// manifest-stale on a recompile, so the absence of that finding under --session is direct
// evidence the recompile was skipped; the same DAG in the full cadence still trips it.
func TestDoctorSessionDefersCorpusRecompile(t *testing.T) {
	v := freshVault(t)
	// Edit a note without recompiling: the on-disk manifest is now stale, so any recompile
	// that ran would emit manifest-stale.
	if err := os.WriteFile(filepath.Join(v.Root, "note.md"), []byte("---\ntitle: Note\n---\n# Note\n\nEdited, never recompiled.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repoRoot := t.TempDir()

	// Session cadence: manifest-fresh and live-fire are deferred (skipped without running).
	hot, err := runChecks(doctorNodes(repoRoot, v, nil, true))
	if err != nil {
		t.Fatalf("runChecks (session): %v", err)
	}
	for _, name := range []string{nodeManifestFresh, nodeLiveFire} {
		o := outcomeFor(t, hot, name)
		if !o.skipped || !o.deferred {
			t.Fatalf("%s under --session = %+v, want deferred skip", name, o)
		}
	}
	for _, f := range outcomeFor(t, hot, nodeManifestFresh).findings {
		if f.token == tokManifestStale {
			t.Fatal("manifest-fresh emitted manifest-stale under --session; the recompile was NOT skipped")
		}
	}

	// Full cadence: the same stale vault trips manifest-fresh, proving deferral — not a
	// fresh manifest — is what silenced it above.
	full, err := runChecks(doctorNodes(repoRoot, v, nil, false))
	if err != nil {
		t.Fatalf("runChecks (full): %v", err)
	}
	fmf := outcomeFor(t, full, nodeManifestFresh)
	if fmf.skipped {
		t.Fatalf("manifest-fresh should run in the full cadence, got %+v", fmf)
	}
	findToken(t, fmf.findings, tokManifestStale)
	if o := outcomeFor(t, full, nodeLiveFire); o.skipped {
		t.Fatalf("live-fire should run in the full cadence, got %+v", o)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
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

// --- headline / exit consistency (memento-tbu.7) -------------------------

// headlineFor renders just the headline line for the given outcomes and context.
func headlineFor(outcomes []checkOutcome, ctx assertContext) string {
	var buf bytes.Buffer
	printDoctorHeadline(&buf, outcomes, "", ctx)
	return strings.TrimSpace(buf.String())
}

// A session-only liveness error (a node assertable only in ctxSession) must not flip
// the headline to a hard OFF in ci context, where it does not gate the exit code: the
// loud OFF would contradict exit 0. In session context the same error both gates and
// reads OFF. The body still carries the detail either way.
func TestHeadlineMatchesExitAcrossContexts(t *testing.T) {
	sessionOnly := checkOutcome{
		node: checkNode{
			name:         nodeBinaryOnPath,
			class:        classLiveness,
			assertableIn: ctxSession,
		},
		findings: []finding{{token: "binary-missing", severity: sevError, detail: "memento not on PATH"}},
	}
	outcomes := []checkOutcome{sessionOnly}

	// ci: the session-only error does not gate, so exit is 0 and the headline must not
	// assert a hard OFF.
	if code := computeExitCode(outcomes, ctxCI, false); code != 0 {
		t.Fatalf("ci exit = %d, want 0 (session-only liveness error does not gate in ci)", code)
	}
	if h := headlineFor(outcomes, ctxCI); strings.Contains(h, "OFF") {
		t.Fatalf("ci headline = %q, want no hard OFF contradicting exit 0", h)
	}

	// session: the same error both gates and reads OFF — headline and exit agree.
	if code := computeExitCode(outcomes, ctxSession, false); code != 1 {
		t.Fatalf("session exit = %d, want 1", code)
	}
	if h := headlineFor(outcomes, ctxSession); !strings.Contains(h, "OFF") {
		t.Fatalf("session headline = %q, want OFF", h)
	}
}

// A liveness error on a node assertable in any context gates and reads OFF in both
// session and ci — the fix must not suppress genuine context-assertable failures.
func TestHeadlineOFFWhenContextAssertable(t *testing.T) {
	anyCtx := checkOutcome{
		node: checkNode{
			name:         nodePostwriteHook,
			class:        classLiveness,
			assertableIn: ctxAny,
		},
		findings: []finding{{token: "postwrite-missing", severity: sevError, detail: "no postwrite hook"}},
	}
	outcomes := []checkOutcome{anyCtx}

	for _, ctx := range []assertContext{ctxSession, ctxCI} {
		if code := computeExitCode(outcomes, ctx, false); code != 1 {
			t.Fatalf("exit in ctx %d = %d, want 1", ctx, code)
		}
		if h := headlineFor(outcomes, ctx); !strings.Contains(h, "OFF") {
			t.Fatalf("headline in ctx %d = %q, want OFF", ctx, h)
		}
	}
}

// --- git-repo & precommit-anchor-live ------------------------------------

func TestGitRepoFindingsPresent(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	assertOK(t, gitRepoFindings(repoRoot))
}

func TestGitRepoFindingsAbsent(t *testing.T) {
	fs := gitRepoFindings(t.TempDir())
	f := sole(t, fs)
	if f.severity != sevNudge {
		t.Fatalf("no-git git-repo severity = %v, want nudge", f.severity)
	}
	// A nudge fails the precondition (not sevOK) so dependents skip, yet never gates.
	o := checkOutcome{findings: fs}
	if o.passed() {
		t.Fatal("a no-git git-repo node must not pass (so precommit-anchor skips)")
	}
}

// The installed anchor at the default .git/hooks location, with no core.hooksPath
// redirect, is the hook git runs — reachable, so live.
func TestPrecommitAnchorDefaultLive(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	writeMementoPreCommitHook(t, repoRoot)
	assertOK(t, precommitAnchorFindings(repoRoot))
}

// memento's step folded in among other steps reads as live, not as drift.
func TestPrecommitAnchorComposedLive(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	composed := "#!/bin/sh\nset -eu\nnpm test\nmemento compile\nmemento clear-grants\n./other-step.sh\n"
	writeDoctorScript(t, repoRoot, ".git/hooks/pre-commit", composed)
	assertOK(t, precommitAnchorFindings(repoRoot))
}

// A core.hooksPath redirect to a dir whose pre-commit never reaches memento makes the
// byte-perfect installed anchor dead: precommit-shadowed (error).
func TestPrecommitShadowedByHooksPath(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	writeMementoPreCommitHook(t, repoRoot) // installed anchor at .git/hooks
	writeDoctorScript(t, repoRoot, "alt-hooks/pre-commit", benignScriptBody)
	runCLIGit(t, repoRoot, "config", "core.hooksPath", "alt-hooks")

	f := findToken(t, precommitAnchorFindings(repoRoot), tokPrecommitShadowed)
	if f.severity != sevError {
		t.Fatalf("precommit-shadowed severity = %v, want error", f.severity)
	}
}

// A husky-managed hooks dir (core.hooksPath into .husky/_) whose wrapper hands off to the
// user hook at .husky/pre-commit, which reaches memento, is live — not shadowed.
func TestPrecommitHuskyReachesMemento(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	writeMementoPreCommitHook(t, repoRoot) // an installed anchor exists, but is bypassed
	writeDoctorScript(t, repoRoot, ".husky/_/pre-commit", "#!/usr/bin/env sh\n. \"${0%/*}/husky.sh\"\n")
	writeDoctorScript(t, repoRoot, ".husky/pre-commit", "#!/usr/bin/env sh\nmemento compile\n")
	runCLIGit(t, repoRoot, "config", "core.hooksPath", ".husky/_")

	assertOK(t, precommitAnchorFindings(repoRoot))
}

// A lefthook-managed redirect whose launcher dispatches into lefthook.yml, where memento's
// step lives, is reachable — live, not shadowed.
func TestPrecommitLefthookReachesMemento(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	writeMementoPreCommitHook(t, repoRoot)
	writeDoctorScript(t, repoRoot, "lhooks/pre-commit", "#!/bin/sh\nlefthook run pre-commit\n")
	writeDoctorScript(t, repoRoot, "lefthook.yml", "pre-commit:\n  commands:\n    memento:\n      run: memento compile\n")
	runCLIGit(t, repoRoot, "config", "core.hooksPath", "lhooks")

	assertOK(t, precommitAnchorFindings(repoRoot))
}

// Content edited but still reachable is at most a nudge, never an error — script-identity
// drift is not a gate.
func TestPrecommitEditedButReachableNotError(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	edited := "#!/bin/sh\n# locally tweaked by a maintainer\nset -euo pipefail\nmemento compile\n"
	writeDoctorScript(t, repoRoot, ".git/hooks/pre-commit", edited)
	for _, f := range precommitAnchorFindings(repoRoot) {
		if f.severity == sevError {
			t.Fatalf("edited-but-reachable emitted an error: %+v", f)
		}
	}
}

// A pre-commit hook whose only memento line is a commented-out command does not run memento,
// so reachability must not fail open on it: it reads as missing, not live.
func TestPrecommitCommentedStepMissing(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	commented := "#!/bin/sh\nset -eu\n# memento compile\n  # memento clear-grants\nnpm test\n"
	writeDoctorScript(t, repoRoot, ".git/hooks/pre-commit", commented)
	f := findToken(t, precommitAnchorFindings(repoRoot), tokPrecommitMissing)
	if f.severity != sevError {
		t.Fatalf("precommit-anchor-missing severity = %v, want error", f.severity)
	}
}

// A full sentinel-bracketed block whose memento commands are all commented out does not run
// memento: the # memento:start marker is itself a comment, so its presence must not prove the
// ratification-boundary audit runs. Reachability reads it as missing, not live.
func TestPrecommitSentinelBlockCommentedMissing(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	commented := "#!/bin/sh\nset -eu\n# memento:start\n# if command -v memento >/dev/null 2>&1; then\n#   memento compile || exit $?\n#   memento clear-grants\n# fi\n# memento:end\nnpm test\n"
	writeDoctorScript(t, repoRoot, ".git/hooks/pre-commit", commented)
	f := findToken(t, precommitAnchorFindings(repoRoot), tokPrecommitMissing)
	if f.severity != sevError {
		t.Fatalf("precommit-anchor-missing severity = %v, want error", f.severity)
	}
}

// An inline trailing comment after a live memento command does not hide the command: the
// step still reaches memento.
func TestPrecommitInlineCommentStillReaches(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	body := "#!/bin/sh\nset -eu\nmemento compile # ratification-boundary audit\n"
	writeDoctorScript(t, repoRoot, ".git/hooks/pre-commit", body)
	assertOK(t, precommitAnchorFindings(repoRoot))
}

// An effective pre-commit hook that never reaches memento, with no redirect, is absence —
// not shadowing — but the ratification-boundary audit is just as dead, so it is a not-live
// error under the precommit-anchor-missing token.
func TestPrecommitNoAnchorNoRedirectMissing(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	writeDoctorScript(t, repoRoot, ".git/hooks/pre-commit", benignScriptBody)
	f := findToken(t, precommitAnchorFindings(repoRoot), tokPrecommitMissing)
	if f.severity != sevError {
		t.Fatalf("precommit-anchor-missing severity = %v, want error", f.severity)
	}
}

// No pre-commit hook file at all (never wired) is also absence: missing, not shadowed.
func TestPrecommitNoHookFileMissing(t *testing.T) {
	repoRoot := t.TempDir()
	initCLIGit(t, repoRoot)
	f := findToken(t, precommitAnchorFindings(repoRoot), tokPrecommitMissing)
	if f.severity != sevError {
		t.Fatalf("precommit-anchor-missing severity = %v, want error", f.severity)
	}
}

// With no .git tree, precommit-anchor-live SKIPS blocked-by git-repo rather than reporting
// a verdict it cannot ground.
func TestPrecommitSkipsWithNoGit(t *testing.T) {
	outcomes, err := runChecks(doctorNodes(t.TempDir(), vault.Vault{}, vault.ErrVaultNotFound, false))
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}
	pc := outcomeFor(t, outcomes, nodePrecommitAnchor)
	if !pc.skipped || pc.blockedBy != nodeGitRepo {
		t.Fatalf("precommit-anchor outcome = %+v, want skipped blocked-by %s", pc, nodeGitRepo)
	}
	if pc.passed() {
		t.Fatal("a skipped precommit-anchor must not count as passed")
	}
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

// --- config-valid --------------------------------------------------------

// hygieneVault creates a bare vault (marker dir only) for the installation-property
// hygiene nodes, which read files under the vault root and marker dir.
func hygieneVault(t *testing.T) vault.Vault {
	t.Helper()
	root := t.TempDir()
	marker := filepath.Join(root, vault.MarkerDirName)
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	return vault.Vault{Root: root, MarkerDir: marker, ManifestPath: filepath.Join(marker, vault.ManifestFileName)}
}

func writeConfig(t *testing.T, v vault.Vault, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(v.MarkerDir, "config.toml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
}

// An absent config.toml is vacuously valid — presence is init's job, this node only judges
// a file that exists.
func TestConfigValidAbsentOK(t *testing.T) {
	assertOK(t, configValidFindings(hygieneVault(t)))
}

// The comment-only default init writes carries no keys, so it is valid.
func TestConfigValidCommentOnlyOK(t *testing.T) {
	v := hygieneVault(t)
	writeConfig(t, v, "# memento vault configuration\n")
	assertOK(t, configValidFindings(v))
}

// A file the minimal scanner cannot parse is a non-gating WARNING, not a hard error: the
// hand-scanner is not a real TOML parser, so its confusion must not fail the build on what
// may well be valid TOML it does not model (memento-tbu.4).
func TestConfigValidMalformed(t *testing.T) {
	v := hygieneVault(t)
	writeConfig(t, v, "this line is not valid toml\n")
	f := findToken(t, configValidFindings(v), tokConfigInvalid)
	if f.severity != sevWarning {
		t.Fatalf("unparseable config severity = %v, want warning", f.severity)
	}
}

func TestConfigValidUnrecognisedKey(t *testing.T) {
	v := hygieneVault(t)
	writeConfig(t, v, "bogus = \"value\"\n")
	f := findToken(t, configValidFindings(v), tokConfigInvalid)
	if f.severity != sevError || !strings.Contains(f.detail, "bogus") {
		t.Fatalf("unrecognised key: got %+v, want config-invalid error naming bogus", f)
	}
}

func TestConfigValidUnrecognisedTable(t *testing.T) {
	v := hygieneVault(t)
	writeConfig(t, v, "[unknown]\nx = 1\n")
	f := findToken(t, configValidFindings(v), tokConfigInvalid)
	if f.severity != sevError {
		t.Fatalf("unrecognised table severity = %v, want error", f.severity)
	}
}

// withRecognisedKeys temporarily swaps the closed-world allowlist (empty in production)
// so the well-formed-syntax tests below can assert that recognised keys pass config-valid
// rather than tripping the unrecognised-key error, which is orthogonal to the parse step.
func withRecognisedKeys(t *testing.T, keys ...string) {
	t.Helper()
	prev := recognisedConfigKeys
	t.Cleanup(func() { recognisedConfigKeys = prev })
	recognisedConfigKeys = map[string]bool{}
	for _, k := range keys {
		recognisedConfigKeys[k] = true
	}
}

// scanConfigKeys must read well-formed TOML the old hand-scanner rejected as unparseable
// (memento-tbu.4): a `[table]` header with a trailing inline comment, inline comments on
// key lines, a multi-line array, and a multi-line string.
func TestScanConfigKeysWellFormedSyntaxes(t *testing.T) {
	cases := []struct {
		name     string
		contents string
		want     []string
	}{
		{"table header inline comment", "[hooks] # configure hooks\npre = true\n", []string{"hooks", "pre"}},
		{"key line inline comment", "name = \"vault\" # the display name\n", []string{"name"}},
		{"hash inside string is not a comment", "name = \"a#b\"\n", []string{"name"}},
		{"multi-line array", "tags = [\n  \"a\",\n  \"b\",\n]\n", []string{"tags"}},
		{"multi-line string", "note = \"\"\"\nline one\n# not a comment\nline two\n\"\"\"\n", []string{"note"}},
		{"inline table", "owner = { name = \"x\", id = 1 }\n", []string{"owner"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keys, ok := scanConfigKeys(tc.contents)
			if !ok {
				t.Fatalf("scanConfigKeys(%q) ok=false, want true", tc.contents)
			}
			if !reflect.DeepEqual(keys, tc.want) {
				t.Fatalf("scanConfigKeys(%q) keys = %v, want %v", tc.contents, keys, tc.want)
			}
		})
	}
}

// End-to-end: those same well-formed syntaxes must clear config-valid with no finding at
// all when their keys are recognised — proving the parse step no longer raises
// config-invalid on valid TOML.
func TestConfigValidWellFormedSyntaxes(t *testing.T) {
	withRecognisedKeys(t, "hooks", "pre", "name", "tags", "note", "owner")
	cases := []struct {
		name     string
		contents string
	}{
		{"table header inline comment", "[hooks] # configure hooks\npre = true\n"},
		{"key line inline comment", "name = \"vault\" # the display name\n"},
		{"multi-line array", "tags = [\n  \"a\",\n  \"b\",\n]\n"},
		{"multi-line string", "note = \"\"\"\nline one\nline two\n\"\"\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := hygieneVault(t)
			writeConfig(t, v, tc.contents)
			assertOK(t, configValidFindings(v))
		})
	}
}

// --- ignore-correct ------------------------------------------------------

func writeGitignoreStanza(t *testing.T, repoRoot string) {
	t.Helper()
	stanza := gitignoreStartSentinel + "\n**/.memento/grants.json\n" + gitignoreEndSentinel + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitignore"), []byte(stanza), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
}

func writeMementoignore(t *testing.T, v vault.Vault) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(v.Root, vault.IgnoreFileName), []byte(".memento/\n"), 0o644); err != nil {
		t.Fatalf("write .mementoignore: %v", err)
	}
}

func TestIgnoreCorrectOK(t *testing.T) {
	repoRoot := t.TempDir()
	v := hygieneVault(t)
	writeGitignoreStanza(t, repoRoot)
	writeMementoignore(t, v)
	assertOK(t, ignoreCorrectFindings(repoRoot, v))
}

func TestIgnoreCorrectMissingGitignoreStanza(t *testing.T) {
	repoRoot := t.TempDir() // no .gitignore at all
	v := hygieneVault(t)
	writeMementoignore(t, v)
	f := findToken(t, ignoreCorrectFindings(repoRoot, v), tokGitignoreStanzaMissing)
	if f.severity != sevWarning {
		t.Fatalf("missing gitignore stanza severity = %v, want warning", f.severity)
	}
}

func TestIgnoreCorrectMissingMementoignore(t *testing.T) {
	repoRoot := t.TempDir()
	v := hygieneVault(t) // no .mementoignore
	writeGitignoreStanza(t, repoRoot)
	f := findToken(t, ignoreCorrectFindings(repoRoot, v), tokMementoignoreMissing)
	if f.severity != sevWarning {
		t.Fatalf("missing .mementoignore severity = %v, want warning", f.severity)
	}
}

// An incomplete stanza (start sentinel without end) is malformed: still gitignore-stanza-missing.
func TestIgnoreCorrectMalformedStanza(t *testing.T) {
	repoRoot := t.TempDir()
	v := hygieneVault(t)
	writeMementoignore(t, v)
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitignore"), []byte(gitignoreStartSentinel+"\n.memento/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := findToken(t, ignoreCorrectFindings(repoRoot, v), tokGitignoreStanzaMissing)
	if f.severity != sevWarning {
		t.Fatalf("malformed stanza severity = %v, want warning", f.severity)
	}
}

// --- tool-read-files-present ---------------------------------------------

func TestToolReadFilesWritingPresentOK(t *testing.T) {
	v := hygieneVault(t)
	writing := filepath.Join(v.Root, vault.ToolDirName, "conventions", "writing.md")
	if err := os.MkdirAll(filepath.Dir(writing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(writing, []byte("# writing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertOK(t, toolReadFilesFindings(v))
}

// Absent writing.md is a NUDGE that never gates, in any context, strict or not.
func TestToolReadFilesWritingAbsentNudge(t *testing.T) {
	f := findToken(t, toolReadFilesFindings(hygieneVault(t)), tokWritingMdAbsent)
	if f.severity != sevNudge {
		t.Fatalf("writing-md-absent severity = %v, want nudge", f.severity)
	}
	if f.gates(ctxAny, ctxSession, true) {
		t.Fatal("a nudge must never gate, even under strict")
	}
}

// The three hygiene nodes hang off vault-discoverable: with no vault they SKIP, not judge
// files that are not there.
func TestHygieneNodesSkipWithNoVault(t *testing.T) {
	outcomes, err := runChecks(doctorNodes(t.TempDir(), vault.Vault{}, vault.ErrVaultNotFound, false))
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}
	for _, name := range []string{nodeConfigValid, nodeIgnoreCorrect, nodeToolReadFiles} {
		o := outcomeFor(t, outcomes, name)
		if !o.skipped || o.blockedBy != nodeVaultDiscoverable {
			t.Fatalf("%s outcome = %+v, want skipped blocked-by %s", name, o, nodeVaultDiscoverable)
		}
		if o.passed() {
			t.Fatalf("a skipped %s must not count as passed", name)
		}
	}
}

// --- full DAG ------------------------------------------------------------

// TestGrantFreshSkipsWithNoVault is the dishonest-OK fix: with no vault, grant-fresh
// must SKIP blocked-by vault-discoverable, not report ok.
func TestGrantFreshSkipsWithNoVault(t *testing.T) {
	outcomes, err := runChecks(doctorNodes(t.TempDir(), vault.Vault{}, vault.ErrVaultNotFound, false))
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

// installHygieneFiles establishes the config / ignore / tool-read files the ADR-0032
// hygiene nodes assert, so a fully-init'd vault passes config-valid, ignore-correct, and
// tool-read-files-present. It mirrors what `memento init` writes.
func installHygieneFiles(t *testing.T, repoRoot string, v vault.Vault) {
	t.Helper()
	// config.toml: comment-only is valid (no recognised keys to violate).
	if err := os.WriteFile(filepath.Join(v.MarkerDir, "config.toml"), []byte("# memento vault configuration\n"), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	// .gitignore stanza at the repo root.
	gitignore := gitignoreStartSentinel + "\n**/.memento/grants.json\n" + gitignoreEndSentinel + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitignore"), []byte(gitignore), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	// .mementoignore at the vault root.
	if err := os.WriteFile(filepath.Join(v.Root, vault.IgnoreFileName), []byte(".memento/\n"), 0o644); err != nil {
		t.Fatalf("write .mementoignore: %v", err)
	}
	// writing convention under _memento/conventions/.
	writing := filepath.Join(v.Root, vault.ToolDirName, "conventions", "writing.md")
	if err := os.MkdirAll(filepath.Dir(writing), 0o755); err != nil {
		t.Fatalf("mkdir conventions: %v", err)
	}
	if err := os.WriteFile(writing, []byte("---\ntitle: Writing guide\nwhen_to_read: x\n---\n# Writing\n"), 0o644); err != nil {
		t.Fatalf("write writing.md: %v", err)
	}
}

func TestDoctorDAGLive(t *testing.T) {
	repoRoot := liveClaudeRepo(t)
	realBin(t)
	stubGateSchema(t, manifest.CurrentSchemaVersion)
	v := doctorVault(t, repoRoot, manifest.CurrentSchemaVersion)
	installHygieneFiles(t, repoRoot, v)
	outcomes, err := runChecks(doctorNodes(repoRoot, v, nil, false))
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

// --session keeps the loud LIVE headline (the whole point of the ambient signal) while
// rendering the two corpus-scaling checks as deferred, and still exits clean.
func TestRunDoctorSessionHeadlineAndDeferral(t *testing.T) {
	t.Setenv("CI", "")
	repoRoot := liveClaudeRepo(t)
	realBin(t)
	stubGateSchema(t, manifest.CurrentSchemaVersion)
	doctorVault(t, repoRoot, manifest.CurrentSchemaVersion)
	chdirCLI(t, repoRoot)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor", "--session"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor --session exit = %d, want 0; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.HasPrefix(out, "vault write enforcement: LIVE") {
		t.Fatalf("headline = %q, want LIVE prefix under --session", out)
	}
	for _, node := range []string{nodeManifestFresh, nodeLiveFire} {
		if want := "[skip] " + node + ": deferred"; !strings.Contains(out, want) {
			t.Fatalf("output missing deferred line %q; got:\n%s", want, out)
		}
	}
}

func TestRunDoctorExitOff(t *testing.T) {
	t.Setenv("CI", "")
	// A wired gate and a resolvable vault, but the binary the gate shells to is missing:
	// enforcement is genuinely OFF (binary-not-on-path), distinct from "no vault here".
	repoRoot := liveClaudeRepo(t)
	doctorVault(t, repoRoot, manifest.CurrentSchemaVersion)
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

// --- degenerate cases (integration) --------------------------------------
//
// These three prove the ADR-0032 skip-cascade reads cleanly across the REAL check set:
// the most common invocations (no vault / outside a project, ambiguous markers, non-git
// tree) each produce a coherent report with a specific exit code and never panic.

// No vault / run outside a project: vault-discoverable fails as the DAG root, so every
// vault-dependent check skips blocked-by vault-discoverable; doctor leads with "no memento
// vault here" and exits 1 (vault-absent gates) — not a cascade of look-alike errors, not a
// crash.
func TestRunDoctorNoVault(t *testing.T) {
	t.Setenv("CI", "")
	repoRoot := t.TempDir() // empty: no vault, no gate, no git
	realBin(t)              // binary resolves, so the missing vault is the only gating fact
	chdirCLI(t, repoRoot)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("no-vault exit = %d, want 1 (vault-absent gates); stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.HasPrefix(out, "no memento vault here") {
		t.Fatalf("headline = %q, want 'no memento vault here' lead", out)
	}
	// Every vault-dependent check skips, naming vault-discoverable — a coherent cascade.
	for _, node := range []string{nodeManifestPresent, nodeConfigValid, nodeIgnoreCorrect, nodeToolReadFiles, nodeGrantFresh} {
		want := "[skip] " + node + ": blocked-by " + nodeVaultDiscoverable
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q; got:\n%s", want, out)
		}
	}
	// No git either: precommit-anchor skips blocked-by git-repo, distinct from the vault skips.
	if want := "[skip] " + nodePrecommitAnchor + ": blocked-by " + nodeGitRepo; !strings.Contains(out, want) {
		t.Fatalf("output missing %q; got:\n%s", want, out)
	}
}

// Non-git vault: SUPPORTED with a named capability loss. grant-fresh and precommit-anchor
// skip blocked-by git-repo, but the git-independent live-fire probe still runs, so
// enforcement liveness is STILL assertable — the headline is LIVE and the exit is clean.
func TestRunDoctorNonGitVault(t *testing.T) {
	t.Setenv("CI", "")
	repoRoot := t.TempDir() // deliberately not a git tree
	pre := writeDoctorScript(t, repoRoot, ".claude/memento-pre-write-vault-guard.sh", preWriteScriptBody)
	post := writeDoctorScript(t, repoRoot, ".claude/memento-post-write-compile.sh", postWriteScriptBody)
	writeClaudeSettings(t, repoRoot,
		hookSpec{"PreToolUse", "Write|Edit|MultiEdit|Bash", pre},
		hookSpec{"PostToolUse", "Write|Edit|MultiEdit|Bash", post},
	)
	realBin(t)
	stubGateSchema(t, manifest.CurrentSchemaVersion)
	doctorVault(t, repoRoot, manifest.CurrentSchemaVersion)
	chdirCLI(t, repoRoot)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("non-git vault exit = %d, want 0; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.HasPrefix(out, "vault write enforcement: LIVE") {
		t.Fatalf("headline = %q, want LIVE (enforcement is assertable without git)", out)
	}
	for _, node := range []string{nodePrecommitAnchor, nodeGrantFresh} {
		want := "[skip] " + node + ": blocked-by " + nodeGitRepo
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q; got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "[ok] "+nodeLiveFire) {
		t.Fatalf("live-fire did not run/pass without git; got:\n%s", out)
	}
}

// Ambiguous vault (multiple markers): vault-discoverable emits vault-ambiguous (error) with
// the MEMENTO_VAULT_ROOT remediation; doctor exits 1 and does not panic.
func TestRunDoctorAmbiguousVault(t *testing.T) {
	t.Setenv("CI", "")
	repoRoot := t.TempDir()
	for _, sub := range []string{"alpha", "beta"} { // two marker dirs => ambiguous discovery
		if err := os.MkdirAll(filepath.Join(repoRoot, sub, vault.MarkerDirName), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	realBin(t)
	chdirCLI(t, repoRoot)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("ambiguous-vault exit = %d, want 1 (vault-ambiguous gates); stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, tokVaultAmbiguous) {
		t.Fatalf("output missing %q token; got:\n%s", tokVaultAmbiguous, out)
	}
	if !strings.Contains(out, "MEMENTO_VAULT_ROOT") {
		t.Fatalf("output missing MEMENTO_VAULT_ROOT remediation; got:\n%s", out)
	}
}
