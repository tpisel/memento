package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

func runCLIBrief(t *testing.T) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	code = Run([]string{"brief"}, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
}

// manifestModTime returns the on-disk mtime of the compiled manifest.
func manifestModTime(t *testing.T, root string) time.Time {
	t.Helper()
	info, err := os.Stat(filepath.Join(root, vault.MarkerDirName, vault.ManifestFileName))
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	return info.ModTime()
}

// touchNote sets a note's mtime to the given time so freshness comparisons are
// deterministic rather than dependent on wall-clock ordering within the test.
func touchNote(t *testing.T, root, rel string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(filepath.Join(root, filepath.FromSlash(rel)), when, when); err != nil {
		t.Fatalf("chtimes %q: %v", rel, err)
	}
}

// TestBriefRecompilesWhenNoteNewerThanManifest: an out-of-band edit (no agent
// tool write, so the PostToolUse hook never fired) leaves the note newer than the
// manifest; brief must recompile so its projection reflects the edit.
func TestBriefRecompilesWhenNoteNewerThanManifest(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "---\nsummary: Original summary text.\n---\n# Note\n\nBody.\n")

	if _, code := runCLICompile(t); code != 0 {
		t.Fatalf("initial compile exit = %d", code)
	}
	manifestMtime := manifestModTime(t, root)

	// Edit out of band, then mark the note newer than the manifest.
	writeCLIFile(t, root, "note.md", "---\nsummary: Edited out of band summary.\n---\n# Note\n\nBody changed.\n")
	touchNote(t, root, "note.md", manifestMtime.Add(2*time.Second))

	stdout, stderr, code := runCLIBrief(t)
	if code != 0 {
		t.Fatalf("brief exit = %d; stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Edited out of band summary.") {
		t.Fatalf("brief stdout did not reflect the out-of-band edit:\n%s", stdout)
	}
	if strings.Contains(stdout, "Original summary text.") {
		t.Fatalf("brief stdout still shows the stale summary:\n%s", stdout)
	}
	if !manifestModTime(t, root).After(manifestMtime) {
		t.Fatalf("manifest mtime did not advance; expected a recompile")
	}
}

// TestBriefNoOpWhenManifestUpToDate: when every note is older than the manifest,
// brief must serve the existing artifacts without rewriting them — the stat gate
// short-circuits and writeCompileArtifacts never runs.
func TestBriefNoOpWhenManifestUpToDate(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "---\nsummary: Stable summary.\n---\n# Note\n\nBody.\n")

	if _, code := runCLICompile(t); code != 0 {
		t.Fatalf("initial compile exit = %d", code)
	}
	manifestMtime := manifestModTime(t, root)
	// Force the note strictly older than the manifest so the gate sees it as fresh.
	touchNote(t, root, "note.md", manifestMtime.Add(-10*time.Second))

	stdout, stderr, code := runCLIBrief(t)
	if code != 0 {
		t.Fatalf("brief exit = %d; stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Stable summary.") {
		t.Fatalf("brief stdout missing expected content:\n%s", stdout)
	}
	if got := manifestModTime(t, root); !got.Equal(manifestMtime) {
		t.Fatalf("manifest mtime changed from %v to %v; expected a stat-only no-op", manifestMtime, got)
	}
}

// TestBriefLazyRecompileSkipsAudits: the lazy recompile is pure coherence work. An
// out-of-band edit that breaks a ratified read-only note's mode is reflected in
// the refreshed brief, but the DRIFT ALARM / MODE VIOLATION audits stay off the
// read path — they belong to explicit compile, PostToolUse, and pre-commit so the
// integrity signals are never silently absorbed by a read-side verb.
func TestBriefLazyRecompileSkipsAudits(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	ratifyNote(t, root, "frozen.md", "---\nmode: read-only\nsummary: Frozen original.\n---\n# Frozen\n\nOriginal.\n")

	if _, code := runCLICompile(t); code != 0 {
		t.Fatalf("initial compile exit = %d", code)
	}
	manifestMtime := manifestModTime(t, root)

	// Tamper with the ratified read-only note directly on disk (ungated), then mark
	// it newer than the manifest so brief's gate recompiles.
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\nsummary: Tampered summary.\n---\n# Frozen\n\nTampered.\n")
	touchNote(t, root, "frozen.md", manifestMtime.Add(2*time.Second))

	stdout, stderr, code := runCLIBrief(t)
	if code != 0 {
		t.Fatalf("brief exit = %d; stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Tampered summary.") {
		t.Fatalf("brief did not recompile to reflect the edit:\n%s", stdout)
	}
	if strings.Contains(stderr, "MODE VIOLATION") {
		t.Fatalf("brief lazy recompile must not raise a MODE VIOLATION; stderr = %q", stderr)
	}
	if strings.Contains(stderr, "DRIFT ALARM") {
		t.Fatalf("brief lazy recompile must not raise a DRIFT ALARM; stderr = %q", stderr)
	}
}

// TestWriteCompileArtifactsSurfacesBriefWriteError: a failure writing the brief
// artifact must propagate as an error rather than be swallowed as a nil-error
// warning. Otherwise the lazy-compile path on 'brief' would go on to serve the
// stale on-disk brief with no signal (memento-tbu.8). The error is forced by
// turning the brief artifact into a directory so the atomic rename onto it fails.
func TestWriteCompileArtifactsSurfacesBriefWriteError(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "---\nsummary: A summary.\n---\n# Note\n\nBody.\n")
	if _, code := runCLICompile(t); code != 0 {
		t.Fatalf("initial compile exit = %d", code)
	}

	v, err := vault.Open(root)
	if err != nil {
		t.Fatalf("vault.Open() error = %v, want nil", err)
	}
	briefPath := vault.BriefPath(v)
	if err := os.Remove(briefPath); err != nil {
		t.Fatalf("remove brief artifact: %v", err)
	}
	if err := os.Mkdir(briefPath, 0o755); err != nil {
		t.Fatalf("replace brief artifact with directory: %v", err)
	}

	if _, _, err := writeCompileArtifacts(v); err == nil {
		t.Fatal("writeCompileArtifacts() error = nil; brief write failure must surface, not be swallowed")
	}
}

// TestWriteCompileArtifactsConcurrentWritesStayWhole: parallel recompiles racing
// on the same manifest/brief artifact (the lazy-compile path two 'brief' calls can
// hit at once) must never leave a corrupt or partial file. The atomic write-temp +
// rename means each artifact lands whole; this asserts every invocation succeeds,
// the final artifacts parse as the complete projection, and no temp debris is left
// behind (memento-tbu.8).
func TestWriteCompileArtifactsConcurrentWritesStayWhole(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "---\nsummary: Concurrent summary.\n---\n# Note\n\nBody.\n")
	if _, code := runCLICompile(t); code != 0 {
		t.Fatalf("initial compile exit = %d", code)
	}
	v, err := vault.Open(root)
	if err != nil {
		t.Fatalf("vault.Open() error = %v, want nil", err)
	}

	const workers = 16
	var wg sync.WaitGroup
	errs := make([]error, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			_, _, errs[i] = writeCompileArtifacts(v)
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("worker %d writeCompileArtifacts() error = %v, want nil", i, e)
		}
	}

	// The manifest must still load and the brief must render the full projection —
	// a torn write would fail to parse or drop the entry.
	if _, err := manifest.Load(v); err != nil {
		t.Fatalf("manifest.Load after concurrent writes: %v", err)
	}
	briefData, err := os.ReadFile(vault.BriefPath(v))
	if err != nil {
		t.Fatalf("read brief after concurrent writes: %v", err)
	}
	if !strings.Contains(string(briefData), "Concurrent summary.") {
		t.Fatalf("brief is incomplete after concurrent writes:\n%s", briefData)
	}

	// No temp files should survive a clean run.
	entries, err := os.ReadDir(filepath.Join(root, vault.ToolDirName))
	if err != nil {
		t.Fatalf("read tool dir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("leftover temp file in tool dir: %s", e.Name())
		}
	}
}
