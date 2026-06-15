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

func TestBindingForReadTargetTreatsTrackedPathspecMetacharactersLiterally(t *testing.T) {
	root := makeVault(t)
	initGit(t, root)
	key := "foo[bar].md"
	writeFile(t, root, key, "# Note\n\nTracked.\n")
	commitAll(t, root)

	got, err := BindingForReadTarget(vaultFromRoot(root), key)
	if err != nil {
		t.Fatalf("BindingForReadTarget(%q) error = %v, want nil", key, err)
	}
	if got != BindingRatified {
		t.Fatalf("BindingForReadTarget(%q) = %s, want %s", key, got, BindingRatified)
	}
}

func TestBindingForReadTargetDoesNotRatifyUntrackedPathspecMetacharacterMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reserves '*' as a filename metacharacter")
	}

	root := makeVault(t)
	initGit(t, root)
	writeFile(t, root, "foobar.md", "# Note\n\nTracked.\n")
	commitAll(t, root)

	key := "foo*.md"
	writeFile(t, root, key, "# Note\n\nUntracked.\n")

	got, err := BindingForReadTarget(vaultFromRoot(root), key)
	if err != nil {
		t.Fatalf("BindingForReadTarget(%q) error = %v, want nil", key, err)
	}
	if got != BindingUnratified {
		t.Fatalf("BindingForReadTarget(%q) = %s, want %s", key, got, BindingUnratified)
	}
}

func TestBindingForReadTargetTreatsOldGitNotRepositoryAsNonGitVault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell shim")
	}

	root := makeVault(t)
	shimDir := t.TempDir()
	shimPath := filepath.Join(shimDir, "git")
	shim := "#!/bin/sh\n" +
		"printf 'fatal: not a git repository (or any of the parent directories): .git\\n' >&2\n" +
		"exit 128\n"
	if err := os.WriteFile(shimPath, []byte(shim), 0o755); err != nil {
		t.Fatalf("write git shim: %v", err)
	}
	t.Setenv("PATH", shimDir)

	got, err := BindingForReadTarget(vaultFromRoot(root), "note.md")
	if err != nil {
		t.Fatalf("BindingForReadTarget() error = %v, want nil", err)
	}
	if got != BindingRatified {
		t.Fatalf("BindingForReadTarget() = %s, want %s", got, BindingRatified)
	}
}

func TestBindingForReadTargetSurfacesBrokenRepositoryRevParseFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell shim")
	}

	root := makeVault(t)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	shimDir := t.TempDir()
	shimPath := filepath.Join(shimDir, "git")
	shim := "#!/bin/sh\n" +
		"printf 'fatal: not a git repository (or any of the parent directories): .git\\n' >&2\n" +
		"exit 128\n"
	if err := os.WriteFile(shimPath, []byte(shim), 0o755); err != nil {
		t.Fatalf("write git shim: %v", err)
	}
	t.Setenv("PATH", shimDir)

	_, err := BindingForReadTarget(vaultFromRoot(root), "note.md")
	if err == nil {
		t.Fatal("BindingForReadTarget() error = nil, want broken repository error")
	}
	if !strings.Contains(err.Error(), "check git work tree") {
		t.Fatalf("BindingForReadTarget() error = %q, want work tree context", err.Error())
	}
	if !strings.Contains(err.Error(), "fatal: not a git repository") {
		t.Fatalf("BindingForReadTarget() error = %q, want git stderr", err.Error())
	}
}

func TestBindingForReadTargetSurfacesFatalRevParseFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell shim")
	}

	root := makeVault(t)
	shimDir := t.TempDir()
	shimPath := filepath.Join(shimDir, "git")
	shim := "#!/bin/sh\n" +
		"printf 'fatal: detected dubious ownership in repository\\n' >&2\n" +
		"exit 128\n"
	if err := os.WriteFile(shimPath, []byte(shim), 0o755); err != nil {
		t.Fatalf("write git shim: %v", err)
	}
	t.Setenv("PATH", shimDir)

	_, err := BindingForReadTarget(vaultFromRoot(root), "note.md")
	if err == nil {
		t.Fatal("BindingForReadTarget() error = nil, want fatal git error")
	}
	if !strings.Contains(err.Error(), "check git work tree") {
		t.Fatalf("BindingForReadTarget() error = %q, want work tree context", err.Error())
	}
	if !strings.Contains(err.Error(), "dubious ownership") {
		t.Fatalf("BindingForReadTarget() error = %q, want git stderr", err.Error())
	}
}

func TestBindingForReadTargetSurfacesFatalLSFilesFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell shim")
	}

	root := makeVault(t)
	shimDir := t.TempDir()
	shimPath := filepath.Join(shimDir, "git")
	shim := "#!/bin/sh\n" +
		"if [ \"$2\" = \"rev-parse\" ]; then\n" +
		"  printf 'true\\n'\n" +
		"  exit 0\n" +
		"fi\n" +
		"printf 'fatal: bad revision HEAD\\n' >&2\n" +
		"exit 128\n"
	if err := os.WriteFile(shimPath, []byte(shim), 0o755); err != nil {
		t.Fatalf("write git shim: %v", err)
	}
	t.Setenv("PATH", shimDir)

	_, err := BindingForReadTarget(vaultFromRoot(root), "note.md")
	if err == nil {
		t.Fatal("BindingForReadTarget() error = nil, want fatal git error")
	}
	if !strings.Contains(err.Error(), "check git ratification for note.md") {
		t.Fatalf("BindingForReadTarget() error = %q, want ratification context", err.Error())
	}
	if !strings.Contains(err.Error(), "bad revision HEAD") {
		t.Fatalf("BindingForReadTarget() error = %q, want git stderr", err.Error())
	}
}

func TestBindingForReadTargetFailsWhenGitDisappearsAfterWorkTreeCheck(t *testing.T) {
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

	_, err := BindingForReadTarget(vaultFromRoot(root), "note.md")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("BindingForReadTarget() error = %v, want exec.ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "check git ratification for note.md") {
		t.Fatalf("BindingForReadTarget() error = %q, want ratification context", err.Error())
	}
}
