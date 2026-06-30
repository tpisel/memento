package cli

import (
	"bytes"
	"strings"
	"testing"
)

// These tests exercise the ADR-0032 doctor engine over SYNTHETIC DAGs only. The real
// v1 liveness checks are retrofitted onto these nodes in dependent beads
// (memento-e3x.2+); here we prove the engine's contract independent of any check.

// recordingNode builds a synthetic node whose run records its name into order (to
// prove topo ordering) and returns the given findings.
func recordingNode(name string, class checkClass, ctx assertContext, pre []string, order *[]string, findings ...finding) checkNode {
	return checkNode{
		name:          name,
		class:         class,
		assertableIn:  ctx,
		preconditions: pre,
		run: func() []finding {
			*order = append(*order, name)
			if len(findings) == 0 {
				return []finding{{severity: sevOK}}
			}
			return findings
		},
	}
}

func okFinding() finding { return finding{severity: sevOK} }
func errFinding(token string) finding {
	return finding{token: token, severity: sevError, detail: "boom", remediation: "fix it"}
}
func warnFinding(token string) finding {
	return finding{token: token, severity: sevWarning, detail: "degraded", remediation: "tidy it"}
}

func outcomeFor(t *testing.T, outcomes []checkOutcome, name string) checkOutcome {
	t.Helper()
	for _, o := range outcomes {
		if o.node.name == name {
			return o
		}
	}
	t.Fatalf("no outcome for node %q", name)
	return checkOutcome{}
}

// TestTopoOrderRunsPreconditionsFirst proves preconditions run before dependents.
func TestTopoOrderRunsPreconditionsFirst(t *testing.T) {
	var order []string
	nodes := []checkNode{
		recordingNode("c", classHygiene, ctxAny, []string{"b"}, &order),
		recordingNode("b", classHygiene, ctxAny, []string{"a"}, &order),
		recordingNode("a", classHygiene, ctxAny, nil, &order),
	}
	outcomes, err := runChecks(nodes)
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}
	if got := strings.Join(order, ","); got != "a,b,c" {
		t.Fatalf("run order = %q, want a,b,c", got)
	}
	for _, name := range []string{"a", "b", "c"} {
		if !outcomeFor(t, outcomes, name).passed() {
			t.Fatalf("node %q should have passed", name)
		}
	}
}

// TestFailedPreconditionSkipsOnlyDescendants proves a failed precondition skips its
// descendants while sibling branches still run — the no-short-circuit guarantee.
func TestFailedPreconditionSkipsOnlyDescendants(t *testing.T) {
	var order []string
	nodes := []checkNode{
		recordingNode("root", classLiveness, ctxAny, nil, &order, errFinding("root-broke")),
		recordingNode("dependent", classLiveness, ctxAny, []string{"root"}, &order),
		recordingNode("sibling", classHygiene, ctxAny, nil, &order),
		recordingNode("sibling-child", classHygiene, ctxAny, []string{"sibling"}, &order),
	}
	outcomes, err := runChecks(nodes)
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}

	dep := outcomeFor(t, outcomes, "dependent")
	if !dep.skipped || dep.blockedBy != "root" {
		t.Fatalf("dependent should skip blocked-by root, got skipped=%v blockedBy=%q", dep.skipped, dep.blockedBy)
	}
	for _, name := range []string{"sibling", "sibling-child"} {
		if o := outcomeFor(t, outcomes, name); o.skipped || !o.passed() {
			t.Fatalf("sibling branch node %q should have run and passed, got skipped=%v", name, o.skipped)
		}
	}
	if contains(order, "dependent") {
		t.Fatalf("dependent's run should never have executed; order=%v", order)
	}
	if !contains(order, "sibling") || !contains(order, "sibling-child") {
		t.Fatalf("sibling branch should have executed; order=%v", order)
	}
}

// TestSkipPropagatesTransitively proves a skip cascades to a skipped node's own
// descendants, each blocked-by its immediate precondition.
func TestSkipPropagatesTransitively(t *testing.T) {
	var order []string
	nodes := []checkNode{
		recordingNode("root", classHygiene, ctxAny, nil, &order, errFinding("root-broke")),
		recordingNode("mid", classHygiene, ctxAny, []string{"root"}, &order),
		recordingNode("leaf", classHygiene, ctxAny, []string{"mid"}, &order),
	}
	outcomes, err := runChecks(nodes)
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}
	if o := outcomeFor(t, outcomes, "mid"); !o.skipped || o.blockedBy != "root" {
		t.Fatalf("mid should skip blocked-by root, got %+v", o)
	}
	if o := outcomeFor(t, outcomes, "leaf"); !o.skipped || o.blockedBy != "mid" {
		t.Fatalf("leaf should skip blocked-by mid, got %+v", o)
	}
}

// TestNodeEmitsMultipleTokens proves one node can emit multiple distinct tokens.
func TestNodeEmitsMultipleTokens(t *testing.T) {
	node := checkNode{
		name:         "multi",
		class:        classLiveness,
		assertableIn: ctxAny,
		run: func() []finding {
			return []finding{warnFinding("token-a"), warnFinding("token-b")}
		},
	}
	outcomes, err := runChecks([]checkNode{node})
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}
	o := outcomeFor(t, outcomes, "multi")
	if len(o.findings) != 2 {
		t.Fatalf("want 2 findings, got %d", len(o.findings))
	}
	if o.findings[0].token != "token-a" || o.findings[1].token != "token-b" {
		t.Fatalf("distinct tokens not preserved: %+v", o.findings)
	}
}

// TestSkipIsNotGreenAndExitNeutral proves a skip renders as not-green and does not
// contribute to the exit code.
func TestSkipIsNotGreenAndExitNeutral(t *testing.T) {
	var order []string
	nodes := []checkNode{
		recordingNode("root", classHygiene, ctxAny, nil, &order, errFinding("root-broke")),
		recordingNode("dependent", classHygiene, ctxAny, []string{"root"}, &order),
	}
	outcomes, err := runChecks(nodes)
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}

	dep := outcomeFor(t, outcomes, "dependent")
	if dep.statusTag() != "skip" {
		t.Fatalf("skip status tag = %q, want skip", dep.statusTag())
	}
	if dep.passed() {
		t.Fatal("a skipped node must not count as passed (not green)")
	}

	var buf bytes.Buffer
	renderEngineReport(&buf, []checkOutcome{dep})
	line := buf.String()
	if strings.Contains(line, "[ok]") {
		t.Fatalf("skip rendered green: %q", line)
	}
	if !strings.Contains(line, "[skip]") || !strings.Contains(line, "blocked-by root") {
		t.Fatalf("skip line missing skip tag or blocked-by: %q", line)
	}

	// The skip itself contributes nothing to the exit code (the gating root does).
	if code := computeExitCode([]checkOutcome{dep}, ctxSession, false); code != 0 {
		t.Fatalf("a lone skip should be exit-neutral, got %d", code)
	}
}

// TestDeferredNodeIsNotRun proves a deferred node is skipped without running, renders as
// deferred (not blocked-by), is exit-neutral, and leaves sibling branches untouched — the
// cost escape hatch the SessionStart hot path needs.
func TestDeferredNodeIsNotRun(t *testing.T) {
	var order []string
	costly := recordingNode("costly", classHygiene, ctxAny, nil, &order, errFinding("would-fail"))
	costly.deferred = true
	nodes := []checkNode{
		costly,
		recordingNode("sibling", classLiveness, ctxAny, nil, &order),
	}
	outcomes, err := runChecks(nodes)
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}

	if contains(order, "costly") {
		t.Fatalf("a deferred node's run must never execute; order=%v", order)
	}
	o := outcomeFor(t, outcomes, "costly")
	if !o.skipped || !o.deferred {
		t.Fatalf("costly outcome = %+v, want skipped+deferred", o)
	}
	if o.blockedBy != "" {
		t.Fatalf("a deferred skip names no precondition, got blockedBy=%q", o.blockedBy)
	}
	if o.passed() {
		t.Fatal("a deferred node must not count as passed (not green)")
	}
	if o.statusTag() != "skip" {
		t.Fatalf("deferred status tag = %q, want skip", o.statusTag())
	}

	var buf bytes.Buffer
	renderEngineReport(&buf, []checkOutcome{o})
	line := buf.String()
	if !strings.Contains(line, "[skip] costly: deferred") || strings.Contains(line, "blocked-by") {
		t.Fatalf("deferred render = %q, want a deferred skip line with no blocked-by", line)
	}

	// Even though the deferred node WOULD have emitted an error, deferral runs nothing,
	// so it gates nowhere; the sibling still ran.
	if code := computeExitCode(outcomes, ctxSession, true); code != 0 {
		t.Fatalf("a deferred node must be exit-neutral even under strict, got %d", code)
	}
	if !contains(order, "sibling") {
		t.Fatalf("sibling should still run alongside a deferred node; order=%v", order)
	}
}

// --- exit contract -------------------------------------------------------

func runForExit(t *testing.T, nodes []checkNode) []checkOutcome {
	t.Helper()
	outcomes, err := runChecks(nodes)
	if err != nil {
		t.Fatalf("runChecks: %v", err)
	}
	return outcomes
}

func TestExitCleanIsZero(t *testing.T) {
	out := runForExit(t, []checkNode{
		{name: "a", class: classLiveness, assertableIn: ctxAny, run: func() []finding { return []finding{okFinding()} }},
	})
	if code := computeExitCode(out, ctxSession, false); code != 0 {
		t.Fatalf("clean DAG exit = %d, want 0", code)
	}
}

func TestExitErrorGates(t *testing.T) {
	out := runForExit(t, []checkNode{
		{name: "a", class: classLiveness, assertableIn: ctxAny, run: func() []finding { return []finding{errFinding("broke")} }},
	})
	if code := computeExitCode(out, ctxSession, false); code != 1 {
		t.Fatalf("error-severity node exit = %d, want 1", code)
	}
}

func TestExitWarningGatesOnlyUnderStrict(t *testing.T) {
	out := runForExit(t, []checkNode{
		{name: "a", class: classHygiene, assertableIn: ctxAny, run: func() []finding { return []finding{warnFinding("degraded")} }},
	})
	if code := computeExitCode(out, ctxSession, false); code != 0 {
		t.Fatalf("warning exit (default) = %d, want 0", code)
	}
	if code := computeExitCode(out, ctxSession, true); code != 1 {
		t.Fatalf("warning exit (strict) = %d, want 1", code)
	}
}

func TestExitNudgeNeverGates(t *testing.T) {
	out := runForExit(t, []checkNode{
		{name: "a", class: classHygiene, assertableIn: ctxAny, run: func() []finding {
			return []finding{{token: "advisory", severity: sevNudge, detail: "fyi"}}
		}},
	})
	if code := computeExitCode(out, ctxSession, false); code != 0 {
		t.Fatalf("nudge exit (default) = %d, want 0", code)
	}
	if code := computeExitCode(out, ctxSession, true); code != 0 {
		t.Fatalf("nudge must never gate, even under strict; got %d", code)
	}
}

func TestSessionOnlyFindingDoesNotGateInCI(t *testing.T) {
	out := runForExit(t, []checkNode{
		{name: "session-only", class: classLiveness, assertableIn: ctxSession, run: func() []finding {
			return []finding{errFinding("local-only")}
		}},
	})
	if code := computeExitCode(out, ctxCI, false); code != 0 {
		t.Fatalf("session-only error must not gate in ci, got %d", code)
	}
	if code := computeExitCode(out, ctxSession, false); code != 1 {
		t.Fatalf("session-only error must gate in session, got %d", code)
	}
}

// --- malformed DAGs ------------------------------------------------------

func TestRunChecksRejectsUnknownPrecondition(t *testing.T) {
	_, err := runChecks([]checkNode{
		{name: "a", preconditions: []string{"ghost"}, run: func() []finding { return nil }},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown precondition") {
		t.Fatalf("want unknown-precondition error, got %v", err)
	}
}

func TestRunChecksRejectsCycle(t *testing.T) {
	_, err := runChecks([]checkNode{
		{name: "a", preconditions: []string{"b"}, run: func() []finding { return nil }},
		{name: "b", preconditions: []string{"a"}, run: func() []finding { return nil }},
	})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}

func TestRunChecksRejectsDuplicate(t *testing.T) {
	_, err := runChecks([]checkNode{
		{name: "a", run: func() []finding { return nil }},
		{name: "a", run: func() []finding { return nil }},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("want duplicate error, got %v", err)
	}
}

// --- strict / context parsing -------------------------------------------

func TestDoctorStrictParsesLikeStrictCommit(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"0":     false,
		"false": false,
		"NO":    false,
		"off":   false,
		"1":     true,
		"true":  true,
		"yes":   true,
	}
	for val, want := range cases {
		t.Setenv(doctorStrictEnv, val)
		if got := doctorStrict(); got != want {
			t.Fatalf("doctorStrict with %q = %v, want %v", val, got, want)
		}
		// The shared parser must agree with the commit-strict surface.
		t.Setenv(strictCommitEnv, val)
		if strictCommit() != want {
			t.Fatalf("strictCommit with %q disagrees with doctorStrict", val)
		}
	}
}

func TestDetectContext(t *testing.T) {
	t.Setenv("CI", "true")
	if detectContext() != ctxCI {
		t.Fatal("CI=true should detect ctxCI")
	}
	t.Setenv("CI", "")
	if detectContext() != ctxSession {
		t.Fatal("empty CI should detect ctxSession")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
