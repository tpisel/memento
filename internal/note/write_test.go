package note

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tpisel/memento/internal/markdown"
	"github.com/tpisel/memento/internal/vault"
)

func TestWriteCreatesNewMarkdownFile(t *testing.T) {
	root := makeVault(t)
	content := []byte("# New\n\nDurable note.\n")

	if err := Write(vaultFromRoot(root), "notes/new.md", content, WriteOptions{}); err != nil {
		t.Fatalf("Write(create) error = %v, want nil", err)
	}

	got := readFile(t, root, "notes/new.md")
	if got != string(content) {
		t.Fatalf("created file = %q, want %q", got, string(content))
	}
}

func TestWriteAppendsExistingMarkdownFile(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "note.md", "# Note\n\nExisting.\n")

	err := Write(vaultFromRoot(root), "note.md", []byte("\nAppended.\n"), WriteOptions{})
	if err != nil {
		t.Fatalf("Write(append) error = %v, want nil", err)
	}

	want := "# Note\n\nExisting.\n\nAppended.\n"
	if got := readFile(t, root, "note.md"); got != want {
		t.Fatalf("appended file = %q, want %q", got, want)
	}
}

func TestWriteRejectsReadOnlyModeWithoutChangingFile(t *testing.T) {
	root := makeVault(t)
	original := "---\nmode: read-only\n---\n# Note\n\nOriginal.\n"
	writeFile(t, root, "note.md", original)

	err := Write(vaultFromRoot(root), "note.md", []byte("\nAppended.\n"), WriteOptions{})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Write(read-only) error = %v, want ErrReadOnly", err)
	}

	if got := readFile(t, root, "note.md"); got != original {
		t.Fatalf("read-only file changed to %q, want %q", got, original)
	}
}

func TestWriteAppendsExistingFilesWithNonReadOnlyModes(t *testing.T) {
	for _, mode := range []markdown.WriteMode{
		markdown.ModeAppendOnly,
		markdown.ModeSectionReplace,
		markdown.ModeKeyedUpsert,
	} {
		t.Run(string(mode), func(t *testing.T) {
			root := makeVault(t)
			writeFile(t, root, "note.md", "---\nmode: "+string(mode)+"\n---\n# Note\n\nOriginal.\n")

			err := Write(vaultFromRoot(root), "note.md", []byte("\nAppended.\n"), WriteOptions{})
			if err != nil {
				t.Fatalf("Write(%s) error = %v, want nil", mode, err)
			}

			want := "---\nmode: " + string(mode) + "\n---\n# Note\n\nOriginal.\n\nAppended.\n"
			if got := readFile(t, root, "note.md"); got != want {
				t.Fatalf("appended file = %q, want %q", got, want)
			}
		})
	}
}

func TestWriteAppendsExistingFileWithMissingMode(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "note.md", "---\ntitle: Note\n---\n# Note\n\nOriginal.\n")

	err := Write(vaultFromRoot(root), "note.md", []byte("\nAppended.\n"), WriteOptions{})
	if err != nil {
		t.Fatalf("Write(missing mode) error = %v, want nil", err)
	}

	want := "---\ntitle: Note\n---\n# Note\n\nOriginal.\n\nAppended.\n"
	if got := readFile(t, root, "note.md"); got != want {
		t.Fatalf("appended file = %q, want %q", got, want)
	}
}

func TestWriteRejectsUnreadableFrontmatterWithoutChangingFile(t *testing.T) {
	root := makeVault(t)
	original := "---\ntitle\n---\n# Note\n\nOriginal.\n"
	writeFile(t, root, "note.md", original)

	err := Write(vaultFromRoot(root), "note.md", []byte("\nAppended.\n"), WriteOptions{})
	if !errors.Is(err, markdown.ErrMalformedFrontmatter) {
		t.Fatalf("Write(malformed frontmatter) error = %v, want ErrMalformedFrontmatter", err)
	}

	if got := readFile(t, root, "note.md"); got != original {
		t.Fatalf("malformed-frontmatter file changed to %q, want %q", got, original)
	}
}

func TestWriteRejectsOverwriteStyleOperations(t *testing.T) {
	for _, op := range []WriteOperation{
		OperationOverwrite,
		OperationSectionReplace,
		OperationKeyedUpsert,
	} {
		t.Run(string(op), func(t *testing.T) {
			root := makeVault(t)
			writeFile(t, root, "note.md", "# Note\n\nOriginal.\n")

			err := Write(vaultFromRoot(root), "note.md", []byte("replacement\n"), WriteOptions{Operation: op})
			if !errors.Is(err, ErrUnsupportedWriteOperation) {
				t.Fatalf("Write(operation %q) error = %v, want ErrUnsupportedWriteOperation", op, err)
			}

			if got := readFile(t, root, "note.md"); got != "# Note\n\nOriginal.\n" {
				t.Fatalf("file changed after rejected operation: %q", got)
			}
		})
	}
}

func TestWriteRejectsPathTraversal(t *testing.T) {
	root := makeVault(t)

	for _, key := range []string{
		"../outside.md",
		"notes/../../outside.md",
		"/absolute.md",
		"notes\\outside.md",
	} {
		t.Run(key, func(t *testing.T) {
			err := Write(vaultFromRoot(root), key, []byte("# Outside\n"), WriteOptions{})
			if !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Write(%q) error = %v, want ErrInvalidKey", key, err)
			}
		})
	}
}

func TestWriteRejectsSymlinkTraversal(t *testing.T) {
	root := makeVault(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	err := Write(vaultFromRoot(root), "link/outside.md", []byte("# Outside\n"), WriteOptions{})
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Write(symlink traversal) error = %v, want ErrInvalidKey", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "outside.md")); !os.IsNotExist(err) {
		t.Fatalf("outside file was created through symlink; stat err = %v", err)
	}
}

func TestWriteRejectsNonMarkdownAndOperationalPaths(t *testing.T) {
	root := makeVault(t)

	for _, key := range []string{
		"note.txt",
		vault.IgnoreFileName,
		vault.WritingGuideFileName,
		vault.MarkerDirName + "/manifest.md",
	} {
		t.Run(key, func(t *testing.T) {
			err := Write(vaultFromRoot(root), key, []byte("content\n"), WriteOptions{})
			if !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Write(%q) error = %v, want ErrInvalidKey", key, err)
			}
		})
	}
}

func TestWriteRejectsIgnoredContentPaths(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, vault.IgnoreFileName, "private/\nignored.md\n")

	for _, key := range []string{"ignored.md", "private/note.md"} {
		t.Run(key, func(t *testing.T) {
			err := Write(vaultFromRoot(root), key, []byte("# Ignored\n"), WriteOptions{})
			if !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Write(%q) error = %v, want ErrInvalidKey", key, err)
			}
		})
	}
}

func readFile(t *testing.T, root, relPath string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("read %q: %v", relPath, err)
	}
	return string(data)
}
