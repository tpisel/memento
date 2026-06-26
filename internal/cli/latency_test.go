package cli

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// mementoBinaryPath is the freshly built memento binary used by the latency
// gates. It is populated once by TestMain (skipped under -short) so the gates
// measure a real per-invocation cost — process cold start plus any shells the
// verb spawns — rather than an in-process Run call that hides both.
var mementoBinaryPath string

func TestMain(m *testing.M) {
	flag.Parse() // testing.Short reads the -test.short flag, which must be parsed first
	if !testing.Short() {
		dir, err := os.MkdirTemp("", "memento-latency-bin")
		if err == nil {
			bin := filepath.Join(dir, "memento")
			build := exec.Command("go", "build", "-o", bin, "github.com/tpisel/memento/cmd/memento")
			if out, berr := build.CombinedOutput(); berr == nil {
				mementoBinaryPath = bin
			} else {
				// Leave mementoBinaryPath empty; the gates skip and report why.
				_, _ = os.Stderr.WriteString("latency gate: build memento binary failed: " + berr.Error() + "\n" + string(out))
			}
			defer os.RemoveAll(dir)
		}
	}
	os.Exit(m.Run())
}

// mementoBinary returns the prebuilt binary path or skips the calling test when
// it is unavailable (-short, or a build failure already logged by TestMain).
func mementoBinary(tb testing.TB) string {
	tb.Helper()
	if mementoBinaryPath == "" {
		tb.Skip("memento binary unavailable (skipping latency gate; -short or build failure)")
	}
	return mementoBinaryPath
}

// timeMementoRun runs the prebuilt binary once with cwd=dir, feeds stdin, and
// returns the wall-clock duration plus captured streams. Each call is a fresh
// process, so the duration includes process cold start.
func timeMementoRun(tb testing.TB, dir, stdin string, args ...string) (time.Duration, string, string, int) {
	tb.Helper()
	cmd := exec.Command(mementoBinary(tb), args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			tb.Fatalf("run memento %v: %v", args, err)
		}
	}
	return elapsed, stdout.String(), stderr.String(), code
}

// medianDuration returns the median of a copy of the samples (samples must be
// non-empty). The median tolerates the first-call cold disk-cache outlier and
// scheduler noise that a single timing or a max would let flap CI.
func medianDuration(samples []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

// latencyGateSamples is the per-gate invocation count: enough that the median is
// stable against a cold first run without making the suite slow.
const latencyGateSamples = 9

// checkWriteVerdictBudget bounds a single check-write verdict. The hot-path cost
// is process cold start plus one `git ls-files` ratification shell per call; the
// median on dev hardware is tens of ms, so this ceiling is generous CI headroom
// that still trips on an order-of-magnitude regression (e.g. a full-vault walk or
// repeated git shells).
const checkWriteVerdictBudget = 500 * time.Millisecond

// TestCheckWriteVerdictLatency gates a single check-write verdict against a
// ratified read-only note: the deny path that forces both the on-disk mode read
// and the per-call `git ls-files` ratification shell, so the timing reflects the
// real PreToolUse hot path rather than a cheap inert exit.
func TestCheckWriteVerdictLatency(t *testing.T) {
	mementoBinary(t) // skip early under -short / build failure, before vault setup

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".memento"), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	initCLIGit(t, root)
	commitCLIGit(t, root)

	payload := checkWritePayload(t,
		"Write",
		filepath.Join(root, "frozen.md"),
		"---\nmode: read-only\n---\n# Frozen\n\nRewritten.\n")

	samples := make([]time.Duration, latencyGateSamples)
	for i := range samples {
		elapsed, stdout, stderr, code := timeMementoRun(t, root, payload, "check-write")
		if code != 0 {
			t.Fatalf("check-write exit code = %d, want 0; stdout = %q; stderr = %q", code, stdout, stderr)
		}
		// A read-only rewrite of a ratified note must be denied; assert it so a
		// silently inert verdict (which would skip the ratification shell and
		// understate the cost) fails the gate instead of passing cheaply.
		if !strings.Contains(stdout, `"permissionDecision":"deny"`) {
			t.Fatalf("check-write verdict did not deny the read-only rewrite; stdout = %q", stdout)
		}
		samples[i] = elapsed
	}

	median := medianDuration(samples)
	t.Logf("check-write verdict median over %d runs: %s (samples: %v)", latencyGateSamples, median, samples)
	if median >= checkWriteVerdictBudget {
		t.Fatalf("check-write verdict median %s exceeds budget %s (samples: %v)", median, checkWriteVerdictBudget, samples)
	}
}
