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

func TestCompileWithin1s(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile performance gate under -short")
	}

	root := makeSyntheticCompileVault(t, syntheticCompileDocCount)

	start := time.Now()
	stdout, stderr, code := runSyntheticCompile(root)
	elapsed := time.Since(start)

	if code != 0 {
		t.Fatalf("memento compile exit code = %d, want 0; stdout = %q; stderr = %q", code, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("memento compile stdout = %q, want empty", stdout)
	}
	if stderr != "" {
		t.Fatalf("memento compile stderr = %q, want empty", stderr)
	}
	if elapsed >= time.Second {
		t.Fatalf("memento compile for %d synthetic docs took %s, want < 1s", syntheticCompileDocCount, elapsed)
	}
	t.Logf("memento compile for %d synthetic docs took %s", syntheticCompileDocCount, elapsed)
}

func BenchmarkCompile500Docs(b *testing.B) {
	root := makeSyntheticCompileVault(b, syntheticCompileDocCount)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stdout, stderr, code := runSyntheticCompile(root)
		if code != 0 {
			b.Fatalf("memento compile exit code = %d, want 0; stdout = %q; stderr = %q", code, stdout, stderr)
		}
		if stdout != "" {
			b.Fatalf("memento compile stdout = %q, want empty", stdout)
		}
		if stderr != "" {
			b.Fatalf("memento compile stderr = %q, want empty", stderr)
		}
	}
}

func runSyntheticCompile(root string) (stdout string, stderr string, code int) {
	var stdoutBuf, stderrBuf bytes.Buffer
	code = Run([]string{"compile", "--dir", root}, &stdoutBuf, &stderrBuf)
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
