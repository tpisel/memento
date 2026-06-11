package note

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tpisel/memento/internal/vault"
)

func TestReadReturnsNestedMarkdownByVaultRelativeKey(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "notes/deep.md", "# Deep\n\nNested content.\n")

	got, err := Read(vaultFromRoot(root), "notes/deep.md")
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}

	want := "# Deep\n\nNested content.\n"
	if string(got) != want {
		t.Fatalf("Read() = %q, want %q", string(got), want)
	}
}

func TestReadReturnsSectionByExactHeadingText(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "spec.md", `# Spec

Intro.

## API, Read & Write!

Target content.

### Child Detail

Child content.

## Next

Other content.
`)

	got, err := Read(vaultFromRoot(root), "spec.md#API, Read & Write!")
	if err != nil {
		t.Fatalf("Read(section) error = %v, want nil", err)
	}

	want := `## API, Read & Write!

Target content.

### Child Detail

Child content.

`
	if string(got) != want {
		t.Fatalf("Read(section) = %q, want %q", string(got), want)
	}
}

func TestReadReturnsSectionByNormalizedSlug(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "spec.md", `# Spec

## API, Read & Write!

Target content.

## Next

Other content.
`)

	got, err := Read(vaultFromRoot(root), "spec.md#api-read-write")
	if err != nil {
		t.Fatalf("Read(slug section) error = %v, want nil", err)
	}

	want := `## API, Read & Write!

Target content.

`
	if string(got) != want {
		t.Fatalf("Read(slug section) = %q, want %q", string(got), want)
	}
}

func TestReadReturnsNestedSectionBySlug(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "spec.md", `# Spec

## Parent

Parent content.

### Child

Child content.

### Sibling

Sibling content.
`)

	got, err := Read(vaultFromRoot(root), "spec.md#child")
	if err != nil {
		t.Fatalf("Read(nested section) error = %v, want nil", err)
	}

	want := `### Child

Child content.

`
	if string(got) != want {
		t.Fatalf("Read(nested section) = %q, want %q", string(got), want)
	}
}

func TestReadReturnsDuplicateHeadingSectionsBySlugSuffix(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "spec.md", `# Spec

## Context

First.

## Context

Second.

## Context!

Third.

## End

Done.
`)

	got, err := Read(vaultFromRoot(root), "spec.md#context-2")
	if err != nil {
		t.Fatalf("Read(duplicate section) error = %v, want nil", err)
	}

	want := `## Context!

Third.

`
	if string(got) != want {
		t.Fatalf("Read(duplicate section) = %q, want %q", string(got), want)
	}
}

func TestReadSectionIgnoresFrontmatterForHeadingOffsets(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "spec.md", `---
title: Spec
mode: read-only
---
# Spec

## Target

Target content.

## Next

Other content.
`)

	got, err := Read(vaultFromRoot(root), "spec.md#target")
	if err != nil {
		t.Fatalf("Read(frontmatter section) error = %v, want nil", err)
	}

	want := `## Target

Target content.

`
	if string(got) != want {
		t.Fatalf("Read(frontmatter section) = %q, want %q", string(got), want)
	}
}

func TestReadUnknownSectionFailsClearly(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "spec.md", "# Spec\n\n## Present\n\nContent.\n")

	_, err := Read(vaultFromRoot(root), "spec.md#missing")
	if !errors.Is(err, ErrSectionNotFound) {
		t.Fatalf("Read(unknown section) error = %v, want ErrSectionNotFound", err)
	}
}

func TestReadTreatsIgnoredMarkdownAsMissing(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, ".mementoignore", "ignored.md\n")
	writeFile(t, root, "ignored.md", "# Ignored\n")

	_, err := Read(vaultFromRoot(root), "ignored.md")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Read(ignored.md) error = %v, want ErrNotFound", err)
	}
}

func TestReadRejectsPathTraversal(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "inside.md", "# Inside\n")

	for _, key := range []string{
		"../outside.md",
		"notes/../../outside.md",
		"/absolute.md",
		"notes\\outside.md",
	} {
		t.Run(key, func(t *testing.T) {
			_, err := Read(vaultFromRoot(root), key)
			if !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Read(%q) error = %v, want ErrInvalidKey", key, err)
			}
		})
	}
}

func TestReadMissingKeyFailsClearly(t *testing.T) {
	root := makeVault(t)

	_, err := Read(vaultFromRoot(root), "missing.md")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Read(missing.md) error = %v, want ErrNotFound", err)
	}
}

func makeVault(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, vault.MarkerDirName), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	return root
}

func vaultFromRoot(root string) vault.Vault {
	marker := filepath.Join(root, vault.MarkerDirName)
	return vault.Vault{
		Root:         root,
		MarkerDir:    marker,
		ManifestPath: filepath.Join(marker, vault.ManifestFileName),
	}
}

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %q: %v", relPath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", relPath, err)
	}
}
