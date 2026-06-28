package cli

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const syntheticCompileDocCount = 500

// perWriteCompileDocCount approximates a real project-memory vault — the size a
// PostToolUse compile actually re-runs on every native write. The 500-doc gate
// below still guards the worst case, but the per-write hot path lives here.
const perWriteCompileDocCount = 50

// perWriteCompileBudget bounds one PostToolUse compile invocation, process cold
// start included. The median on dev hardware is tens of ms, so this ceiling is
// generous CI headroom that still trips on an order-of-magnitude regression.
const perWriteCompileBudget = 500 * time.Millisecond

// TestCompilePerWriteLatency gates a single per-write compile over a realistic
// vault using the prebuilt binary, so the budget reflects what a PostToolUse hook
// pays per write — process cold start plus a full recompile — not an in-process
// Run call. The 500-doc TestCompileWithin1s gate remains the worst-case ceiling.
func TestCompilePerWriteLatency(t *testing.T) {
	mementoBinary(t) // skip early under -short / build failure, before vault setup

	root := makeSyntheticCompileVault(t, perWriteCompileDocCount)

	samples := make([]time.Duration, latencyGateSamples)
	for i := range samples {
		elapsed, stdout, stderr, code := timeMementoRun(t, root, "", "compile")
		if code != 0 {
			t.Fatalf("memento compile exit code = %d, want 0; stdout = %q; stderr = %q", code, stdout, stderr)
		}
		if want := compiledStatusLine(perWriteCompileDocCount); stderr != want {
			t.Fatalf("memento compile stderr = %q, want %q", stderr, want)
		}
		samples[i] = elapsed
	}

	median := medianDuration(samples)
	t.Logf("per-write compile (%d docs) median over %d runs: %s (samples: %v)", perWriteCompileDocCount, latencyGateSamples, median, samples)
	if median >= perWriteCompileBudget {
		t.Fatalf("per-write compile median %s exceeds budget %s (samples: %v)", median, perWriteCompileBudget, samples)
	}
}

// worstCaseCompileRuns is the best-of-N sample count for the 500-doc ceiling. A
// throughput ceiling asks "is compile capable of <1s for 500 docs?", so the best
// run is the honest statistic: it is immune to the CPU contention of a parallel
// `go test ./...` (which once flaked a single in-process sample over the 1s line)
// while a genuine regression still inflates even the least-contended run.
const worstCaseCompileRuns = 3

func TestCompileWithin1s(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile performance gate under -short")
	}

	root := makeSyntheticCompileVault(t, syntheticCompileDocCount)

	best := time.Duration(1<<63 - 1)
	for i := 0; i < worstCaseCompileRuns; i++ {
		start := time.Now()
		stdout, stderr, code := runSyntheticCompile(t, root)
		elapsed := time.Since(start)

		if code != 0 {
			t.Fatalf("memento compile exit code = %d, want 0; stdout = %q; stderr = %q", code, stdout, stderr)
		}
		if stdout != "" {
			t.Fatalf("memento compile stdout = %q, want empty", stdout)
		}
		if want := compiledStatusLine(syntheticCompileDocCount); stderr != want {
			t.Fatalf("memento compile stderr = %q, want %q", stderr, want)
		}
		if elapsed < best {
			best = elapsed
		}
	}

	if best >= time.Second {
		t.Fatalf("memento compile for %d synthetic docs took %s (best of %d), want < 1s", syntheticCompileDocCount, best, worstCaseCompileRuns)
	}
	t.Logf("memento compile for %d synthetic docs took %s (best of %d)", syntheticCompileDocCount, best, worstCaseCompileRuns)
}

func BenchmarkCompile500Docs(b *testing.B) {
	root := makeSyntheticCompileVault(b, syntheticCompileDocCount)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stdout, stderr, code := runSyntheticCompile(b, root)
		if code != 0 {
			b.Fatalf("memento compile exit code = %d, want 0; stdout = %q; stderr = %q", code, stdout, stderr)
		}
		if stdout != "" {
			b.Fatalf("memento compile stdout = %q, want empty", stdout)
		}
		if want := compiledStatusLine(syntheticCompileDocCount); stderr != want {
			b.Fatalf("memento compile stderr = %q, want %q", stderr, want)
		}
	}
}

func runSyntheticCompile(tb testing.TB, root string) (stdout string, stderr string, code int) {
	tb.Helper()

	previous, err := os.Getwd()
	if err != nil {
		return "", fmt.Sprintf("getwd: %v", err), 1
	}
	if err := os.Chdir(root); err != nil {
		return "", fmt.Sprintf("chdir %q: %v", root, err), 1
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			tb.Fatalf("restore cwd: %v", err)
		}
	}()

	var stdoutBuf, stderrBuf bytes.Buffer
	code = Run([]string{"compile"}, &stdoutBuf, &stderrBuf)
	return stdoutBuf.String(), stderrBuf.String(), code
}

func makeSyntheticCompileVault(tb testing.TB, docs int) string {
	tb.Helper()

	root := tb.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".memento"), 0o755); err != nil {
		tb.Fatalf("mkdir marker: %v", err)
	}

	keys := make([]string, docs)
	for i := 0; i < docs; i++ {
		keys[i] = fmt.Sprintf("area-%d/doc-%03d.md", i%5, i)
	}

	rng := rand.New(rand.NewSource(20260614))
	for i, key := range keys {
		writeSyntheticCompileDoc(tb, root, key, i, keys, rng)
	}
	return root
}

func writeSyntheticCompileDoc(tb testing.TB, root, key string, index int, keys []string, rng *rand.Rand) {
	tb.Helper()

	var b strings.Builder
	title := fmt.Sprintf("Synthetic Doc %03d", index)
	if index%3 != 0 {
		mode := "append-only"
		if index%11 == 0 {
			mode = "read-only"
		}
		fmt.Fprintf(&b, "---\ntitle: %s\nsummary: Synthetic summary for document %03d.\ntags: [synthetic, area-%d]\nmode: %s\nupdated: 2026-06-%02d\n---\n\n", title, index, index%5, mode, (index%28)+1)
	} else {
		fmt.Fprintf(&b, "# %s\n\nSynthetic summary for document %03d.\n\n", title, index)
	}

	headingCount := 3 + rng.Intn(3)
	for section := 0; section < headingCount; section++ {
		level := "##"
		if section%2 == 1 {
			level = "###"
		}
		fmt.Fprintf(&b, "%s Section %d\n\n", level, section+1)
		fmt.Fprintf(&b, "This deterministic paragraph exercises markdown parsing for document %03d section %d.", index, section+1)
		if rng.Intn(10) == 0 {
			target := strings.TrimSuffix(keys[rng.Intn(len(keys))], ".md")
			fmt.Fprintf(&b, " Related note: [[%s]].", target)
		}
		b.WriteString("\n\n")
	}

	path := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir parent for %q: %v", key, err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		tb.Fatalf("write %q: %v", key, err)
	}
}
