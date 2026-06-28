package enforce

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/vault"
)

// ResolveWritablePath resolves key to an absolute path inside the vault and
// confirms the target is writable, without mutating the filesystem. Unlike the
// v0 write path it does NOT create parent directories — a verdict must never
// touch disk. It rejects keys that name an existing directory or symlink, or
// whose deepest existing ancestor resolves outside the vault.
func ResolveWritablePath(v vault.Vault, key string) (string, error) {
	root, err := filepath.EvalSymlinks(v.Root)
	if err != nil {
		return "", fmt.Errorf("resolve vault root: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve vault root: %w", err)
	}
	root = filepath.Clean(root)

	path := filepath.Join(root, filepath.FromSlash(key))
	if info, err := os.Lstat(path); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("%w: key names a directory: %s", note.ErrInvalidKey, key)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: symlink targets are not writable: %s", note.ErrInvalidKey, key)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat %s: %w", key, err)
	}

	realAncestor, err := deepestExistingDir(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("resolve parent directory for %s: %w", key, err)
	}

	rel, err := filepath.Rel(root, realAncestor)
	if err != nil {
		return "", fmt.Errorf("resolve parent directory for %s: %w", key, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: path resolves outside vault: %s", note.ErrInvalidKey, key)
	}
	return path, nil
}

// deepestExistingDir climbs from dir until it finds a directory that exists,
// resolving symlinks along the way, and returns its real cleaned path. Because a
// non-existent path component cannot be a symlink, resolving the deepest existing
// ancestor is enough to detect a vault escape without creating anything (the
// pure analog of the write path's MkdirAll-then-EvalSymlinks).
func deepestExistingDir(dir string) (string, error) {
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", err
		}
		dir = parent
	}
}
