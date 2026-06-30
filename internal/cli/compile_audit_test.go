package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/tpisel/memento/internal/enforce"
)

// ratifyNote commits a note so it is ratified at HEAD, the baseline the mode
// audit diffs against.
func ratifyNote(t *testing.T, root, rel, content string) {
	t.Helper()
	writeCLIFile(t, root, rel, content)
	commitCLIGit(t, root)
}

// TestCompileModeViolationReadOnlyEditedOnDisk: a read-only note edited directly
// on disk (no gate, simulating an ungated codex/opaque-shell write) is caught by
// compile as a MODE VIOLATION. Default posture is detection — exit 0.
func TestCompileModeViolationReadOnlyEditedOnDisk(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	ratifyNote(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")

	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nTampered.\n")

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0 (detection default); stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "MODE VIOLATION") || !strings.Contains(stderr, "frozen.md") {
		t.Fatalf("stderr = %q, want a MODE VIOLATION naming frozen.md", stderr)
	}
}

// TestCompileNoModeViolationLivingEdited: a living note edited the same ungated
// way is not a violation.
func TestCompileNoModeViolationLivingEdited(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	ratifyNote(t, root, "live.md", "---\nmode: living\n---\n# Live\n\nOriginal.\n")

	writeCLIFile(t, root, "live.md", "---\nmode: living\n---\n# Live\n\nRewritten freely.\n")

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.Contains(stderr, "MODE VIOLATION") {
		t.Fatalf("stderr = %q, want no MODE VIOLATION for a living note edit", stderr)
	}
}

// TestCompileNoModeViolationAppendOnlyAppend: a pure append to an append-only
// note keeps HEAD as a prefix and is not a violation.
func TestCompileNoModeViolationAppendOnlyAppend(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	ratifyNote(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\n")

	writeCLIFile(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\nEntry two.\n")

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.Contains(stderr, "MODE VIOLATION") {
		t.Fatalf("stderr = %q, want no MODE VIOLATION for a pure append", stderr)
	}
}

// TestCompileModeViolationAppendOnlyInteriorRewrite: an append-only note whose
// existing content is rewritten breaks the prefix invariant and is caught.
func TestCompileModeViolationAppendOnlyInteriorRewrite(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	ratifyNote(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\n")

	writeCLIFile(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry CHANGED.\n")

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "MODE VIOLATION") || !strings.Contains(stderr, "log.md") {
		t.Fatalf("stderr = %q, want a MODE VIOLATION naming log.md", stderr)
	}
}

// TestCompileNoModeViolationGrantCovered: a read-only edit covered by an active
// unlock grant (the user's recorded authorisation) is not flagged.
func TestCompileNoModeViolationGrantCovered(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	ratifyNote(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")

	if err := enforce.AddGrant(vaultFor(root), "frozen.md", "authorised fix", time.Now()); err != nil {
		t.Fatalf("add grant: %v", err)
	}
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nEdited under grant.\n")

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.Contains(stderr, "MODE VIOLATION") {
		t.Fatalf("stderr = %q, want no MODE VIOLATION when an active grant covers the change", stderr)
	}
}

// TestCompileNoModeViolationBrandNewNote: a brand-new read-only note (untracked,
// absent at HEAD) is birth, not a violation.
func TestCompileNoModeViolationBrandNewNote(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	ratifyNote(t, root, "seed.md", "---\nmode: living\n---\n# Seed\n\nSeed.\n")

	writeCLIFile(t, root, "born.md", "---\nmode: read-only\n---\n# Born\n\nFresh content.\n")

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.Contains(stderr, "MODE VIOLATION") {
		t.Fatalf("stderr = %q, want no MODE VIOLATION for a brand-new note", stderr)
	}
}

// TestCompileNoModeViolationWriteModeChange: a legitimate write-mode change (only
// the mode: line differs from HEAD) is not flagged.
func TestCompileNoModeViolationWriteModeChange(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	ratifyNote(t, root, "n.md", "---\nmode: read-only\n---\n# N\n\nBody.\n")

	writeCLIFile(t, root, "n.md", "---\nmode: living\n---\n# N\n\nBody.\n")

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.Contains(stderr, "MODE VIOLATION") {
		t.Fatalf("stderr = %q, want no MODE VIOLATION for a pure write-mode change", stderr)
	}
}

// TestCompileStrictCommitBlocks: with MEMENTO_STRICT_COMMIT set, a violation
// flips compile to a non-zero exit so the pre-commit hook (set -eu) blocks the
// commit, while still emitting the alarm.
func TestCompileStrictCommitBlocks(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	ratifyNote(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nTampered.\n")

	t.Setenv("MEMENTO_STRICT_COMMIT", "1")
	stderr, code := runCLICompile(t)
	if code == 0 {
		t.Fatalf("compile exit = 0, want non-zero under MEMENTO_STRICT_COMMIT; stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "MODE VIOLATION") {
		t.Fatalf("stderr = %q, want the MODE VIOLATION alarm even when blocking", stderr)
	}
}

// TestCompileMalformedFrontmatterLoudButZero: a note with malformed frontmatter
// no longer compiles green-with-a-quiet-warning. Default posture is detection —
// a loud MALFORMED FRONTMATTER alarm naming the consequence, exit 0 (memento-o0a).
func TestCompileMalformedFrontmatterLoudButZero(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "broken.md", "---\nmode: living\ntitle\n---\n# Broken\n\nBody.\n")

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0 (detection default); stderr = %q", code, stderr)
	}
	for _, want := range []string{"MALFORMED FRONTMATTER", "broken.md", "held read-only"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q, want it to contain %q", stderr, want)
		}
	}
}

// TestCompileMalformedFrontmatterStrictCommitBlocks: under MEMENTO_STRICT_COMMIT a
// malformed-frontmatter note flips compile to a non-zero exit so the pre-commit
// hook holds the silently-locked note out of ratified state (memento-o0a).
func TestCompileMalformedFrontmatterStrictCommitBlocks(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "broken.md", "---\nmode: living\ntitle\n---\n# Broken\n\nBody.\n")

	t.Setenv("MEMENTO_STRICT_COMMIT", "1")
	stderr, code := runCLICompile(t)
	if code == 0 {
		t.Fatalf("compile exit = 0, want non-zero under MEMENTO_STRICT_COMMIT; stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "MALFORMED FRONTMATTER") {
		t.Fatalf("stderr = %q, want the MALFORMED FRONTMATTER alarm even when blocking", stderr)
	}
}

// TestCompileStrictCommitCleanStaysZero: strict mode must not block a compile
// with no violations.
func TestCompileStrictCommitCleanStaysZero(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	ratifyNote(t, root, "live.md", "---\nmode: living\n---\n# Live\n\nOriginal.\n")
	writeCLIFile(t, root, "live.md", "---\nmode: living\n---\n# Live\n\nEdited.\n")

	t.Setenv("MEMENTO_STRICT_COMMIT", "1")
	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0 with no violations even under strict; stderr = %q", code, stderr)
	}
}
