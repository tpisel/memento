package enforce

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/tpisel/memento/internal/note"
)

func TestNormalizeWritableKeyAcceptsValidKey(t *testing.T) {
	v := vaultFromRoot(makeVault(t))

	got, err := NormalizeWritableKey(v, "learnings/new.md")
	if err != nil {
		t.Fatalf("NormalizeWritableKey error = %v, want nil", err)
	}
	if got != "learnings/new.md" {
		t.Fatalf("NormalizeWritableKey = %q, want %q", got, "learnings/new.md")
	}
}

func TestNormalizeWritableKeyRejectsVaultPrefixedKey(t *testing.T) {
	root := makeVault(t)
	v := vaultFromRoot(root)

	key := filepath.Base(root) + "/notes/new.md"
	_, err := NormalizeWritableKey(v, key)
	if !errors.Is(err, ErrVaultPrefixedKey) {
		t.Fatalf("NormalizeWritableKey(%q) error = %v, want ErrVaultPrefixedKey", key, err)
	}
}

func TestNormalizeWritableKeyRejectsInvalidKeys(t *testing.T) {
	v := vaultFromRoot(makeVault(t))

	cases := []string{
		"",                     // empty (base normalization)
		"../escape.md",         // ".." component (base normalization)
		"/abs/note.md",         // absolute (base normalization)
		"notes/plain.txt",      // non-.md
		"notes/no-ext",         // non-.md
		".mementoignore",       // operational path
		"writing_guide.md",     // operational path
		".memento/config.md",   // marker directory
		".memento/manifest.md", // marker directory
		"_memento/writing.md",  // operational tool namespace (ADR-0030/0031)
		"_memento/skills/x.md", // operational tool namespace
	}
	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			_, err := NormalizeWritableKey(v, key)
			if !errors.Is(err, note.ErrInvalidKey) && !errors.Is(err, ErrVaultPrefixedKey) {
				t.Fatalf("NormalizeWritableKey(%q) error = %v, want invalid key", key, err)
			}
		})
	}
}

func TestNormalizeWritableKeyAdmitsConventions(t *testing.T) {
	root := makeVault(t)
	v := vaultFromRoot(root)
	// Mirror the default posture: the whole operational namespace is ignored.
	// The convention carve-out must still admit conventions for editing
	// (ADR-0029/0030), bypassing both the _memento/ and ignored-path rejections.
	writeFile(t, root, ".mementoignore", "_memento/\n")

	for _, key := range []string{
		"_memento/conventions/writing.md",
		"_memento/conventions/summarising.md",
	} {
		t.Run(key, func(t *testing.T) {
			got, err := NormalizeWritableKey(v, key)
			if err != nil {
				t.Fatalf("NormalizeWritableKey(%q) error = %v, want nil", key, err)
			}
			if got != key {
				t.Fatalf("NormalizeWritableKey(%q) = %q, want it unchanged", key, got)
			}
		})
	}

	// A non-convention path under the operational namespace stays rejected, so
	// the carve-out is not a back door for misfiling notes into _memento/.
	for _, key := range []string{
		"_memento/conventions/sub/deep.md", // too deep to be a convention
		"_memento/skills/write.md",         // sibling operational subtree
		"_memento/brief.md",                // generated artifact
	} {
		t.Run("rejects "+key, func(t *testing.T) {
			if _, err := NormalizeWritableKey(v, key); !errors.Is(err, note.ErrInvalidKey) {
				t.Fatalf("NormalizeWritableKey(%q) error = %v, want ErrInvalidKey", key, err)
			}
		})
	}
}

func TestNormalizeWritableKeyRejectsIgnoredPaths(t *testing.T) {
	root := makeVault(t)
	v := vaultFromRoot(root)
	writeFile(t, root, ".mementoignore", "drafts/\nsecret.md\n")

	for _, key := range []string{"drafts/note.md", "secret.md"} {
		t.Run(key, func(t *testing.T) {
			_, err := NormalizeWritableKey(v, key)
			if !errors.Is(err, note.ErrInvalidKey) {
				t.Fatalf("NormalizeWritableKey(%q) error = %v, want ErrInvalidKey", key, err)
			}
		})
	}
}
