package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

// doctor is memento's loud write-enforcement liveness signal (ADR-0031 names this
// a hard dependency; the cadence note [[doctor-scoping]] carves it out of the full
// doctor verb as the urgent-and-missing piece). Mode enforcement rests entirely on
// the PreToolUse check-write hook firing, and that failure is SILENT — the harness
// is fail-open on hook absence, crash, or a missing binary. The commit-time
// diff-audit backstop is detective, not preventive. doctor is the only loud surface
// for "is enforcement actually on", so v1 does liveness ONLY: every mechanical check
// that the gate is wired, the binary is reachable, no legacy guard bricks the vault,
// and — the one check that proves the CHAIN, not just that parts exist — a live-fire
// self-test that a read-only overwrite is actually denied. The rest of doctor's
// candidate checks (config validity, manifest freshness, malformed conventions) stay
// deferred to the future doctor ADR.

const doctorHelpText = `memento doctor

Usage:
  memento doctor

Report whether vault write enforcement is LIVE: the PreToolUse check-write gate is
wired and executable, the memento binary the gate shells to is reachable and not
older than the on-disk manifest schema, no legacy broad-deny guard bricks the vault,
and a live-fire self-test confirms a read-only overwrite is actually denied.

Exit status:
  0   enforcement is LIVE.
  1   enforcement is OFF (a critical check failed) or doctor could not run.

No flags.

For the deeper picture, run: memento orient
`

// checkLevel is a single check's outcome. statusFail is the only level that flips
// the headline to OFF and the exit code to non-zero; statusWarn keeps enforcement
// LIVE but surfaces a degraded backstop or hygiene signal.
type checkLevel int

const (
	statusOK checkLevel = iota
	statusWarn
	statusFail
)

func (l checkLevel) tag() string {
	switch l {
	case statusOK:
		return "ok"
	case statusWarn:
		return "warn"
	default:
		return "FAIL"
	}
}

// checkResult is one mechanical liveness check. reason is the short clause used to
// build the OFF headline when status is statusFail; detail is the per-check line.
type checkResult struct {
	name   string
	status checkLevel
	reason string
	detail string
}

// wiredHook is one agent-configured hook command, normalised across families: the
// Claude settings.json array entries and the codex config.toml `[[hooks.<Event>]]`
// tables both flatten to (event, matcher, command). doctor classifies a command by
// reading the script it points at, not by trusting its filename.
type wiredHook struct {
	event   string
	matcher string
	command string
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	if ok, code := parseSubcommandFlags(flags, args, stdout, stderr, "doctor", doctorHelpText); !ok {
		return code
	}
	if flags.NArg() != 0 {
		printCLIError(stderr, "doctor", fmt.Errorf("%w: unexpected argument %q", ErrInvalidArguments, flags.Arg(0)))
		return 2
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		printCLIError(stderr, "doctor", fmt.Errorf("%w: get current directory: %v", ErrIO, err))
		return 1
	}

	v, vaultErr := resolveVault()

	results, caveat := runDoctorChecks(repoRoot, v, vaultErr)
	live := isLive(results)

	printDoctorReport(stdout, results, live, caveat)
	if !live {
		return 1
	}
	return 0
}

// runDoctorChecks runs every liveness check and returns the per-check results plus
// any platform caveat (codex gates only apply_patch; raw shell writes are ungated —
// ryr.39 — so a codex LIVE is never a bare LIVE). vaultErr carries a failed vault
// resolution: the grant and live-fire checks need a vault, but the gate checks read
// the agent config under repoRoot and run regardless.
func runDoctorChecks(repoRoot string, v vault.Vault, vaultErr error) (results []checkResult, caveat string) {
	// A family counts only if memento actually wired a gate for it — its installed
	// pre-write script is present. A bare .claude/ or .codex/ from another tool (a
	// beads-only .codex/config.toml, say) is not a memento-enforced family and must
	// not force a phantom gate failure. gateChecks still validates that the script is
	// referenced and executable, so a deleted reference under a present script still
	// fails loudly.
	claudeWired := fileExists(filepath.Join(repoRoot, ".claude", "memento-pre-write-vault-guard.sh"))
	codexWired := fileExists(filepath.Join(repoRoot, ".codex", "memento-pre-write-vault-guard.sh"))

	if !claudeWired && !codexWired {
		results = append(results, checkResult{
			name:   "PreToolUse gate",
			status: statusFail,
			reason: "no memento gate installed",
			detail: "no memento PreToolUse gate wired for any agent under " + repoRoot + "; run memento init",
		})
	}
	if claudeWired {
		results = append(results, gateChecks(repoRoot, "claude", readClaudeHooks(repoRoot), claudeMatcherCovers)...)
	}
	if codexWired {
		results = append(results, gateChecks(repoRoot, "codex", readCodexHooks(repoRoot), codexMatcherCovers)...)
		caveat = "apply_patch only; raw shell writes are ungated on codex"
	}

	results = append(results, binaryReachableCheck(v, vaultErr))
	results = append(results, liveFireCheck())
	results = append(results, staleGrantCheck(v, vaultErr))
	return results, caveat
}

func isLive(results []checkResult) bool {
	for _, r := range results {
		if r.status == statusFail {
			return false
		}
	}
	return true
}

func printDoctorReport(stdout io.Writer, results []checkResult, live bool, caveat string) {
	if live {
		if caveat != "" {
			fmt.Fprintf(stdout, "vault write enforcement: LIVE (%s)\n", caveat)
		} else {
			fmt.Fprintln(stdout, "vault write enforcement: LIVE")
		}
	} else {
		fmt.Fprintf(stdout, "vault write enforcement: OFF (%s)\n", firstFailReason(results))
	}
	for _, r := range results {
		fmt.Fprintf(stdout, "  [%s] %s: %s\n", r.status.tag(), r.name, r.detail)
	}
}

func firstFailReason(results []checkResult) string {
	for _, r := range results {
		if r.status == statusFail {
			return r.reason
		}
	}
	return "unknown"
}

// gateChecks evaluates the two wiring checks plus the legacy-guard check for one
// agent family from its flattened hooks. covers reports whether a matcher covers
// the family's write tools (Claude: Write|Edit|MultiEdit|Bash; codex: apply_patch).
func gateChecks(repoRoot, family string, hooks []wiredHook, covers func(matcher string) (full, fileTools bool)) []checkResult {
	var results []checkResult
	var gate *resolvedHook
	var postGate *resolvedHook
	var legacy *resolvedHook
	var gateMatcher string

	for _, h := range hooks {
		rh := resolveHookCommand(repoRoot, h.command)
		switch {
		case rh.content != "" && isLegacyBroadDeny(rh.content):
			legacy = &rh
		case rh.content != "" && strings.Contains(rh.content, "check-write"):
			if h.event == "PreToolUse" {
				g := rh
				gate = &g
				gateMatcher = h.matcher
			}
		case strings.Contains(filepath.Base(h.command), "pre-write-vault-guard"):
			// A memento gate by filename whose script does not resolve/read: record it
			// so check #1 fails loudly with "command does not resolve" rather than
			// silently reporting no gate at all.
			if h.event == "PreToolUse" && gate == nil {
				g := rh
				gate = &g
				gateMatcher = h.matcher
			}
		}
		if h.event == "PostToolUse" && rh.content != "" && strings.Contains(rh.content, "compile") {
			p := rh
			postGate = &p
		}
	}

	results = append(results, gateCheck(family, gate, gateMatcher, covers))
	results = append(results, postWriteCheck(family, postGate))
	results = append(results, legacyGuardCheck(family, legacy))
	return results
}

// gateCheck is liveness check #1: a PreToolUse check-write gate exists, its command
// resolves to an executable file, and its matcher covers the write tools.
func gateCheck(family string, gate *resolvedHook, matcher string, covers func(matcher string) (full, fileTools bool)) checkResult {
	name := family + " PreToolUse gate"
	if gate == nil {
		return checkResult{name: name, status: statusFail,
			reason: "no PreToolUse check-write gate", detail: "no memento check-write hook found"}
	}
	if !gate.exists {
		return checkResult{name: name, status: statusFail,
			reason: "gate command does not resolve", detail: "gate command " + gate.command + " does not exist"}
	}
	if !gate.executable {
		return checkResult{name: name, status: statusFail,
			reason: "gate command not executable", detail: gate.command + " is not executable"}
	}
	full, fileTools := covers(matcher)
	if !fileTools {
		return checkResult{name: name, status: statusFail,
			reason: "gate matcher misses write tools", detail: fmt.Sprintf("matcher %q does not cover the write tools", matcher)}
	}
	if !full {
		return checkResult{name: name, status: statusWarn,
			detail: fmt.Sprintf("%s (matcher %q does not cover every write tool; some writes are ungated)", gate.command, matcher)}
	}
	return checkResult{name: name, status: statusOK,
		detail: fmt.Sprintf("%s (matcher %q)", gate.command, matcher)}
}

// postWriteCheck is liveness check #2. A missing PostToolUse compile hook degrades
// the detective drift backstop but does not disable the preventive PreToolUse gate,
// so it warns rather than flipping enforcement OFF.
func postWriteCheck(family string, post *resolvedHook) checkResult {
	name := family + " PostToolUse compile hook"
	if post == nil {
		return checkResult{name: name, status: statusWarn,
			detail: "no compile hook found; the post-write drift alarm is not wired"}
	}
	if !post.exists {
		return checkResult{name: name, status: statusWarn,
			detail: "compile hook " + post.command + " does not exist; drift alarm not wired"}
	}
	if !post.executable {
		return checkResult{name: name, status: statusWarn,
			detail: post.command + " is not executable; drift alarm not wired"}
	}
	return checkResult{name: name, status: statusOK, detail: post.command}
}

// legacyGuardCheck is liveness check #4. A PreToolUse hook pointing at the
// pre-ADR-0031 broad-deny guard bricks the vault: it denies every vault write and
// there is no surviving `write` verb to satisfy it. Flag it as a hard failure.
func legacyGuardCheck(family string, legacy *resolvedHook) checkResult {
	name := family + " no legacy broad-deny guard"
	if legacy != nil {
		return checkResult{name: name, status: statusFail,
			reason: "legacy broad-deny guard wired",
			detail: legacy.command + " is a pre-ADR-0031 broad-deny guard; it bricks the vault (deny with no write verb). Remove it."}
	}
	return checkResult{name: name, status: statusOK, detail: "none"}
}

// binaryReachableCheck is liveness check #3. The gate shells to ${MEMENTO_BIN:-memento};
// if that binary is not on PATH the gate fails closed and every vault write is
// blocked, and if the binary is older than the on-disk manifest schema it cannot
// read the vault it is meant to guard. Either is fatal to a useful LIVE.
func binaryReachableCheck(v vault.Vault, vaultErr error) checkResult {
	const name = "check-write binary"
	bin := os.Getenv("MEMENTO_BIN")
	if bin == "" {
		bin = "memento"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return checkResult{name: name, status: statusFail,
			reason: bin + " not on PATH",
			detail: fmt.Sprintf("the gate shells to %q, which is not reachable; the gate would fail closed and block every vault write", bin)}
	}
	if vaultErr != nil {
		// No vault resolved (or ambiguous): we cannot compare against a manifest
		// schema, but the binary is reachable, which is the part this check owns.
		return checkResult{name: name, status: statusOK,
			detail: bin + " on PATH (manifest schema not checked: no resolved vault)"}
	}
	schema, present, err := readManifestSchemaVersion(v)
	if err != nil {
		return checkResult{name: name, status: statusWarn,
			detail: bin + " on PATH; manifest schema unreadable: " + err.Error()}
	}
	if !present {
		return checkResult{name: name, status: statusWarn,
			detail: bin + " on PATH; no manifest to check schema against (run memento compile)"}
	}
	if schema > manifest.CurrentSchemaVersion {
		return checkResult{name: name, status: statusFail,
			reason: "binary older than manifest schema",
			detail: fmt.Sprintf("manifest schema %d exceeds the schema %d this binary supports; the gate cannot read the vault", schema, manifest.CurrentSchemaVersion)}
	}
	return checkResult{name: name, status: statusOK,
		detail: fmt.Sprintf("%s on PATH; manifest schema %d <= binary %d", bin, schema, manifest.CurrentSchemaVersion)}
}

// liveFireCheck is liveness check #6 and the only check that proves the CHAIN rather
// than that parts exist: it synthesises a ratified read-only note in a throwaway
// vault and runs the same verdict chokepoint every in-vault write flows through,
// asserting the read-only overwrite is denied. A non-deny means the lattice no
// longer bites and flips enforcement OFF.
//
// The fixture is always synthetic (settling the bead's open question): a temp,
// non-git vault treats every note as ratified (the edit window only opens inside a
// git work tree), so the probe needs no commit and the real vault's notes and
// decision log are never touched — no residue, no dependence on the vault already
// holding a read-only note.
func liveFireCheck() checkResult {
	const name = "live-fire self-test"
	denied, reasonCode, err := liveFireReadOnlyProbe()
	if err != nil {
		return checkResult{name: name, status: statusFail,
			reason: "live-fire self-test could not run", detail: "synthetic read-only probe failed: " + err.Error()}
	}
	if !denied {
		return checkResult{name: name, status: statusFail,
			reason: "read-only overwrite was not denied",
			detail: "synthetic read-only overwrite returned " + reasonCode + ", not a read_only deny; the mode lattice is not enforcing"}
	}
	return checkResult{name: name, status: statusOK, detail: "read-only overwrite denied (" + reasonCode + ")"}
}

// liveFireReadOnlyProbe builds a throwaway vault holding a ratified read-only note,
// then runs the live verdict engine (computeVaultWriteVerdict — the single
// chokepoint checkWriteFile funnels every in-vault file verdict through) against a
// Write that would rewrite it, returning whether the verdict denied. The temp vault
// is removed on return, so the probe leaves no residue.
func liveFireReadOnlyProbe() (denied bool, reasonCode string, err error) {
	dir, err := os.MkdirTemp("", "memento-doctor-probe-")
	if err != nil {
		return false, "", fmt.Errorf("create probe vault: %w", err)
	}
	defer os.RemoveAll(dir)

	marker := filepath.Join(dir, vault.MarkerDirName)
	if err := os.MkdirAll(marker, 0o755); err != nil {
		return false, "", fmt.Errorf("create probe marker: %w", err)
	}
	v := vault.Vault{
		Root:         dir,
		MarkerDir:    marker,
		ManifestPath: filepath.Join(marker, vault.ManifestFileName),
	}

	const key = "doctor-probe.md"
	original := "---\nmode: read-only\n---\n# Doctor probe\n\nFrozen.\n"
	if err := os.WriteFile(filepath.Join(dir, key), []byte(original), 0o644); err != nil {
		return false, "", fmt.Errorf("write probe note: %w", err)
	}

	tampered := []byte(original + "\nTampered by the doctor live-fire self-test.\n")
	verdict, err := computeVaultWriteVerdict(v, key, "Write", enforce.ReasonAppendOnlyOverwrite,
		func(_ []byte, _ bool) ([]byte, error) { return tampered, nil }, io.Discard)
	if err != nil {
		return false, "", fmt.Errorf("compute probe verdict: %w", err)
	}
	return verdict.decision == "deny" && verdict.reasonCode == enforce.ReasonReadOnly, verdict.reasonCode, nil
}

// staleGrantCheck is liveness check #5: an unlock grant whose key has no matching
// uncommitted edit is stale — the edit it thawed the note for already committed (and
// should have re-locked) or never happened. It is a hygiene warning, not an
// enforcement failure, so it never flips enforcement OFF.
func staleGrantCheck(v vault.Vault, vaultErr error) checkResult {
	const name = "unlock grants"
	if vaultErr != nil {
		return checkResult{name: name, status: statusOK, detail: "not checked (no resolved vault)"}
	}
	grants, err := enforce.LoadGrants(v)
	if err != nil {
		return checkResult{name: name, status: statusWarn, detail: "could not read unlock grants: " + err.Error()}
	}
	if len(grants) == 0 {
		return checkResult{name: name, status: statusOK, detail: "none"}
	}
	var stale []string
	for key := range grants {
		edited, err := keyHasUncommittedEdit(v.Root, key)
		if err != nil {
			// Cannot prove staleness (no git, git error): do not warn on what we
			// cannot determine.
			continue
		}
		if !edited {
			stale = append(stale, key)
		}
	}
	if len(stale) == 0 {
		return checkResult{name: name, status: statusOK, detail: fmt.Sprintf("%d active, none stale", len(grants))}
	}
	return checkResult{name: name, status: statusWarn,
		detail: fmt.Sprintf("stale grant(s) with no pending edit: %s; commit or drop them", strings.Join(stale, ", "))}
}

// resolvedHook is a hook command resolved against the filesystem: whether its path
// exists, is executable, and its contents (empty when unreadable), so a check can
// classify the script by what it does, not by its name.
type resolvedHook struct {
	command    string
	exists     bool
	executable bool
	content    string
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func resolveHookCommand(repoRoot, command string) resolvedHook {
	rh := resolvedHook{command: command}
	if command == "" {
		return rh
	}
	path := command
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return rh
	}
	rh.exists = !info.IsDir()
	rh.executable = hookExecutable(info)
	if data, err := os.ReadFile(path); err == nil {
		rh.content = string(data)
	}
	return rh
}

// hookExecutable reports whether a hook script is runnable. Unix needs an execute
// bit; Windows has no such bit (runnability is by association), so existence is the
// honest answer there and the bead's Windows CI stays green.
func hookExecutable(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// isLegacyBroadDeny recognises the pre-ADR-0031 broad-deny guard: a memento vault
// guard that denies writes and routes them through the now-removed `memento write`
// verb, and crucially predates `check-write`. The ADR-0031 dumb-pipe gate always
// shells to `check-write`, so its presence is the discriminator.
func isLegacyBroadDeny(content string) bool {
	if strings.Contains(content, "check-write") {
		return false
	}
	return strings.Contains(content, "memento") && strings.Contains(content, "permission_decision")
}

// claudeMatcherCovers reports whether a Claude matcher covers the write tools. full
// requires every tool init wires (Write|Edit|MultiEdit|Bash); fileTools requires at
// least the derivable file tools (Write/Edit/MultiEdit) — without those the gate
// does not see ordinary note writes at all.
func claudeMatcherCovers(matcher string) (full, fileTools bool) {
	has := func(tool string) bool { return matcherHasTool(matcher, tool) }
	fileTools = has("Write") && has("Edit") && has("MultiEdit")
	full = fileTools && has("Bash")
	return full, fileTools
}

// codexMatcherCovers reports whether a codex matcher covers its one hookable write
// tool, apply_patch (codex matches exact tool names; shell writes never fire the
// PreToolUse hook — ryr.39 — which is why the caveat rides every codex LIVE).
func codexMatcherCovers(matcher string) (full, fileTools bool) {
	covered := matcherHasTool(matcher, "apply_patch")
	return covered, covered
}

// matcherHasTool reports whether a `|`-separated matcher names tool exactly,
// matching how both Claude and codex split the matcher into tool names.
func matcherHasTool(matcher, tool string) bool {
	for _, part := range strings.Split(matcher, "|") {
		if strings.TrimSpace(part) == tool {
			return true
		}
	}
	return false
}

// readClaudeHooks flattens .claude/settings.json's hooks object into wiredHooks. A
// missing or malformed file yields no hooks rather than an error — doctor's job is
// to report the gate missing, loudly, not to abort.
func readClaudeHooks(repoRoot string) (hooks []wiredHook) {
	path := filepath.Join(repoRoot, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var parsed struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	for event, entries := range parsed.Hooks {
		for _, entry := range entries {
			for _, h := range entry.Hooks {
				hooks = append(hooks, wiredHook{event: event, matcher: entry.Matcher, command: h.Command})
			}
		}
	}
	return hooks
}

// readCodexHooks flattens the memento sentinel block in .codex/config.toml into
// wiredHooks. memento has no TOML dependency (init writes the block with string
// builders), so this is a line scan of the memento-owned block only: it tracks the
// current `[[hooks.<Event>]]` header and captures each `command`/`matcher` under it.
func readCodexHooks(repoRoot string) (hooks []wiredHook) {
	path := filepath.Join(repoRoot, ".codex", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	contents := string(data)
	start := strings.Index(contents, "# memento:start")
	if start == -1 {
		return nil
	}
	block := contents[start:]
	if end := strings.Index(block, "# memento:end"); end != -1 {
		block = block[:end]
	}

	var event, matcher string
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "[[hooks.") && !strings.Contains(trimmed, ".hooks]]"):
			event = strings.TrimSuffix(strings.TrimPrefix(trimmed, "[[hooks."), "]]")
			matcher = ""
		case strings.HasPrefix(trimmed, "matcher"):
			matcher = tomlStringValue(trimmed)
		case strings.HasPrefix(trimmed, "command"):
			hooks = append(hooks, wiredHook{event: event, matcher: matcher, command: tomlStringValue(trimmed)})
		}
	}
	return hooks
}

// tomlStringValue reads the basic-string value of a `key = "value"` line, undoing
// the backslash/quote escaping init's tomlBasicString applies. It is the minimal
// inverse the hook scan needs, not a TOML parser.
func tomlStringValue(line string) string {
	_, rhs, ok := strings.Cut(line, "=")
	if !ok {
		return ""
	}
	rhs = strings.TrimSpace(rhs)
	rhs = strings.TrimPrefix(rhs, `"`)
	rhs = strings.TrimSuffix(rhs, `"`)
	r := strings.NewReplacer(`\"`, `"`, `\\`, `\`)
	return r.Replace(rhs)
}

func readManifestSchemaVersion(v vault.Vault) (version int, present bool, err error) {
	data, err := os.ReadFile(v.ManifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, err
	}
	var head struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return 0, false, err
	}
	return head.SchemaVersion, true, nil
}

// keyHasUncommittedEdit reports whether the vault-relative key shows an uncommitted
// change in the working tree (modified, staged, or untracked). A non-git tree or a
// git error returns an error so the caller can decline to judge staleness rather
// than warn on what it cannot determine.
func keyHasUncommittedEdit(root, key string) (bool, error) {
	cmd := exec.Command("git", "--literal-pathspecs", "status", "--porcelain", "--", filepath.FromSlash(key))
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}
