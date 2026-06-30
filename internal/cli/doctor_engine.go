package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// This file is the ADR-0032 doctor check engine: the precondition-DAG core that
// every doctor check (liveness and hygiene alike) runs through. It ships with
// SYNTHETIC checks only — the real v1 liveness checks in doctor.go are retrofitted
// onto these nodes in dependent beads (memento-e3x.2 and friends). The engine owns
// four things the flat checkResult list could not express: the two orthogonal axes
// per check (class × assertable-in), skip as a first-class exit-neutral outcome, a
// node→one-to-many-token model, and a topo runner that never short-circuits
// unrelated branches. See "memento doctor — scope, the two-axis check model, and
// the diagnose-only contract" (ADR-0032).

// checkClass is the faculty-region axis of a doctor check: liveness is a property of
// THIS machine / this clone (is enforcement actually on here); hygiene is a property
// of the committed vault / installation (is the configuration well-formed). It is
// orthogonal to assertContext — the two genuinely cross (a liveness property can be
// CI-assertable; a hygiene property can be fine-when-absent on a fresh runner), which
// is why they are two axes and not one tag.
type checkClass int

const (
	classLiveness checkClass = iota
	classHygiene
)

func (c checkClass) String() string {
	if c == classHygiene {
		return "hygiene"
	}
	return "liveness"
}

// assertContext is an invocation context in which a check is permitted to GATE
// (contribute to the exit code). A check still runs and reports in every context;
// the mask governs only whether a failure counts against the caller's exit code
// there. With two contexts {session, ci}, the ADR-0032 catalog's "any" and
// "session, ci" coincide operationally — both gate in both contexts — and so both
// map to ctxAny here; their distinction is rationale (hermetic vs committed-repo
// state), carried in each node's class and comment, not in the mask.
type assertContext uint8

const (
	ctxSession assertContext = 1 << iota
	ctxCI
)

const ctxAny = ctxSession | ctxCI

// severity is a finding's outcome level. skip is deliberately NOT a severity: it is
// produced by the runner for a node whose precondition failed, never emitted by a
// check itself (see checkOutcome). ok < nudge < warning < error so the worst finding
// on a node is a max.
type severity int

const (
	sevOK severity = iota
	sevNudge
	sevWarning
	sevError
)

func (s severity) tag() string {
	switch s {
	case sevError:
		return "error"
	case sevWarning:
		return "warning"
	case sevNudge:
		return "nudge"
	default:
		return "ok"
	}
}

// finding is one outcome a check emits. A passing check emits a single sevOK finding
// with an empty token; a failing check emits one or more findings each carrying a
// stable token (the wire value a downstream consumer branches on — ADR-0032's
// node→token map). One node may emit one-to-many distinct tokens (the gate node
// alone emits gate-missing / gate-unresolved / gate-matcher-partial).
type finding struct {
	token       string
	severity    severity
	detail      string
	remediation string
}

// checkNode is one node in the doctor DAG. It carries the two orthogonal axes
// (class, assertableIn), its preconditions (names of nodes that must PASS before it
// runs), and run, which evaluates the node and returns its findings. Real checks
// close run over the repo root / vault they read; synthetic checks close over a
// fixed result.
type checkNode struct {
	name          string
	class         checkClass
	assertableIn  assertContext
	preconditions []string
	run           func() []finding
}

// checkOutcome is the result of evaluating one node: either it ran (findings set) or
// it was skipped because a precondition did not pass (skipped set, blockedBy naming
// the precondition). skip is exit-neutral but NOT green — the v1 verb dishonestly
// reported skipped checks as ok; this models the third state explicitly.
type checkOutcome struct {
	node      checkNode
	skipped   bool
	blockedBy string
	findings  []finding
}

// passed reports whether this node satisfied its asserted property, which is the
// precondition predicate for its dependents. A skipped node never passes (the skip
// propagates). A node that ran passes only if every finding is sevOK — any failing
// token, at ANY severity, means the property does not hold, so a warning-severity
// precondition failure (e.g. manifest-not-found) still skips its dependents.
func (o checkOutcome) passed() bool {
	if o.skipped {
		return false
	}
	for _, f := range o.findings {
		if f.severity != sevOK {
			return false
		}
	}
	return true
}

// statusTag is the rendered status of the outcome: skip, or the worst finding's tag.
func (o checkOutcome) statusTag() string {
	if o.skipped {
		return "skip"
	}
	worst := sevOK
	for _, f := range o.findings {
		if f.severity > worst {
			worst = f.severity
		}
	}
	return worst.tag()
}

// runChecks topologically orders the DAG and runs every node whose preconditions all
// passed, emitting a skip(blocked-by) outcome for any node gated by a failed
// precondition. It NEVER short-circuits: a failing node skips only its own
// descendants, leaving sibling branches to run. The returned outcomes are in topo
// order. An error means the DAG is malformed (duplicate node, unknown precondition,
// or a cycle) — a programming error in the node set, not a vault condition.
func runChecks(nodes []checkNode) ([]checkOutcome, error) {
	order, err := topoOrder(nodes)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]checkNode, len(nodes))
	for _, n := range nodes {
		byName[n.name] = n
	}

	outcomes := make(map[string]checkOutcome, len(nodes))
	results := make([]checkOutcome, 0, len(nodes))
	for _, name := range order {
		n := byName[name]
		blocker := ""
		for _, p := range n.preconditions {
			if !outcomes[p].passed() {
				blocker = p
				break
			}
		}
		var o checkOutcome
		if blocker != "" {
			o = checkOutcome{node: n, skipped: true, blockedBy: blocker}
		} else {
			o = checkOutcome{node: n, findings: n.run()}
		}
		outcomes[name] = o
		results = append(results, o)
	}
	return results, nil
}

// topoOrder returns the node names in a precondition-before-dependent order using
// Kahn's algorithm, processing ready nodes in definition order for a deterministic
// result. It rejects duplicate names, unknown preconditions, and cycles.
func topoOrder(nodes []checkNode) ([]string, error) {
	indeg := make(map[string]int, len(nodes))
	adj := make(map[string][]string, len(nodes))
	known := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if known[n.name] {
			return nil, fmt.Errorf("duplicate check node %q", n.name)
		}
		known[n.name] = true
		indeg[n.name] = 0
	}
	for _, n := range nodes {
		for _, p := range n.preconditions {
			if !known[p] {
				return nil, fmt.Errorf("check %q has unknown precondition %q", n.name, p)
			}
			adj[p] = append(adj[p], n.name)
			indeg[n.name]++
		}
	}

	var queue []string
	for _, n := range nodes {
		if indeg[n.name] == 0 {
			queue = append(queue, n.name)
		}
	}
	order := make([]string, 0, len(nodes))
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		order = append(order, name)
		for _, m := range adj[name] {
			indeg[m]--
			if indeg[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	if len(order) != len(nodes) {
		return nil, fmt.Errorf("check DAG has a cycle")
	}
	return order, nil
}

// gates reports whether a finding contributes to the exit code in ctx. A finding
// gates only when its node is assertable in ctx (the [[doctor-scoping]] cadence
// constraint expressed mechanically): an error always gates; a warning gates only
// under strict (the MEMENTO_DOCTOR_STRICT opt-in mirroring MEMENTO_STRICT_COMMIT);
// nudge and ok never gate.
func (f finding) gates(nodeAssertable, ctx assertContext, strict bool) bool {
	if nodeAssertable&ctx == 0 {
		return false
	}
	switch f.severity {
	case sevError:
		return true
	case sevWarning:
		return strict
	default:
		return false
	}
}

// computeExitCode implements the gating half of the ADR-0032 exit contract: 0 when
// no finding gates in ctx, 1 when at least one does. Skipped outcomes are
// exit-neutral. The 2 (usage) code is owned by runDoctor's flag parsing, never by a
// severity tier, so it is not produced here.
func computeExitCode(outcomes []checkOutcome, ctx assertContext, strict bool) int {
	for _, o := range outcomes {
		if o.skipped {
			continue
		}
		for _, f := range o.findings {
			if f.gates(o.node.assertableIn, ctx, strict) {
				return 1
			}
		}
	}
	return 0
}

// renderEngineReport prints each outcome one per line. A skip renders as not-green
// with its blocking precondition; a passed node renders ok; a failing node renders
// each non-ok finding with its severity tag, token, and remediation.
func renderEngineReport(w io.Writer, outcomes []checkOutcome) {
	for _, o := range outcomes {
		switch {
		case o.skipped:
			fmt.Fprintf(w, "  [skip] %s: blocked-by %s\n", o.node.name, o.blockedBy)
		case o.passed():
			fmt.Fprintf(w, "  [ok] %s\n", o.node.name)
		default:
			for _, f := range o.findings {
				if f.severity == sevOK {
					continue
				}
				fmt.Fprintf(w, "  [%s] %s: %s (%s) — %s\n",
					f.severity.tag(), o.node.name, f.detail, f.token, f.remediation)
			}
		}
	}
}

const doctorStrictEnv = "MEMENTO_DOCTOR_STRICT"

// doctorStrict reports whether warnings should gate, parsed exactly like
// MEMENTO_STRICT_COMMIT (compile.go) — the same detection-default / mitigation-opt-in
// idiom, reused rather than reinvented as a second policy surface.
func doctorStrict() bool {
	return envFlagEnabled(doctorStrictEnv)
}

// detectContext picks the invocation context for gating. A truthy CI env var means
// CI (committed-repo state is all that is assertable there); otherwise session. The
// ADR leaves widening this via a flag to a later bead.
func detectContext() assertContext {
	if envFlagEnabled("CI") {
		return ctxCI
	}
	return ctxSession
}

// envFlagEnabled reports whether an environment variable is set to a truthy value:
// any value other than empty, "0", "false", "no", or "off" (case-insensitive). This
// is the shared parser behind MEMENTO_STRICT_COMMIT and MEMENTO_DOCTOR_STRICT, so the
// two strict surfaces cannot drift.
func envFlagEnabled(name string) bool {
	v, ok := os.LookupEnv(name)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
