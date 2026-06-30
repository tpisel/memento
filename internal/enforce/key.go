// Package enforce holds pure, side-effect-free enforcement primitives shared by
// the write path and the hook-facing check-write verdict (ADR-0031): writable
// key/path normalization, the three-mode lattice evaluation, and the
// ratification predicate. Nothing here mutates the filesystem — a verdict must
// be derivable without touching disk.
package enforce

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tpisel/memento/internal/convention"
	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/vault"
)

// ErrVaultPrefixedKey reports a key whose leading segment is the vault directory
// name — i.e. a repo-relative path supplied where a vault-relative key is
// expected.
var ErrVaultPrefixedKey = errors.New("key is vault-relative, not repo-relative")

// NormalizeWritableKey validates key as a writable vault-relative markdown key
// and returns it unchanged on success. It is side-effect-free: it reads the
// vault's ignore rules but never mutates the filesystem. Rejected: vault-prefixed
// keys, non-.md keys, operational paths (.mementoignore, the writing guide, the
// marker directory), and ignored paths. Base normalization (empty, backslash,
// absolute, "."/"..") is delegated to note.NormalizeKey.
func NormalizeWritableKey(v vault.Vault, key string) (string, error) {
	key, err := note.NormalizeKey(key)
	if err != nil {
		return "", err
	}
	parts := strings.Split(key, "/")
	if strings.EqualFold(parts[0], filepath.Base(v.Root)) {
		suggestion := strings.Join(parts[1:], "/")
		return "", fmt.Errorf("%w; did you mean %q?", ErrVaultPrefixedKey, suggestion)
	}
	if filepath.Ext(key) != ".md" {
		return "", fmt.Errorf("%w: write keys must name markdown files: %s", note.ErrInvalidKey, key)
	}
	// Convention carve-out: conventions live in the operational namespace but are
	// the project-editable source of workflow policy (ADR-0029/0030), so admit
	// them ahead of the blanket _memento/ rejection and the ignored-path check —
	// both of which would otherwise deny every gated agent edit to a convention.
	// IsConventionKey already constrains the shape to a valid bare stem, so this
	// cannot become a back door for misfiling a normal note into _memento/.
	if convention.IsConventionKey(key) {
		return key, nil
	}
	if key == vault.IgnoreFileName || key == vault.WritingGuideFileName {
		return "", fmt.Errorf("%w: operational path is not writable: %s", note.ErrInvalidKey, key)
	}
	if parts[0] == vault.MarkerDirName || parts[0] == vault.ToolDirName {
		return "", fmt.Errorf("%w: operational path is not writable: %s", note.ErrInvalidKey, key)
	}

	for i := 0; i < len(parts)-1; i++ {
		dir := strings.Join(parts[:i+1], "/")
		ignored, err := vault.IsIgnored(v, dir, true)
		if err != nil {
			return "", err
		}
		if ignored {
			return "", fmt.Errorf("%w: ignored path is not writable: %s", note.ErrInvalidKey, key)
		}
	}

	ignored, err := vault.IsIgnored(v, key, false)
	if err != nil {
		return "", err
	}
	if ignored {
		return "", fmt.Errorf("%w: ignored path is not writable: %s", note.ErrInvalidKey, key)
	}
	return key, nil
}
