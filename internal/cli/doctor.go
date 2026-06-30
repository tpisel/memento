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
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/tpisel/memento/internal/convention"
	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

// doctor is memento's loud write-enforcement liveness signal (ADR-0031 names this a
// hard dependency). The shipped v1 liveness checks (memento-aan, memento-mbd) are
// retrofitted here onto the ADR-0032 precondition-DAG engine (doctor_engine.go): each
// check is a checkNode emitting the canonical ADR-0032 tokens, run through runChecks
// so a failed precondition SKIPS its dependents (exit-neutral but not green) rather
// than being dishonestly reported ok. Mode enforcement rests entirely on the
// PreToolUse check-write hook firing, and that failure is SILENT — the harness is
// fail-open on hook absence, crash, or a missing binary; the commit-time diff-audit
// backstop is detective, not preventive. doctor is the only loud surface for "is
// enforcement actually on", so the liveness-class nodes here prove the gate is wired,
// the binary is reachable, no legacy guard bricks the vault, and — the one check that
// proves the CHAIN, not just that parts exist — a live-fire self-test that a read-only
// overwrite is actually denied. The schema-compat split (binary-schema-compatible /
// manifest-schema-readable) lands here; the remaining hygiene nodes land in dependent
// beads.

const doctorHelpText = `memento doctor

Usage:
  memento doctor [--session]

Report whether vault write enforcement is LIVE: the PreToolUse check-write gate is
wired and executable, the memento binary the gate shells to is reachable, no legacy
broad-deny guard bricks the vault, and a live-fire self-test confirms a read-only
overwrite is actually denied. Alongside that, it checks vault/installation hygiene
(manifest freshness, config validity, ignore stanzas).

  --session   session cadence: defer the corpus-scaling checks (the manifest-fresh
              recompile and the live-fire probe) to the manual/CI cadence so the
              SessionStart orient hook stays cheap. The LIVE/OFF headline is still
              emitted from the structural liveness checks.

Each check reports one of four severities, or skips:
  error     vault unusable or enforcement off; flips the headline and gates.
  warning   degraded but usable; gates only under MEMENTO_DOCTOR_STRICT.
  nudge     advisory; never gates.
  ok        passed.
  skip      a precondition failed, so the check did not run — exit-neutral, not green.

Environment:
  MEMENTO_DOCTOR_STRICT   truthy promotes warnings to gating findings (CI opt-in;
                          parsed like MEMENTO_STRICT_COMMIT).

Exit status:
  0   no gating finding in this context.
  1   a gating finding (an error, or a warning under MEMENTO_DOCTOR_STRICT).
  2   usage error.

For the deeper picture, run: memento orient
`

// Canonical ADR-0032 node names. A node is named for the property it asserts, phrased
// positively; the DAG and preconditions reference these.
const (
	nodeVaultDiscoverable  = "vault-discoverable"
	nodeGateCommitted      = "gate-committed-config"
	nodeGateEffectiveLocal = "gate-effective-local"
	nodePostwriteHook      = "postwrite-hook-live"
	nodeNoLegacyBroadDeny  = "no-legacy-broad-deny"
	nodeBinaryOnPath       = "binary-on-path"
	nodeBinarySchemaCompat = "binary-schema-compatible"
	nodeManifestPresent    = "manifest-present"
	nodeManifestSchemaRead = "manifest-schema-readable"
	nodeManifestFresh      = "manifest-fresh"
	nodeLiveFire           = "live-fire"
	nodeGrantFresh         = "grant-fresh"
	nodeGitRepo            = "git-repo"
	nodePrecommitAnchor    = "precommit-anchor-live"
	nodeConfigValid        = "config-valid"
	nodeIgnoreCorrect      = "ignore-correct"
	nodeToolReadFiles      = "tool-read-files-present"
)

// Canonical ADR-0032 failure tokens (the catalog table is the only source of these
// spellings). A token is the stable wire value of a specific failure a node emits; one
// node may emit one-to-many distinct tokens. A downstream consumer branches on the
// token, never on a node name.
const (
	tokGateMissing            = "gate-missing"
	tokGateUnresolved         = "gate-unresolved"
	tokGateMatcherPartial     = "gate-matcher-partial"
	tokGateLocallyOverridden  = "gate-locally-overridden"
	tokPostwriteHookMissing   = "postwrite-hook-missing"
	tokLegacyBroadDenyWired   = "legacy-broad-deny-wired"
	tokBinaryNotOnPath        = "binary-not-on-path"
	tokBinarySchemaTooOld     = "binary-schema-too-old"
	tokManifestNotFound       = "manifest-not-found"
	tokManifestSchemaUnread   = "manifest-schema-unreadable"
	tokManifestStale          = "manifest-stale"
	tokLiveFireNotDenied      = "live-fire-not-denied"
	tokGrantStale             = "grant-stale"
	tokVaultAmbiguous         = "vault-ambiguous"
	tokVaultAbsent            = "vault-absent"
	tokPrecommitShadowed      = "precommit-shadowed"
	tokPrecommitMissing       = "precommit-anchor-missing"
	tokConfigInvalid          = "config-invalid"
	tokGitignoreStanzaMissing = "gitignore-stanza-missing"
	tokWritingMdAbsent        = "writing-md-absent"
)

func runDoctor(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	sessionMode := flags.Bool("session", false, "session cadence: defer the corpus-scaling checks (manifest-fresh recompile, live-fire probe) to manual/CI; the SessionStart orient hook passes this so the hot path stays cheap")
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

	outcomes, err := runChecks(doctorNodes(repoRoot, v, vaultErr, *sessionMode))
	if err != nil {
		// A malformed DAG (duplicate node, unknown precondition, cycle) is a
		// programming error in the node set, not a vault condition.
		printCLIError(stderr, "doctor", fmt.Errorf("%w: doctor check graph is malformed: %v", ErrIO, err))
		return 1
	}

	caveat := ""
	if fileExists(filepath.Join(repoRoot, ".codex", "memento-pre-write-vault-guard.sh")) {
		// codex gates only apply_patch; raw shell writes are ungated (ryr.39), so a
		// codex LIVE is never a bare LIVE.
		caveat = "apply_patch only; raw shell writes are ungated on codex"
	}
	printDoctorHeadline(stdout, outcomes, caveat)
	renderEngineReport(stdout, outcomes)
	// --session declares the session cadence: CI never passes it, so gate as ctxSession
	// rather than re-detecting; the deferred corpus-scaling checks gate nowhere anyway.
	ctx := detectContext()
	if *sessionMode {
		ctx = ctxSession
	}
	return computeExitCode(outcomes, ctx, doctorStrict())
}

// doctorNodes assembles the ADR-0032 check DAG over a memento installation. The gate /
// postwrite / legacy nodes read the COMMITTED agent config under repoRoot and run
// regardless of vault resolution; the manifest chain and grant-fresh need a vault, so
// they hang off vault-discoverable and SKIP (blocked-by) when no vault resolves — never a
// green "not checked". The manifest chain is the ADR-0032 spine
// vault-discoverable → manifest-present → manifest-schema-readable → manifest-fresh: each
// edge is a real precondition, so an absent manifest SKIPS schema-readable and fresh
// rather than letting them judge an artifact that is not there.
// binary-schema-compatible depends on
// binary-on-path (no point asking a binary that is not there what schema it reads) and
// reads the manifest opportunistically, treating an absent vault as nothing to judge.
// precommit-anchor-live and grant-fresh both depend on git-repo: there are no .git/hooks
// to reason about and no `git status` to date a grant against outside a git tree, so each
// SKIPS (blocked-by git-repo) rather than reporting a dishonest verdict. grant-fresh names
// vault-discoverable first, so in a non-vault non-git dir it skips blocked-by the vault
// root (the deeper cause) rather than git.
// The installation-property hygiene nodes config-valid, ignore-correct, and
// tool-read-files-present each assert something init establishes (the one-directional
// init↔doctor symmetry) and hang off vault-discoverable, so with no vault they SKIP rather
// than judge files that are not there.
// (assertable-in "session,ci" and "any" both gate in both contexts and
// so both map to ctxAny; their distinction is rationale, carried in the comments.)
//
// hotPath (the orient hook's --session) defers the two corpus-scaling checks —
// manifest-fresh (a full in-buffer recompile) and live-fire (a temp-vault probe) — so
// the SessionStart hot path does not pay ADR-0032's "run all checks" cost on every
// startup/resume/compact. Both are leaf nodes (nothing depends on them), so deferring
// them skips nothing else; they run in full at the manual/CI cadence. The structural
// liveness checks (gate wired, binary on PATH, no legacy guard, anchor live) still run,
// so the LIVE/OFF headline is still emitted — only the deep chain proof is deferred.
func doctorNodes(repoRoot string, v vault.Vault, vaultErr error, hotPath bool) []checkNode {
	return []checkNode{
		{
			name: nodeVaultDiscoverable, class: classHygiene, assertableIn: ctxAny,
			run: func() []finding { return vaultDiscoverableFindings(vaultErr) },
		},
		{
			name: nodeGateCommitted, class: classLiveness, assertableIn: ctxAny, // session, ci
			run: func() []finding { return gateCommittedFindings(repoRoot) },
		},
		{
			name: nodeGateEffectiveLocal, class: classLiveness, assertableIn: ctxSession,
			run: func() []finding { return gateEffectiveLocalFindings(repoRoot) },
		},
		{
			name: nodePostwriteHook, class: classLiveness, assertableIn: ctxAny, // session, ci
			run: func() []finding { return postwriteFindings(repoRoot) },
		},
		{
			name: nodeNoLegacyBroadDeny, class: classLiveness, assertableIn: ctxAny, // session, ci
			run: func() []finding { return legacyFindings(repoRoot) },
		},
		{
			name: nodeBinaryOnPath, class: classLiveness, assertableIn: ctxSession,
			run: func() []finding { return binaryOnPathFindings() },
		},
		{
			name: nodeBinarySchemaCompat, class: classLiveness, assertableIn: ctxSession,
			preconditions: []string{nodeBinaryOnPath},
			run:           func() []finding { return binarySchemaCompatFindings(v, vaultErr) },
		},
		{
			name: nodeManifestPresent, class: classHygiene, assertableIn: ctxAny, // session, ci
			preconditions: []string{nodeVaultDiscoverable},
			run:           func() []finding { return manifestPresentFindings(v) },
		},
		{
			name: nodeManifestSchemaRead, class: classHygiene, assertableIn: ctxAny,
			preconditions: []string{nodeManifestPresent},
			run:           func() []finding { return manifestSchemaReadableFindings(v) },
		},
		{
			name: nodeManifestFresh, class: classHygiene, assertableIn: ctxAny, // session, ci
			preconditions: []string{nodeManifestSchemaRead},
			deferred:      hotPath, // full-corpus recompile — too costly for the session hot path
			run:           func() []finding { return manifestFreshFindings(v) },
		},
		{
			name: nodeLiveFire, class: classLiveness, assertableIn: ctxAny,
			deferred: hotPath, // temp-vault probe — deferred with manifest-fresh off the hot path
			run:      func() []finding { return liveFireFindings() },
		},
		{
			name: nodeGitRepo, class: classHygiene, assertableIn: ctxAny,
			run: func() []finding { return gitRepoFindings(repoRoot) },
		},
		{
			name: nodePrecommitAnchor, class: classLiveness, assertableIn: ctxAny, // session, ci
			preconditions: []string{nodeGitRepo},
			run:           func() []finding { return precommitAnchorFindings(repoRoot) },
		},
		{
			name: nodeGrantFresh, class: classLiveness, assertableIn: ctxSession,
			preconditions: []string{nodeVaultDiscoverable, nodeGitRepo},
			run:           func() []finding { return grantFreshFindings(v) },
		},
		{
			name: nodeConfigValid, class: classHygiene, assertableIn: ctxAny,
			preconditions: []string{nodeVaultDiscoverable},
			run:           func() []finding { return configValidFindings(v) },
		},
		{
			name: nodeIgnoreCorrect, class: classHygiene, assertableIn: ctxAny,
			preconditions: []string{nodeVaultDiscoverable},
			run:           func() []finding { return ignoreCorrectFindings(repoRoot, v) },
		},
		{
			name: nodeToolReadFiles, class: classHygiene, assertableIn: ctxSession,
			preconditions: []string{nodeVaultDiscoverable},
			run:           func() []finding { return toolReadFilesFindings(v) },
		},
	}
}

// printDoctorHeadline keeps the loud `vault write enforcement: LIVE / OFF` line as the
// liveness-class summary (ADR-0032 token-retrofit boundary). It reflects only
// liveness-class errors: a hygiene error (e.g. no vault) can still gate the exit code
// while enforcement itself is LIVE. This is the line the SessionStart orient hook
// projects when doctor exits clean — the hook simply INVOKES the engine, it is not a
// separate verb (ADR-0032: one engine, the cadences are assertable-in masks).
func printDoctorHeadline(stdout io.Writer, outcomes []checkOutcome, caveat string) {
	// A failed vault root is the dominant fact: run outside a project (no vault) or with
	// ambiguous markers, "enforcement OFF" is a look-alike of "no usable vault here", so
	// lead with the vault verdict and let the body carry the per-node detail rather than
	// blaring a cascade (ADR-0032 degenerate cases).
	if headline, blocking := vaultHeadline(outcomes); blocking {
		fmt.Fprintln(stdout, headline)
		return
	}
	if live, reason := livenessSummary(outcomes); !live {
		fmt.Fprintf(stdout, "vault write enforcement: OFF (%s)\n", reason)
		return
	}
	if caveat != "" {
		fmt.Fprintf(stdout, "vault write enforcement: LIVE (%s)\n", caveat)
		return
	}
	fmt.Fprintln(stdout, "vault write enforcement: LIVE")
}

// vaultHeadline returns the headline to lead with when the vault root could not be
// resolved, and whether that case applies. vault-absent (run outside a project) reports
// "no memento vault here"; ambiguous markers report the set-MEMENTO_VAULT_ROOT remediation.
// A resolved vault returns blocking=false, deferring to the LIVE/OFF liveness summary. The
// body still renders the vault-discoverable error and every skip(blocked-by) below this
// line, so leading the headline collapses the look-alike cascade without hiding detail.
func vaultHeadline(outcomes []checkOutcome) (headline string, blocking bool) {
	for _, o := range outcomes {
		if o.node.name != nodeVaultDiscoverable {
			continue
		}
		for _, f := range o.findings {
			switch f.token {
			case tokVaultAbsent:
				return "no memento vault here", true
			case tokVaultAmbiguous:
				return "ambiguous memento vault; " + f.remediation, true
			}
		}
		return "", false
	}
	return "", false
}

// livenessSummary reports whether enforcement is LIVE and, when not, the detail of the
// first liveness-class error in topo order. A skipped node is neither (it propagates a
// precondition failure, not an enforcement verdict).
func livenessSummary(outcomes []checkOutcome) (live bool, reason string) {
	for _, o := range outcomes {
		if o.skipped || o.node.class != classLiveness {
			continue
		}
		for _, f := range o.findings {
			if f.severity == sevError {
				return false, f.detail
			}
		}
	}
	return true, ""
}

// --- vault-discoverable --------------------------------------------------

// vaultDiscoverableFindings is the DAG root for vault-dependent checks. Ambiguity
// (multiple markers) and absence are distinct tokens; both are errors per the catalog.
func vaultDiscoverableFindings(vaultErr error) []finding {
	if vaultErr == nil {
		return okFindings()
	}
	if errors.Is(vaultErr, vault.ErrMultipleVaults) {
		return []finding{{token: tokVaultAmbiguous, severity: sevError,
			detail: vaultErr.Error(), remediation: "set MEMENTO_VAULT_ROOT"}}
	}
	return []finding{{token: tokVaultAbsent, severity: sevError,
		detail: vaultErr.Error(), remediation: "set MEMENTO_VAULT_ROOT"}}
}

// --- gate nodes ----------------------------------------------------------

// gateScan is one agent family's resolved gate state, scanned once and shared by the
// gate / postwrite / legacy nodes. covers reports whether a matcher covers the
// family's write tools.
type gateScan struct {
	family      string
	gate        *resolvedHook
	gateMatcher string
	post        *resolvedHook
	legacy      *resolvedHook
	covers      func(matcher string) (full, fileTools bool)
}

// wiredFamilies returns a scan for each agent family memento actually wired — its
// installed pre-write guard script is present. A bare .claude/ or .codex/ from another
// tool is not a memento-enforced family and is skipped, so it cannot force a phantom
// gate failure.
func wiredFamilies(repoRoot string) []gateScan {
	var fams []gateScan
	if fileExists(filepath.Join(repoRoot, ".claude", "memento-pre-write-vault-guard.sh")) {
		fams = append(fams, scanHooks(repoRoot, "claude", readClaudeHooks(repoRoot), claudeMatcherCovers))
	}
	if fileExists(filepath.Join(repoRoot, ".codex", "memento-pre-write-vault-guard.sh")) {
		fams = append(fams, scanHooks(repoRoot, "codex", readCodexHooks(repoRoot), codexMatcherCovers))
	}
	return fams
}

// scanHooks classifies a family's flattened hooks into its gate, post-hook, and any
// legacy broad-deny guard by reading the script each points at, not by trusting names.
func scanHooks(repoRoot, family string, hooks []wiredHook, covers func(matcher string) (full, fileTools bool)) gateScan {
	s := gateScan{family: family, covers: covers}
	for _, h := range hooks {
		rh := resolveHookCommand(repoRoot, h.command)
		switch {
		case rh.content != "" && isLegacyBroadDeny(rh.content):
			l := rh
			s.legacy = &l
		case rh.content != "" && strings.Contains(rh.content, "check-write"):
			if h.event == "PreToolUse" {
				g := rh
				s.gate = &g
				s.gateMatcher = h.matcher
			}
		case strings.Contains(filepath.Base(h.command), "pre-write-vault-guard"):
			// A memento gate by filename whose script does not resolve/read: record it
			// so gate-committed-config emits gate-unresolved rather than silently
			// reporting no gate at all.
			if h.event == "PreToolUse" && s.gate == nil {
				g := rh
				s.gate = &g
				s.gateMatcher = h.matcher
			}
		}
		if h.event == "PostToolUse" && rh.content != "" && strings.Contains(rh.content, "compile") {
			p := rh
			s.post = &p
		}
	}
	return s
}

// gateCommittedFindings is the gate-committed-config node: it reads the COMMITTED agent
// config (.claude/settings.json or the codex sentinel block), never the machine-local
// layer. No wired family at all is gate-missing — the loud "run memento init" signal.
func gateCommittedFindings(repoRoot string) []finding {
	fams := wiredFamilies(repoRoot)
	if len(fams) == 0 {
		return []finding{{token: tokGateMissing, severity: sevError,
			detail:      "no memento check-write gate wired for any agent under " + repoRoot,
			remediation: "memento init"}}
	}
	var fs []finding
	for _, f := range fams {
		fs = append(fs, gateCommittedFinding(f))
	}
	return fs
}

// gateCommittedFinding is one family's committed-gate verdict. A matcher that covers no
// note-write tool is gate-missing (the gate never fires on note writes — effectively
// absent), distinct from a matcher that covers file tools but not Bash, which is the
// warning-severity gate-matcher-partial. A command that does not resolve or is not
// executable is gate-unresolved.
func gateCommittedFinding(s gateScan) finding {
	if s.gate == nil {
		return finding{token: tokGateMissing, severity: sevError,
			detail: s.family + ": no PreToolUse check-write gate found", remediation: "memento init"}
	}
	if !s.gate.exists {
		return finding{token: tokGateUnresolved, severity: sevError,
			detail: s.family + " gate command " + s.gate.command + " does not resolve", remediation: "memento init"}
	}
	if !s.gate.executable {
		return finding{token: tokGateUnresolved, severity: sevError,
			detail: s.family + " gate command " + s.gate.command + " is not executable", remediation: "memento init"}
	}
	full, fileTools := s.covers(s.gateMatcher)
	if !fileTools {
		return finding{token: tokGateMissing, severity: sevError,
			detail:      fmt.Sprintf("%s gate matcher %q covers no note-write tool; the gate never fires on note writes", s.family, s.gateMatcher),
			remediation: "memento init"}
	}
	if !full {
		return finding{token: tokGateMatcherPartial, severity: sevWarning,
			detail:      fmt.Sprintf("%s gate matcher %q does not cover every write tool; some writes are ungated", s.family, s.gateMatcher),
			remediation: "memento init"}
	}
	return finding{severity: sevOK, detail: s.family + " gate live (matcher " + s.gateMatcher + ")"}
}

// gateEffectiveLocalFindings is the gate-effective-local node. Claude merges hooks across
// settings.json/settings.local.json ADDITIVELY: a local layer can only ADD hooks (and
// identical handlers dedup), never remove a committed one, so redefining a PreToolUse
// array in the local layer cannot drop the committed gate. The one local vector that does
// switch the gate off is disableAllHooks:true, which disables every hook including the
// committed gate. This node fires gate-locally-overridden when the committed gate is
// healthy but the effective local config sets disableAllHooks:true. session-only: CI
// cannot see, and does not own, the dev machine's local layer. codex has no machine-local
// layer, so there is nothing local can override.
func gateEffectiveLocalFindings(repoRoot string) []finding {
	if !fileExists(filepath.Join(repoRoot, ".claude", "memento-pre-write-vault-guard.sh")) {
		return okFindings()
	}
	committed := scanHooks(repoRoot, "claude", readClaudeHooks(repoRoot), claudeMatcherCovers)
	if gateHealthy(committed) && claudeHooksDisabled(repoRoot) {
		return []finding{{token: tokGateLocallyOverridden, severity: sevError,
			detail:      "machine-local Claude config sets disableAllHooks:true; it switches off the committed gate along with every other hook",
			remediation: "restore local config"}}
	}
	return okFindings()
}

// gateHealthy reports whether a scanned gate would actually fire on note writes: its
// command resolves and is executable and its matcher covers the file-write tools.
// Matcher partiality (Bash uncovered) is a degradation, not an absent gate, so it does
// not count as unhealthy here.
func gateHealthy(s gateScan) bool {
	if s.gate == nil || !s.gate.exists || !s.gate.executable {
		return false
	}
	_, fileTools := s.covers(s.gateMatcher)
	return fileTools
}

// postwriteFindings is the postwrite-hook-live node. A missing PostToolUse compile hook
// degrades the detective drift backstop but does not disable the preventive PreToolUse
// gate, so it is a warning, never an enforcement-OFF error.
func postwriteFindings(repoRoot string) []finding {
	fams := wiredFamilies(repoRoot)
	if len(fams) == 0 {
		// No wired family: gate-committed-config owns the loud gate-missing signal; the
		// post-write drift alarm is moot.
		return okFindings()
	}
	var fs []finding
	for _, f := range fams {
		fs = append(fs, postFinding(f))
	}
	return fs
}

func postFinding(s gateScan) finding {
	switch {
	case s.post == nil:
		return finding{token: tokPostwriteHookMissing, severity: sevWarning,
			detail: s.family + ": no PostToolUse compile hook found; the post-write drift alarm is not wired", remediation: "memento init"}
	case !s.post.exists:
		return finding{token: tokPostwriteHookMissing, severity: sevWarning,
			detail: s.family + " compile hook " + s.post.command + " does not resolve; drift alarm not wired", remediation: "memento init"}
	case !s.post.executable:
		return finding{token: tokPostwriteHookMissing, severity: sevWarning,
			detail: s.family + " compile hook " + s.post.command + " is not executable; drift alarm not wired", remediation: "memento init"}
	}
	return finding{severity: sevOK, detail: s.family + " compile hook live"}
}

// legacyFindings is the no-legacy-broad-deny node. A PreToolUse hook pointing at the
// pre-ADR-0031 broad-deny guard bricks the vault: it denies every vault write and there
// is no surviving `write` verb to satisfy it. Flag it as a hard error.
func legacyFindings(repoRoot string) []finding {
	var fs []finding
	for _, f := range wiredFamilies(repoRoot) {
		if f.legacy != nil {
			fs = append(fs, finding{token: tokLegacyBroadDenyWired, severity: sevError,
				detail:      f.family + ": " + f.legacy.command + " is a pre-ADR-0031 broad-deny guard; it bricks the vault (deny with no write verb)",
				remediation: "remove legacy guard"})
		}
	}
	if len(fs) == 0 {
		return okFindings()
	}
	return fs
}

// --- binary-on-path ------------------------------------------------------

// binaryOnPathFindings is the binary-on-path node. The gate shells to
// ${MEMENTO_BIN:-memento}; if that binary is not on PATH the gate fails closed and
// every vault write is blocked. session-only: CI's binary says nothing about the dev
// machine's. Schema compatibility of that same binary is binary-schema-compatible,
// which depends on this node.
func binaryOnPathFindings() []finding {
	bin := mementoBin()
	if _, err := exec.LookPath(bin); err != nil {
		return []finding{{token: tokBinaryNotOnPath, severity: sevError,
			detail:      fmt.Sprintf("the gate shells to %q, which is not on PATH; the gate would fail closed and block every vault write", bin),
			remediation: "install memento"}}
	}
	return okFindings()
}

// mementoBin is the binary the gate shells to: ${MEMENTO_BIN:-memento}. The
// binary-on-path and binary-schema-compatible nodes both reason about THIS binary, so
// they resolve it the one way here.
func mementoBin() string {
	if bin := os.Getenv("MEMENTO_BIN"); bin != "" {
		return bin
	}
	return "memento"
}

// --- binary-schema-compatible --------------------------------------------

// gateSchemaProbe asks the gate's binary which manifest schema it supports by shelling
// to `${bin} schema`. ok is false when the binary cannot be queried — an exec error or
// output that does not parse as an integer (e.g. a binary too old to know the verb).
// It is a package var so a test can substitute a binary reporting a schema other than
// doctor's own without compiling one, exercising the divergence the split exists to
// surface; the real exec path is covered against a freshly built binary.
var gateSchemaProbe = func(bin string) (schema int, ok bool) {
	out, err := exec.Command(bin, "schema").Output()
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, false
	}
	return n, true
}

// binarySchemaCompatFindings is the binary-schema-compatible node: the binary the gate
// shells to HERE cannot read the vault it guards, so it enforces nothing. It is keyed on
// the gate's resolved binary (${MEMENTO_BIN:-memento}) — distinct from
// manifest-schema-readable, which is keyed on doctor's own compiled-in schema; the two
// diverge whenever the gate's binary is not the one running doctor. session-only: CI's
// binary version says nothing about the dev machine's. With no resolved vault there is
// no manifest to be incompatible with (vault-discoverable owns that error), and a gate
// binary that does not report a schema is not judged on what cannot be determined.
func binarySchemaCompatFindings(v vault.Vault, vaultErr error) []finding {
	if vaultErr != nil {
		return okFindings()
	}
	manifestSchema, present, err := readManifestSchemaVersion(v.ManifestPath)
	if err != nil || !present {
		// An unreadable or absent manifest is manifest-schema-readable's / a future
		// manifest-present node's concern; this node only judges binary-vs-manifest
		// once a schema is readable.
		return okFindings()
	}
	bin := mementoBin()
	schema, ok := gateSchemaProbe(bin)
	if !ok {
		return okFindings()
	}
	if schema < manifestSchema {
		return []finding{{token: tokBinarySchemaTooOld, severity: sevError,
			detail:      fmt.Sprintf("the gate shells to %q, which supports manifest schema %d but the on-disk manifest is schema %d; that binary cannot read the vault it guards", bin, schema, manifestSchema),
			remediation: "upgrade memento"}}
	}
	return okFindings()
}

// --- manifest-present ----------------------------------------------------

// manifestPresentFindings is the manifest-present node: is there a compiled manifest on
// disk at all. It is the DAG root for the rest of the manifest chain
// (manifest-schema-readable and manifest-fresh depend on it), so an absent manifest SKIPS
// those rather than letting them judge a file that is not there. A never-compiled vault is
// a hygiene warning, not an enforcement-OFF error — it is one `memento compile` away from
// healthy, and the gate's loud signal is owned by gate-committed-config.
func manifestPresentFindings(v vault.Vault) []finding {
	if fileExists(v.ManifestPath) {
		return okFindings()
	}
	return []finding{{token: tokManifestNotFound, severity: sevWarning,
		detail: "no compiled manifest at " + v.ManifestPath, remediation: "memento compile"}}
}

// --- manifest-schema-readable --------------------------------------------

// manifestSchemaReadableFindings is the manifest-schema-readable node: can a memento
// binary at THIS schema (doctor's own compiled-in CurrentSchemaVersion) decode the
// committed on-disk manifest at all. A static fact about the artifact-vs-schema,
// independent of any gate and true even where none is wired — hygiene, any. Its
// manifest-present precondition guarantees the file exists, so the !present branch is
// defensive; a manifest whose schema exceeds this binary's, or one that will not decode,
// is unreadable.
func manifestSchemaReadableFindings(v vault.Vault) []finding {
	schema, present, err := readManifestSchemaVersion(v.ManifestPath)
	if err != nil {
		return []finding{{token: tokManifestSchemaUnread, severity: sevError,
			detail:      "the on-disk manifest cannot be decoded: " + err.Error(),
			remediation: "upgrade memento"}}
	}
	if !present {
		return okFindings()
	}
	if schema > manifest.CurrentSchemaVersion {
		return []finding{{token: tokManifestSchemaUnread, severity: sevError,
			detail:      fmt.Sprintf("the on-disk manifest is schema %d but this binary supports up to schema %d; it cannot decode the manifest", schema, manifest.CurrentSchemaVersion),
			remediation: "upgrade memento"}}
	}
	return okFindings()
}

// readManifestSchemaVersion reads just the schema_version off the on-disk manifest at
// manifestPath, raw rather than via manifest.Load (which rejects unsupported versions
// before doctor can report them). A missing file is present=false, not an error, so the
// absent-manifest case is the caller's to route; a read or JSON error is returned so the
// caller can distinguish "undecodable" from "too new".
func readManifestSchemaVersion(manifestPath string) (version int, present bool, err error) {
	data, err := os.ReadFile(manifestPath)
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

// --- manifest-fresh ------------------------------------------------------

// manifestFreshFindings is the manifest-fresh node: does the committed on-disk manifest
// match an AUTHORITATIVE in-buffer recompile of the vault. It recompiles to memory via
// manifest.Compile and diffs the CANONICAL DECODED PROJECTION both sides compute — the
// freshly compiled manifest round-tripped through Marshal→Decode against the on-disk
// Load — never raw bytes, so re-serialization, key-ordering, or whitespace differences
// can never raise a phantom manifest-stale. It MUST NOT write: a write would race the
// PostToolUse compile hook, and a diagnostic must not mutate what it diagnoses. This is
// brief/read's mtime freshness predicate (ADR-0033 / memento-7z4) at higher fidelity and
// cost; the cheap mtime heuristic is allowed to report fresh where this content diff
// reports stale (touch-without-change, clock skew). A hygiene warning, never an
// enforcement error. Its preconditions guarantee a present, schema-readable manifest, so a
// Compile or Load failure here is unexpected and reported as an undiagnosable warning
// rather than asserted as staleness.
func manifestFreshFindings(v vault.Vault) []finding {
	fresh, err := manifest.Compile(v)
	if err != nil {
		return []finding{{severity: sevWarning,
			detail: "could not recompile the vault to check manifest freshness: " + err.Error()}}
	}
	onDisk, err := manifest.Load(v)
	if err != nil {
		return []finding{{severity: sevWarning,
			detail: "could not load the on-disk manifest to check freshness: " + err.Error()}}
	}
	equal, err := manifestProjectionsEqual(fresh, onDisk)
	if err != nil {
		return []finding{{severity: sevWarning,
			detail: "could not canonicalise the recompiled manifest to check freshness: " + err.Error()}}
	}
	if !equal {
		return []finding{{token: tokManifestStale, severity: sevWarning,
			detail:      "the on-disk manifest does not match a recompile of the vault; a note changed without recompiling",
			remediation: "memento compile"}}
	}
	return okFindings()
}

// manifestProjectionMarshal canonicalises a freshly compiled manifest for comparison. It is
// a seam so the near-impossible marshal failure can be exercised by manifest-fresh's tests.
var manifestProjectionMarshal = manifest.Marshal

// manifestProjectionsEqual reports whether two manifests are equal as their canonical
// DECODED projection. The freshly compiled manifest is round-tripped through
// Marshal→Decode so it is compared in exactly the form the on-disk artifact was loaded in:
// this zeroes compile-only unexported state (OutLink.occurrence) and normalises
// omitempty / whitespace / key-ordering, so only a genuine content divergence trips the
// diff. A marshal/decode failure means the recompile cannot be canonicalised for
// comparison: an internal serialization failure is undiagnosable, not evidence of
// staleness, so it is returned to the caller rather than collapsed into "not equal".
func manifestProjectionsEqual(fresh, onDisk manifest.Manifest) (bool, error) {
	data, err := manifestProjectionMarshal(fresh)
	if err != nil {
		return false, err
	}
	proj, err := manifest.Decode(data)
	if err != nil {
		return false, err
	}
	return reflect.DeepEqual(proj, onDisk), nil
}

// --- live-fire -----------------------------------------------------------

// liveFireFindings is the live-fire node — the only check that proves the CHAIN rather
// than that parts exist. It synthesises a ratified read-only note in a throwaway vault
// and runs the same verdict chokepoint every in-vault write flows through, asserting
// the read-only overwrite is denied. Hermetic (a temp, non-git vault), so assertable
// anywhere a clone exists.
func liveFireFindings() []finding {
	denied, reasonCode, err := liveFireReadOnlyProbe()
	if err != nil {
		return []finding{{token: tokLiveFireNotDenied, severity: sevError,
			detail: "synthetic read-only probe failed: " + err.Error(), remediation: "upgrade / reinstall memento"}}
	}
	if !denied {
		return []finding{{token: tokLiveFireNotDenied, severity: sevError,
			detail:      "synthetic read-only overwrite returned " + reasonCode + ", not a read_only deny; the mode lattice is not enforcing",
			remediation: "upgrade / reinstall memento"}}
	}
	return okFindings()
}

// liveFireReadOnlyProbe builds a throwaway vault holding a ratified read-only note, then
// runs the live verdict engine (computeVaultWriteVerdict — the single chokepoint
// checkWriteFile funnels every in-vault file verdict through) against a Write that would
// rewrite it, returning whether the verdict denied. The temp vault is removed on return,
// so the probe leaves no residue. A non-git temp vault treats every note as ratified
// (the edit window only opens inside a git work tree), so the probe needs no commit and
// the real vault is never touched.
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

// --- git-repo & precommit-anchor-live ------------------------------------

// gitRepoFindings is the git-repo precondition node: is repoRoot inside a git work tree
// at all. It is the DAG root for the git-dependent checks (precommit-anchor-live here;
// grant-fresh reasons about git internally). No git is SUPPORTED, not an error — the
// ratification boundary simply does not exist to anchor against — so absence is a nudge
// that fails the precondition (skipping dependents) without gating the exit code or
// flipping the LIVE/OFF headline (hygiene class, never sevError).
func gitRepoFindings(repoRoot string) []finding {
	if _, err := gitOutput(repoRoot, "rev-parse", "--git-dir"); err != nil {
		return []finding{{severity: sevNudge,
			detail: "not a git repository; git-dependent checks are skipped"}}
	}
	return okFindings()
}

// precommit signals memento's pre-commit step is reachable by: the install sentinel that
// brackets the memento block, or either memento command init wires into it. Matching the
// behavior (a memento invocation is present), never the exact installed bytes, is the
// ADR-0032 "liveness ≠ presence" rule — composition (memento's step folded into a larger
// hook) must read as live, not as drift.
const (
	preCommitSentinel    = "# memento:start"
	preCommitCompileCmd  = "memento compile"
	preCommitClearGrants = "memento clear-grants"
)

// precommitAnchorFindings is the precommit-anchor-live node — the only liveness anchor for
// the ratification-boundary MODE VIOLATION commit audit (ADR-0031's integrity floor). The
// predicate is BEHAVIORAL, not byte-match: resolve the pre-commit hook git will ACTUALLY
// run (honoring core.hooksPath and third-party managers — husky, lefthook, pre-commit) and
// verify memento's step is reachable from it. Unreachable is the same dead audit however it
// arises, so it is always a not-live error; the two shapes differ only in remediation. A
// core.hooksPath redirect that bypasses an installed .git/hooks/pre-commit which DOES reach
// memento is precommit-shadowed (unset the redirect / compose the step). Otherwise the step
// is simply absent from the effective hook — never wired or deleted — which is
// precommit-anchor-missing (re-run init). "Liveness ≠ presence" cuts both ways: absent is
// not reachable, so a green verdict there would leave the integrity floor silently
// uncovered. Reachable-but-content-edited is deliberately NOT a finding — brittle
// script-identity matching is the stale-hook-detector trap the ADR rejects (it fails open on
// a working edit and closed on a shadowed byte-identical script), so identity is at most a
// nudge, never a gate, and we decline to reinvent it.
func precommitAnchorFindings(repoRoot string) []finding {
	gitDir, err := gitOutput(repoRoot, "rev-parse", "--git-dir")
	if err != nil {
		// git-repo guards this node; defensive only.
		return okFindings()
	}
	effRel, err := gitOutput(repoRoot, "rev-parse", "--git-path", "hooks/pre-commit")
	if err != nil {
		return okFindings()
	}
	effectiveHook := resolveGitPath(repoRoot, effRel)
	installedAnchor := resolveGitPath(repoRoot, filepath.Join(gitDir, "hooks", "pre-commit"))

	if precommitReachesMemento(repoRoot, effectiveHook) {
		return okFindings()
	}
	// memento's step is not reachable via the hook git runs, so the ratification-boundary
	// audit is dead either way. It is shadowing when a redirect bypasses an installed anchor
	// at the DEFAULT location that itself reaches memento; otherwise the step is just absent
	// from the effective hook (never wired, or deleted).
	if !sameHookPath(effectiveHook, installedAnchor) && hookFileReachesMemento(installedAnchor) {
		return []finding{{token: tokPrecommitShadowed, severity: sevError,
			detail:      fmt.Sprintf("git runs %s (core.hooksPath), which never reaches memento's pre-commit step, so the installed anchor at %s is dead and the ratification-boundary audit does not run", effectiveHook, installedAnchor),
			remediation: "unset core.hooksPath, or compose memento's step into the effective hook"}}
	}
	return []finding{{token: tokPrecommitMissing, severity: sevError,
		detail:      fmt.Sprintf("the pre-commit hook git runs (%s) never reaches memento's pre-commit step, so the ratification-boundary audit does not run", effectiveHook),
		remediation: "run memento init to install the pre-commit hook"}}
}

// gitOutput runs git in repoRoot and returns trimmed stdout, or an error (e.g. not a git
// tree). The git-repo / precommit-anchor nodes resolve paths the one way through here.
func gitOutput(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveGitPath makes a git-reported path absolute against repoRoot. git prints
// core.hooksPath / git-dir relative to the working directory it ran in (repoRoot here);
// an absolute core.hooksPath is returned unchanged.
func resolveGitPath(repoRoot, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(repoRoot, p))
}

// sameHookPath reports whether two resolved hook paths denote the same file.
func sameHookPath(a, b string) bool { return filepath.Clean(a) == filepath.Clean(b) }

// precommitReachesMemento reports whether memento's pre-commit step is reachable from the
// hook git runs, following the handoffs a real installation uses: shell sourcing and the
// third-party managers (husky's wrapper-into-.husky, lefthook's and pre-commit's
// config-driven dispatch). It is a bounded breadth-first walk over candidate files; an
// unreadable or absent file is simply a dead end, never an error (a diagnostic must not
// blow up on a missing include).
func precommitReachesMemento(repoRoot, startHook string) bool {
	visited := map[string]bool{}
	queue := []string{startHook}
	for len(queue) > 0 && len(visited) < 64 {
		path := filepath.Clean(queue[0])
		queue = queue[1:]
		if visited[path] {
			continue
		}
		visited[path] = true
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		if hookContentReachesMemento(content) {
			return true
		}
		queue = append(queue, hookDelegations(repoRoot, path, content)...)
	}
	return false
}

// hookFileReachesMemento reports whether one hook file directly carries memento's step (no
// handoff). It is the "is the installed anchor real" half of the shadowing test.
func hookFileReachesMemento(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return hookContentReachesMemento(string(data))
}

// hookContentReachesMemento reports whether a script body invokes memento's pre-commit step
// — by the install sentinel or either wired command.
func hookContentReachesMemento(content string) bool {
	return strings.Contains(content, preCommitSentinel) ||
		strings.Contains(content, preCommitCompileCmd) ||
		strings.Contains(content, preCommitClearGrants)
}

// hookDelegations returns the files a hook hands control to, so reachability can follow the
// chain a manager installs rather than only the entry script. It honors: husky (hooks live
// under a .husky dir; the wrapper git runs sources the user hook at <.husky>/pre-commit),
// explicit shell sourcing (. / source), and the config-file dispatch of lefthook and the
// pre-commit framework (their installed hook is a thin launcher; memento's step lives in
// the config). Candidates that do not exist are harmless dead ends in the walk.
func hookDelegations(repoRoot, hookPath, content string) []string {
	var out []string
	if husky := huskyUserHook(hookPath); husky != "" {
		out = append(out, husky)
	}
	for _, p := range sourcedPaths(content) {
		out = append(out, resolveGitPath(repoRoot, p), filepath.Join(filepath.Dir(hookPath), p))
	}
	if strings.Contains(content, "lefthook") {
		for _, c := range []string{"lefthook.yml", "lefthook.yaml", ".lefthook.yml", ".lefthook.yaml"} {
			out = append(out, filepath.Join(repoRoot, c))
		}
	}
	if strings.Contains(content, "pre-commit") { // pre-commit framework launcher
		for _, c := range []string{".pre-commit-config.yaml", ".pre-commit-config.yml"} {
			out = append(out, filepath.Join(repoRoot, c))
		}
	}
	return out
}

// huskyUserHook maps a husky wrapper path (anything under a .husky directory, e.g.
// .husky/_/pre-commit) to the user-authored hook husky sources, <.husky>/pre-commit.
// Empty when the path is not husky-managed.
func huskyUserHook(hookPath string) string {
	parts := strings.Split(filepath.ToSlash(hookPath), "/")
	for i, p := range parts {
		if p == ".husky" {
			return filepath.FromSlash(strings.Join(parts[:i+1], "/")) + string(filepath.Separator) + "pre-commit"
		}
	}
	return ""
}

// sourcedPaths extracts the operands of `. X` / `source X` lines so the walk can follow a
// composed hook into the script it sources. Quotes are stripped; an operand carrying a
// shell variable is unresolvable and dropped (it would only produce a junk dead-end path).
func sourcedPaths(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || (fields[0] != "." && fields[0] != "source") {
			continue
		}
		operand := strings.Trim(fields[1], `"'`)
		if operand == "" || strings.Contains(operand, "$") {
			continue
		}
		out = append(out, operand)
	}
	return out
}

// --- grant-fresh ---------------------------------------------------------

// grantFreshFindings is the grant-fresh node: an unlock grant whose key has no matching
// uncommitted edit is stale — the edit it thawed the note for already committed (and
// should have re-locked) or never happened. A hygiene warning, never an enforcement
// error. It depends on vault-discoverable AND git-repo (dating a grant against the working
// tree needs `git status`), so with no vault or no git it SKIPS rather than reporting a
// dishonest ok; the internal git-error guard below is the defensive backstop for a git
// failure the precondition did not catch.
func grantFreshFindings(v vault.Vault) []finding {
	grants, err := enforce.LoadGrants(v)
	if err != nil {
		return []finding{{severity: sevWarning, detail: "could not read unlock grants: " + err.Error()}}
	}
	if len(grants) == 0 {
		return okFindings()
	}
	var stale []string
	for key := range grants {
		edited, err := keyHasUncommittedEdit(v.Root, key)
		if err != nil {
			// Cannot prove staleness (no git, git error): do not warn on what we cannot
			// determine.
			continue
		}
		if !edited {
			stale = append(stale, key)
		}
	}
	if len(stale) == 0 {
		return okFindings()
	}
	sort.Strings(stale)
	return []finding{{token: tokGrantStale, severity: sevWarning,
		detail:      "stale grant(s) with no pending edit: " + strings.Join(stale, ", ") + "; commit or drop them",
		remediation: "commit or drop the grant"}}
}

// --- config-valid --------------------------------------------------------

// configFileName is the marker-dir-relative memento config. The canonical name is owned
// by internal/setup (setup.ConfigFileName); duplicated here like preCommitSentinel rather
// than imported, so doctor stays self-contained.
const configFileName = "config.toml"

// recognisedConfigKeys is the closed-world allowlist of top-level keys and table names
// memento understands in .memento/config.toml. It is empty today — the default config init
// writes is comment-only and no config key has been defined — so config-valid passes an
// empty or comment-only file and flags any declared key as unrecognised. Extend this set as
// config keys are introduced so doctor and the config reader stay in lockstep.
var recognisedConfigKeys = map[string]bool{}

// configValidFindings is the config-valid node: .memento/config.toml parses and every key
// it declares is recognised. An ABSENT config is vacuously valid — presence is init's job
// and there is no config-present node, so this node only judges a file that exists. A file
// that does not parse, or that declares a key outside the closed-world allowlist, is
// config-invalid: a hard error, because a vault whose config memento cannot understand is
// unusable. Depends on vault-discoverable for the marker dir.
func configValidFindings(v vault.Vault) []finding {
	path := filepath.Join(v.MarkerDir, configFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return okFindings()
		}
		return []finding{{token: tokConfigInvalid, severity: sevError,
			detail: "cannot read " + path + ": " + err.Error(), remediation: "fix .memento/config.toml"}}
	}
	keys, ok := scanConfigKeys(string(data))
	if !ok {
		// scanConfigKeys is a minimal hand-scanner, not a real TOML parser: ok=false
		// means "this scanner could not make sense of the file", which is NOT the same
		// as "this is definitively malformed TOML". Gating (exit 1) on a limited
		// scanner's confusion is the forward trap memento-tbu.4 fixed — so a parse
		// failure is a non-gating WARNING (it still surfaces, and gates under
		// MEMENTO_DOCTOR_STRICT), not a hard error. Only an unambiguous unrecognised
		// key below is a gating error. Revisit if a real TOML dependency is ever added.
		return []finding{{token: tokConfigInvalid, severity: sevWarning,
			detail:      path + " could not be parsed by memento's minimal config scanner",
			remediation: "fix .memento/config.toml"}}
	}
	var unknown []string
	for _, k := range keys {
		if !recognisedConfigKeys[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return []finding{{token: tokConfigInvalid, severity: sevError,
			detail:      path + " declares unrecognised key(s): " + strings.Join(unknown, ", "),
			remediation: "fix .memento/config.toml"}}
	}
	return okFindings()
}

// scanConfigKeys does a minimal, dependency-free scan of a TOML config: it returns the
// top-level keys and table names declared, and ok=false if a non-blank, non-comment line is
// neither a `[table]`/`[[array]]` header nor a `key = value` pair. memento has no TOML
// dependency (init writes config with string builders; readCodexHooks line-scans the codex
// block), so this matches that line-level discipline — sufficient because the recognised
// set is closed-world: a healthy config is comment-only and any declared key is judged
// against the allowlist.
//
// It is string- and continuation-aware (memento-tbu.4): scanLine strips line-ending `#`
// comments without tripping on `#` inside strings, and carries the state of a value that
// spans lines (an open `"""`/`”'` multi-line string or an unbalanced `[`/`{` array or
// inline table) so the lines that continue it are not mis-read as fresh statements. It is
// still not a full TOML parser; ok=false is treated by the caller as a non-gating warning,
// not proof of malformed input.
func scanConfigKeys(contents string) (keys []string, ok bool) {
	var (
		mlDelim string // open multi-line string delimiter (`"""` or `'''`), else ""
		depth   int    // unbalanced [/{ brackets carried from a value that spans lines
	)
	for _, raw := range strings.Split(contents, "\n") {
		// A line begun inside a multi-line string or an unbalanced bracket run is a
		// continuation of the previous statement, never a fresh one: advance the carry
		// state across it and move on.
		continuation := mlDelim != "" || depth > 0
		var code string
		code, mlDelim, depth = scanLine(raw, mlDelim, depth)
		if continuation {
			continue
		}
		trimmed := strings.TrimSpace(code)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			name, valid := tomlTableName(trimmed)
			if !valid {
				return nil, false
			}
			keys = append(keys, name)
			continue
		}
		key, _, found := strings.Cut(trimmed, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, false
		}
		keys = append(keys, strings.Trim(key, `"'`))
	}
	if mlDelim != "" || depth > 0 {
		return nil, false // an unterminated multi-line string or bracket run
	}
	return keys, true
}

// scanLine walks one physical line from the given carry state — an open multi-line string
// delimiter (`"""`/`”'`, else "") and the count of unbalanced `[`/`{` brackets — and
// returns the line's TOML "code" with any line-ending `#` comment removed, plus the carry
// state after the line. It tracks single-line `"`/`'` strings, triple-quoted multi-line
// strings, and `[`/`{` … `]`/`}` nesting so that a `#` inside a string is not read as a
// comment and a value spanning lines is reported as still-open. The returned code is used
// only to classify a fresh statement (table header vs `key = value`); a continuation line's
// code is discarded by the caller.
func scanLine(line, mlDelim string, depth int) (code, outDelim string, outDepth int) {
	var b strings.Builder
	n := len(line)
	i := 0
	// Consume the remainder of an already-open multi-line string first.
	if mlDelim != "" {
		idx := strings.Index(line, mlDelim)
		if idx < 0 {
			return "", mlDelim, depth // the whole line is inside the string
		}
		i = idx + len(mlDelim)
		mlDelim = ""
	}
	for i < n {
		switch {
		case strings.HasPrefix(line[i:], `"""`):
			rel := strings.Index(line[i+3:], `"""`)
			if rel < 0 {
				return b.String(), `"""`, depth
			}
			i += 3 + rel + 3
		case strings.HasPrefix(line[i:], `'''`):
			rel := strings.Index(line[i+3:], `'''`)
			if rel < 0 {
				return b.String(), `'''`, depth
			}
			i += 3 + rel + 3
		case line[i] == '#':
			return b.String(), "", depth // comment runs to end of line
		case line[i] == '"':
			j := i + 1
			for j < n {
				if line[j] == '\\' {
					j += 2
					continue
				}
				if line[j] == '"' {
					break
				}
				j++
			}
			if j > n {
				j = n
			} else if j < n {
				j++ // include the closing quote
			}
			b.WriteString(line[i:j])
			i = j
		case line[i] == '\'':
			j := i + 1
			for j < n && line[j] != '\'' {
				j++
			}
			if j < n {
				j++ // include the closing quote
			}
			b.WriteString(line[i:j])
			i = j
		case line[i] == '[' || line[i] == '{':
			depth++
			b.WriteByte(line[i])
			i++
		case line[i] == ']' || line[i] == '}':
			if depth > 0 {
				depth--
			}
			b.WriteByte(line[i])
			i++
		default:
			b.WriteByte(line[i])
			i++
		}
	}
	return b.String(), "", depth
}

// tomlTableName extracts the top-level table name from a `[table]` or
// `[[array.of.tables]]` header, returning valid=false when the brackets are unbalanced or
// the name is empty. The recognised-key allowlist is keyed on the first dotted segment (the
// top-level table).
func tomlTableName(line string) (name string, valid bool) {
	inner := line
	if strings.HasPrefix(inner, "[[") {
		if !strings.HasSuffix(inner, "]]") || len(inner) <= 4 {
			return "", false
		}
		inner = inner[2 : len(inner)-2]
	} else {
		if !strings.HasSuffix(inner, "]") || len(inner) <= 2 {
			return "", false
		}
		inner = inner[1 : len(inner)-1]
	}
	first, _, _ := strings.Cut(strings.TrimSpace(inner), ".")
	first = strings.Trim(strings.TrimSpace(first), `"'`)
	if first == "" {
		return "", false
	}
	return first, true
}

// --- ignore-correct ------------------------------------------------------

// memento .gitignore stanza sentinels — must match internal/setup's gitignoreStartSentinel
// /gitignoreEndSentinel (init owns the canonical strings). Duplicated here like
// preCommitSentinel rather than imported, so doctor stays self-contained.
const (
	gitignoreStartSentinel = "# memento:gitignore:start"
	gitignoreEndSentinel   = "# memento:gitignore:end"
)

// ignoreCorrectFindings is the ignore-correct node: the memento .gitignore stanza (at the
// repo root) and the vault's .mementoignore are present and well-formed. A missing stanza
// or .mementoignore leaks operational files (unlock grants, the pending-write ledger, the
// decision log, the generated brief) into version control — a hygiene warning, never an
// enforcement error, that one `memento init` re-establishes. Depends on vault-discoverable
// for the .mementoignore location.
func ignoreCorrectFindings(repoRoot string, v vault.Vault) []finding {
	var fs []finding
	if f, ok := gitignoreStanzaFinding(repoRoot); !ok {
		fs = append(fs, f)
	}
	if !fileExists(filepath.Join(v.Root, vault.IgnoreFileName)) {
		fs = append(fs, finding{token: tokGitignoreStanzaMissing, severity: sevWarning,
			detail:      "no " + vault.IgnoreFileName + " in the vault root " + v.Root,
			remediation: "memento init"})
	}
	if len(fs) == 0 {
		return okFindings()
	}
	return fs
}

// gitignoreStanzaFinding reports the memento .gitignore stanza's health. Absent (no
// .gitignore, or no sentinels) and malformed (an incomplete or duplicated sentinel block)
// both emit gitignore-stanza-missing — one token, distinct detail — mirroring init's own
// sentinel-block validation (insertOrReplaceSentinelBlock) so doctor flags exactly what a
// reinstall would repair. ok=true means the stanza is present and well-formed.
func gitignoreStanzaFinding(repoRoot string) (finding, bool) {
	path := filepath.Join(repoRoot, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil {
		return finding{token: tokGitignoreStanzaMissing, severity: sevWarning,
			detail:      "no memento stanza in .gitignore under " + repoRoot,
			remediation: "memento init"}, false
	}
	contents := string(data)
	start := strings.Index(contents, gitignoreStartSentinel)
	end := strings.Index(contents, gitignoreEndSentinel)
	startCount := strings.Count(contents, gitignoreStartSentinel)
	endCount := strings.Count(contents, gitignoreEndSentinel)
	switch {
	case start == -1 && end == -1:
		return finding{token: tokGitignoreStanzaMissing, severity: sevWarning,
			detail:      ".gitignore has no memento stanza",
			remediation: "memento init"}, false
	case start == -1 || end == -1 || end < start || startCount != 1 || endCount != 1:
		return finding{token: tokGitignoreStanzaMissing, severity: sevWarning,
			detail:      ".gitignore has a malformed memento stanza (incomplete or duplicated sentinels)",
			remediation: "memento init"}, false
	}
	return finding{}, true
}

// --- tool-read-files-present ---------------------------------------------

// toolReadFilesFindings is the tool-read-files-present node. init scaffolds the bootloader,
// the orient hook, and the convention templates; of those the ADR-0032 catalog defines
// exactly ONE reportable token — writing-md-absent — because ADR-0010 makes writing.md's
// PRESENCE doctor's business (init↔doctor symmetry) while its quality is not, and the
// discriminator forbids minting new tokens for the rest. So this node reports only whether
// _memento/conventions/writing.md exists, as a NUDGE: advisory, session-only, never gating.
// Depends on vault-discoverable for the _memento location.
func toolReadFilesFindings(v vault.Vault) []finding {
	path := filepath.Join(v.Root, vault.ToolDirName, convention.DirName, "writing.md")
	if fileExists(path) {
		return okFindings()
	}
	return []finding{{token: tokWritingMdAbsent, severity: sevNudge,
		detail:      "no writing convention at " + path + "; agents author vault notes without a writing guide",
		remediation: "author a writing convention"}}
}

// okFindings is the single passing finding a healthy node emits (empty token, sevOK).
func okFindings() []finding { return []finding{{severity: sevOK}} }

// --- hook resolution helpers ---------------------------------------------

// wiredHook is one agent-configured hook command, normalised across families: the
// Claude settings.json array entries and the codex config.toml `[[hooks.<Event>]]`
// tables both flatten to (event, matcher, command).
type wiredHook struct {
	event   string
	matcher string
	command string
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

// hookExecutable reports whether a hook script is runnable. Unix needs an execute bit;
// Windows has no such bit (runnability is by association), so existence is the honest
// answer there.
func hookExecutable(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// isLegacyBroadDeny recognises the pre-ADR-0031 broad-deny guard: a memento vault guard
// that denies writes and routes them through the now-removed `memento write` verb, and
// crucially predates `check-write`. The ADR-0031 dumb-pipe gate always shells to
// `check-write`, so its presence is the discriminator.
func isLegacyBroadDeny(content string) bool {
	if strings.Contains(content, "check-write") {
		return false
	}
	return strings.Contains(content, "memento") && strings.Contains(content, "permission_decision")
}

// claudeMatcherCovers reports whether a Claude matcher covers the write tools. full
// requires every tool init wires (Write|Edit|MultiEdit|Bash); fileTools requires at
// least the derivable file tools (Write/Edit/MultiEdit) — without those the gate does
// not see ordinary note writes at all.
func claudeMatcherCovers(matcher string) (full, fileTools bool) {
	has := func(tool string) bool { return matcherHasTool(matcher, tool) }
	fileTools = has("Write") && has("Edit") && has("MultiEdit")
	full = fileTools && has("Bash")
	return full, fileTools
}

// codexMatcherCovers reports whether a codex matcher covers its one hookable write tool,
// apply_patch (codex matches exact tool names; shell writes never fire the PreToolUse
// hook — ryr.39 — which is why the caveat rides every codex LIVE).
func codexMatcherCovers(matcher string) (full, fileTools bool) {
	covered := matcherHasTool(matcher, "apply_patch")
	return covered, covered
}

// matcherHasTool reports whether a `|`-separated matcher names tool exactly, matching
// how both Claude and codex split the matcher into tool names.
func matcherHasTool(matcher, tool string) bool {
	for _, part := range strings.Split(matcher, "|") {
		if strings.TrimSpace(part) == tool {
			return true
		}
	}
	return false
}

// parseClaudeHooks reads one settings file's hooks object grouped by event. A missing or
// malformed file yields no hooks rather than an error — doctor reports the gate missing,
// loudly, not aborts.
func parseClaudeHooks(path string) map[string][]wiredHook {
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
	out := map[string][]wiredHook{}
	for event, entries := range parsed.Hooks {
		for _, entry := range entries {
			for _, h := range entry.Hooks {
				out[event] = append(out[event], wiredHook{event: event, matcher: entry.Matcher, command: h.Command})
			}
		}
	}
	return out
}

func flattenEvents(m map[string][]wiredHook) []wiredHook {
	var out []wiredHook
	for _, hs := range m {
		out = append(out, hs...)
	}
	return out
}

// readClaudeHooks flattens the COMMITTED .claude/settings.json hooks.
func readClaudeHooks(repoRoot string) []wiredHook {
	return flattenEvents(parseClaudeHooks(filepath.Join(repoRoot, ".claude", "settings.json")))
}

// claudeHooksDisabled reports the EFFECTIVE disableAllHooks setting: the machine-local
// .claude/settings.local.json value wins, falling back to the committed
// .claude/settings.json (Claude merges this scalar local-over-committed). disableAllHooks
// is the only local vector that can switch off the committed gate, since hook arrays
// merge additively (see gateEffectiveLocalFindings).
func claudeHooksDisabled(repoRoot string) bool {
	if v, ok := parseDisableAllHooks(filepath.Join(repoRoot, ".claude", "settings.local.json")); ok {
		return v
	}
	v, _ := parseDisableAllHooks(filepath.Join(repoRoot, ".claude", "settings.json"))
	return v
}

// parseDisableAllHooks reads the disableAllHooks boolean from a Claude settings file. The
// present return distinguishes "explicitly set" from "absent or unreadable" so the local
// layer can override the committed one only when it actually declares the key.
func parseDisableAllHooks(path string) (val, present bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	var parsed struct {
		DisableAllHooks *bool `json:"disableAllHooks"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || parsed.DisableAllHooks == nil {
		return false, false
	}
	return *parsed.DisableAllHooks, true
}

// readCodexHooks flattens the memento sentinel block in .codex/config.toml. memento has
// no TOML dependency (init writes the block with string builders), so this is a line
// scan of the memento-owned block only: it tracks the current `[[hooks.<Event>]]` header
// and captures each `command`/`matcher` under it.
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

// tomlStringValue reads the basic-string value of a `key = "value"` line, undoing the
// backslash/quote escaping init's tomlBasicString applies. It is the minimal inverse the
// hook scan needs, not a TOML parser.
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

// keyHasUncommittedEdit reports whether the vault-relative key shows an uncommitted
// change in the working tree (modified, staged, or untracked). A non-git tree or a git
// error returns an error so the caller can decline to judge staleness rather than warn
// on what it cannot determine.
func keyHasUncommittedEdit(root, key string) (bool, error) {
	cmd := exec.Command("git", "--literal-pathspecs", "status", "--porcelain", "--", filepath.FromSlash(key))
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}
