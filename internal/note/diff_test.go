package note

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tpisel/memento/internal/vault"
)

func makeNoteVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, vault.MarkerDirName), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	return root
}

func noteVault(root string) vault.Vault {
	return vault.Vault{Root: root, MarkerDir: filepath.Join(root, vault.MarkerDirName)}
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	run(t, root, "init")
}

func gitCommitAll(t *testing.T, root string) {
	t.Helper()
	run(t, root, "add", ".")
	run(t, root,
		"-c", "user.name=Memento Test",
		"-c", "user.email=memento-test@example.invalid",
		"commit", "--no-gpg-sign", "-m", "snapshot",
	)
}

func run(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeNoteFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// TestChangedNotesVsHeadListsModifiedNotUntracked: the diff lists tracked files
// changed against HEAD but excludes untracked (brand-new) files — the carve-out
// the mode audit relies on.
func TestChangedNotesVsHeadListsModifiedNotUntracked(t *testing.T) {
	root := makeNoteVault(t)
	v := noteVault(root)
	gitInit(t, root)
	writeNoteFile(t, root, "a.md", "# A\nOriginal.\n")
	writeNoteFile(t, root, "b.md", "# B\nKept.\n")
	gitCommitAll(t, root)

	writeNoteFile(t, root, "a.md", "# A\nChanged.\n") // modified, tracked
	writeNoteFile(t, root, "fresh.md", "# Fresh\n")   // untracked, brand-new

	changed, err := ChangedNotesVsHead(v)
	if err != nil {
		t.Fatalf("ChangedNotesVsHead error = %v", err)
	}
	got := map[string]bool{}
	for _, k := range changed {
		got[k] = true
	}
	if !got["a.md"] {
		t.Fatalf("changed = %v, want a.md (modified tracked file)", changed)
	}
	if got["fresh.md"] {
		t.Fatalf("changed = %v, want fresh.md excluded (untracked, brand-new)", changed)
	}
	if got["b.md"] {
		t.Fatalf("changed = %v, want b.md excluded (unchanged)", changed)
	}
}

// TestChangedNotesVsHeadNonGitTree: a vault outside git has no ratified state, so
// the diff is empty rather than an error.
func TestChangedNotesVsHeadNonGitTree(t *testing.T) {
	v := noteVault(makeNoteVault(t))
	changed, err := ChangedNotesVsHead(v)
	if err != nil {
		t.Fatalf("ChangedNotesVsHead error = %v, want nil for a non-git tree", err)
	}
	if len(changed) != 0 {
		t.Fatalf("changed = %v, want empty for a non-git tree", changed)
	}
}

// TestChangedNotesVsHeadUnbornHead: a git tree with no commit yet has no HEAD;
// nothing is ratified, so the diff is empty rather than an error.
func TestChangedNotesVsHeadUnbornHead(t *testing.T) {
	root := makeNoteVault(t)
	v := noteVault(root)
	gitInit(t, root)
	writeNoteFile(t, root, "a.md", "# A\n")

	changed, err := ChangedNotesVsHead(v)
	if err != nil {
		t.Fatalf("ChangedNotesVsHead error = %v, want nil for an unborn HEAD", err)
	}
	if len(changed) != 0 {
		t.Fatalf("changed = %v, want empty for an unborn HEAD", changed)
	}
}

// TestHeadBytesReturnsRatifiedBytes: a committed note's HEAD bytes are the
// committed content, even after the working copy is edited.
func TestHeadBytesReturnsRatifiedBytes(t *testing.T) {
	root := makeNoteVault(t)
	v := noteVault(root)
	gitInit(t, root)
	writeNoteFile(t, root, "a.md", "# A\nRatified.\n")
	gitCommitAll(t, root)
	writeNoteFile(t, root, "a.md", "# A\nWorking edit.\n")

	got, ok, err := HeadBytes(v, "a.md")
	if err != nil {
		t.Fatalf("HeadBytes error = %v", err)
	}
	if !ok {
		t.Fatalf("HeadBytes ok = false, want true for a committed note")
	}
	if string(got) != "# A\nRatified.\n" {
		t.Fatalf("HeadBytes = %q, want the committed bytes", got)
	}
}

// TestHeadBytesAbsentForUntracked: a brand-new (untracked) note has no HEAD bytes.
func TestHeadBytesAbsentForUntracked(t *testing.T) {
	root := makeNoteVault(t)
	v := noteVault(root)
	gitInit(t, root)
	writeNoteFile(t, root, "tracked.md", "# Tracked\n")
	gitCommitAll(t, root)
	writeNoteFile(t, root, "new.md", "# New\n")

	_, ok, err := HeadBytes(v, "new.md")
	if err != nil {
		t.Fatalf("HeadBytes error = %v", err)
	}
	if ok {
		t.Fatalf("HeadBytes ok = true, want false for an untracked note")
	}
}
