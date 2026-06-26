package enforce

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tpisel/memento/internal/note"
)

func TestResolveWritablePathReturnsJoinedPathForExistingDir(t *testing.T) {
	root := makeVault(t)
	v := vaultFromRoot(root)
	writeFile(t, root, "notes/existing.md", "# Note\n")

	got, err := ResolveWritablePath(v, "notes/new.md")
	if err != nil {
		t.Fatalf("ResolveWritablePath error = %v, want nil", err)
	}
	want := resolveVaultPath(t, root, "notes/new.md")
	if got != want {
		t.Fatalf("ResolveWritablePath = %q, want %q", got, want)
	}
}

// The verdict path must never create directories — that side-effect was dropped
// from the salvaged writablePath (ADR-0031: a verdict must not mutate the FS).
func TestResolveWritablePathDoesNotCreateParentDirectories(t *testing.T) {
	root := makeVault(t)
	v := vaultFromRoot(root)

	got, err := ResolveWritablePath(v, "deep/nested/new.md")
	if err != nil {
		t.Fatalf("ResolveWritablePath error = %v, want nil", err)
	}
	want := resolveVaultPath(t, root, "deep/nested/new.md")
	if got != want {
		t.Fatalf("ResolveWritablePath = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, "deep")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("parent directory was created (stat err = %v); resolve must not mutate the filesystem", err)
	}
}

func TestResolveWritablePathRejectsDirectoryKey(t *testing.T) {
	root := makeVault(t)
	v := vaultFromRoot(root)
	if err := os.MkdirAll(filepath.Join(root, "notes", "sub.md"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := ResolveWritablePath(v, "notes/sub.md")
	if !errors.Is(err, note.ErrInvalidKey) {
		t.Fatalf("ResolveWritablePath(dir) error = %v, want ErrInvalidKey", err)
	}
}

func TestResolveWritablePathRejectsSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privilege on Windows")
	}
	root := makeVault(t)
	v := vaultFromRoot(root)
	writeFile(t, root, "real.md", "# Real\n")
	if err := os.Symlink(filepath.Join(root, "real.md"), filepath.Join(root, "link.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := ResolveWritablePath(v, "link.md")
	if !errors.Is(err, note.ErrInvalidKey) {
		t.Fatalf("ResolveWritablePath(symlink) error = %v, want ErrInvalidKey", err)
	}
}

func TestResolveWritablePathRejectsEscapeViaSymlinkedParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privilege on Windows")
	}
	root := makeVault(t)
	v := vaultFromRoot(root)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := ResolveWritablePath(v, "escape/note.md")
	if !errors.Is(err, note.ErrInvalidKey) {
		t.Fatalf("ResolveWritablePath(escape) error = %v, want ErrInvalidKey", err)
	}
}

func resolveVaultPath(t *testing.T, root, key string) string {
	t.Helper()
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve root %q: %v", root, err)
	}
	realRoot, err = filepath.Abs(realRoot)
	if err != nil {
		t.Fatalf("abs root %q: %v", realRoot, err)
	}
	return filepath.Join(filepath.Clean(realRoot), filepath.FromSlash(key))
}
