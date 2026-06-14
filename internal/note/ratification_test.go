package note

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBindingForKeyTreatsTrackedPathspecMetacharactersLiterally(t *testing.T) {
	root := makeVault(t)
	initGit(t, root)
	key := "foo[bar].md"
	writeFile(t, root, key, "# Note\n\nTracked.\n")
	commitAll(t, root)

	got, err := BindingForKey(vaultFromRoot(root), key)
	if err != nil {
		t.Fatalf("BindingForKey(%q) error = %v, want nil", key, err)
	}
	if got != BindingRatified {
		t.Fatalf("BindingForKey(%q) = %s, want %s", key, got, BindingRatified)
	}
}

func TestBindingForKeyDoesNotRatifyUntrackedPathspecMetacharacterMatch(t *testing.T) {
	root := makeVault(t)
	initGit(t, root)
	writeFile(t, root, "foobar.md", "# Note\n\nTracked.\n")
	commitAll(t, root)

	key := "foo*.md"
	writeFile(t, root, key, "# Note\n\nUntracked.\n")

	got, err := BindingForKey(vaultFromRoot(root), key)
	if err != nil {
		t.Fatalf("BindingForKey(%q) error = %v, want nil", key, err)
	}
	if got != BindingUnratified {
		t.Fatalf("BindingForKey(%q) = %s, want %s", key, got, BindingUnratified)
	}
}

func TestBindingForKeyFailsWhenGitDisappearsAfterWorkTreeCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell shim")
	}

	root := makeVault(t)
	shimDir := t.TempDir()
	shimPath := filepath.Join(shimDir, "git")
	shim := "#!/bin/sh\n" +
		"if [ \"$2\" = \"rev-parse\" ]; then\n" +
		"  /bin/rm \"$GIT_SHIM_PATH\"\n" +
		"  printf 'true\\n'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 99\n"
	if err := os.WriteFile(shimPath, []byte(shim), 0o755); err != nil {
		t.Fatalf("write git shim: %v", err)
	}
	t.Setenv("PATH", shimDir)
	t.Setenv("GIT_SHIM_PATH", shimPath)

	_, err := BindingForKey(vaultFromRoot(root), "note.md")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("BindingForKey() error = %v, want exec.ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "check git ratification for note.md") {
		t.Fatalf("BindingForKey() error = %q, want ratification context", err.Error())
	}
}
