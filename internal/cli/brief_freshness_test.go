package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
