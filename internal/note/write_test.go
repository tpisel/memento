package note

import (
	"errors"
	"os"
	"os/exec"
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

func TestWriteAppendsUnratifiedReadOnlyMode(t *testing.T) {
	root := makeVault(t)
	initGit(t, root)
	original := "---\nmode: read-only\n---\n# Note\n\nOriginal.\n"
	writeFile(t, root, "note.md", original)

	err := Write(vaultFromRoot(root), "note.md", []byte("\nAppended.\n"), WriteOptions{})
	if err != nil {
		t.Fatalf("Write(unratified read-only) error = %v, want nil", err)
	}

	want := original + "\nAppended.\n"
	if got := readFile(t, root, "note.md"); got != want {
		t.Fatalf("unratified read-only file = %q, want %q", got, want)
	}
}

func TestWriteRejectsRatifiedReadOnlyModeWithoutChangingFile(t *testing.T) {
	root := makeVault(t)
	initGit(t, root)
	original := "---\nmode: read-only\n---\n# Note\n\nOriginal.\n"
	writeFile(t, root, "note.md", original)
	commitAll(t, root)

	err := Write(vaultFromRoot(root), "note.md", []byte("\nAppended.\n"), WriteOptions{})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Write(ratified read-only) error = %v, want ErrReadOnly", err)
	}

	if got := readFile(t, root, "note.md"); got != original {
		t.Fatalf("ratified read-only file changed to %q, want %q", got, original)
	}
}

func TestWriteRejectsNonGitReadOnlyModeWithoutChangingFile(t *testing.T) {
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

func TestWriteAppendsUnratifiedNonReadOnlyModes(t *testing.T) {
	tests := []struct {
		name     string
		original string
	}{
		{
			name:     "append-only",
			original: "---\nmode: append-only\n---\n# Note\n\nOriginal.\n",
		},
		{
			name:     "missing mode",
			original: "---\ntitle: Note\n---\n# Note\n\nOriginal.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := makeVault(t)
			initGit(t, root)
			writeFile(t, root, "note.md", tt.original)

			err := Write(vaultFromRoot(root), "note.md", []byte("\nAppended.\n"), WriteOptions{})
			if err != nil {
				t.Fatalf("Write(%s) error = %v, want nil", tt.name, err)
			}

			want := tt.original + "\nAppended.\n"
			if got := readFile(t, root, "note.md"); got != want {
				t.Fatalf("appended file = %q, want %q", got, want)
			}
		})
	}
}

func TestWriteAppendsExistingFilesWithNonReadOnlyModes(t *testing.T) {
	for _, mode := range []markdown.WriteMode{
		markdown.ModeAppendOnly,
		markdown.ModeLiving,
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

func TestWriteModeMatrixForRatificationAndOperations(t *testing.T) {
	tests := []struct {
		name      string
		ratified  bool
		mode      markdown.WriteMode
		operation WriteOperation
		wantErr   error
		want      string
	}{
		{
			name:      "unratified append-only append",
			mode:      markdown.ModeAppendOnly,
			operation: OperationAppend,
			want:      "---\nmode: append-only\n---\n# Note\n\nOriginal.\n\nAppended.\n",
		},
		{
			name:      "unratified append-only overwrite",
			mode:      markdown.ModeAppendOnly,
			operation: OperationOverwrite,
			want:      "# Replacement\n\nChanged.\n",
		},
		{
			name:      "unratified living append",
			mode:      markdown.ModeLiving,
			operation: OperationAppend,
			want:      "---\nmode: living\n---\n# Note\n\nOriginal.\n\nAppended.\n",
		},
		{
			name:      "unratified living overwrite",
			mode:      markdown.ModeLiving,
			operation: OperationOverwrite,
			want:      "# Replacement\n\nChanged.\n",
		},
		{
			name:      "unratified read-only append",
			mode:      markdown.ModeReadOnly,
			operation: OperationAppend,
			want:      "---\nmode: read-only\n---\n# Note\n\nOriginal.\n\nAppended.\n",
		},
		{
			name:      "unratified read-only overwrite",
			mode:      markdown.ModeReadOnly,
			operation: OperationOverwrite,
			want:      "# Replacement\n\nChanged.\n",
		},
		{
			name:      "ratified append-only append",
			ratified:  true,
			mode:      markdown.ModeAppendOnly,
			operation: OperationAppend,
			want:      "---\nmode: append-only\n---\n# Note\n\nOriginal.\n\nAppended.\n",
		},
		{
			name:      "ratified append-only overwrite",
			ratified:  true,
			mode:      markdown.ModeAppendOnly,
			operation: OperationOverwrite,
			wantErr:   ErrReadOnly,
			want:      "---\nmode: append-only\n---\n# Note\n\nOriginal.\n",
		},
		{
			name:      "ratified living append",
			ratified:  true,
			mode:      markdown.ModeLiving,
			operation: OperationAppend,
			want:      "---\nmode: living\n---\n# Note\n\nOriginal.\n\nAppended.\n",
		},
		{
			name:      "ratified living overwrite",
			ratified:  true,
			mode:      markdown.ModeLiving,
			operation: OperationOverwrite,
			want:      "# Replacement\n\nChanged.\n",
		},
		{
			name:      "ratified read-only append",
			ratified:  true,
			mode:      markdown.ModeReadOnly,
			operation: OperationAppend,
			wantErr:   ErrReadOnly,
			want:      "---\nmode: read-only\n---\n# Note\n\nOriginal.\n",
		},
		{
			name:      "ratified read-only overwrite",
			ratified:  true,
			mode:      markdown.ModeReadOnly,
			operation: OperationOverwrite,
			wantErr:   ErrReadOnly,
			want:      "---\nmode: read-only\n---\n# Note\n\nOriginal.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := makeVault(t)
			initGit(t, root)
			original := "---\nmode: " + string(tt.mode) + "\n---\n# Note\n\nOriginal.\n"
			writeFile(t, root, "note.md", original)
			if tt.ratified {
				commitAll(t, root)
			}

			content := []byte("\nAppended.\n")
			if tt.operation == OperationOverwrite {
				content = []byte("# Replacement\n\nChanged.\n")
			}
			err := Write(vaultFromRoot(root), "note.md", content, WriteOptions{Operation: tt.operation})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Write(%s) error = %v, want %v", tt.operation, err, tt.wantErr)
			}
			if got := readFile(t, root, "note.md"); got != tt.want {
				t.Fatalf("file after %s = %q, want %q", tt.operation, got, tt.want)
			}
		})
	}
}

func TestWriteRejectsOverwriteForRatifiedMissingModeLikeAppendOnly(t *testing.T) {
	tests := []struct {
		name     string
		original string
	}{
		{
			name:     "no frontmatter",
			original: "# Note\n\nOriginal.\n",
		},
		{
			name:     "missing mode",
			original: "---\ntitle: Note\n---\n# Note\n\nOriginal.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := makeVault(t)
			initGit(t, root)
			writeFile(t, root, "note.md", tt.original)
			commitAll(t, root)

			err := Write(vaultFromRoot(root), "note.md", []byte("replacement\n"), WriteOptions{Operation: OperationOverwrite})
			if !errors.Is(err, ErrReadOnly) {
				t.Fatalf("Write(overwrite) error = %v, want ErrReadOnly", err)
			}

			if got := readFile(t, root, "note.md"); got != tt.original {
				t.Fatalf("file changed after rejected overwrite: %q", got)
			}
		})
	}
}

func TestWriteOverwriteChangingBodyInvalidatesStoredSummaryHash(t *testing.T) {
	root := makeVault(t)
	initGit(t, root)
	originalBody := "# Note\n\nOriginal.\n"
	originalMeta := metadataFor(t, "note.md", []byte(originalBody))
	original := "---\nmode: living\nsummary: Original.\nsummary_hash: " + originalMeta.BodyHash + "\n---\n" + originalBody
	writeFile(t, root, "note.md", original)
	commitAll(t, root)

	replacement := []byte("---\nmode: living\nsummary: Original.\nsummary_hash: " + originalMeta.BodyHash + "\n---\n# Note\n\nChanged.\n")
	err := Write(vaultFromRoot(root), "note.md", replacement, WriteOptions{Operation: OperationOverwrite})
	if err != nil {
		t.Fatalf("Write(overwrite living with stale summary hash) error = %v, want nil", err)
	}

	got := metadataFor(t, "note.md", []byte(readFile(t, root, "note.md")))
	if !got.SummaryStale {
		t.Fatal("SummaryStale = false, want true after overwrite changes body but preserves old summary_hash")
	}
}

func metadataFor(t *testing.T, key string, source []byte) markdown.Metadata {
	t.Helper()

	meta, err := markdown.ExtractMetadata(key, source)
	if err != nil {
		t.Fatalf("ExtractMetadata(%s) error = %v", key, err)
	}
	return meta
}

func TestWriteRejectsUnsupportedMutationOperations(t *testing.T) {
	for _, op := range []WriteOperation{
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

func initGit(t *testing.T, root string) {
	t.Helper()

	runGit(t, root, "init")
}

func commitAll(t *testing.T, root string) {
	t.Helper()

	runGit(t, root, "add", ".")
	runGit(t, root,
		"-c", "user.name=Memento Test",
		"-c", "user.email=memento-test@example.invalid",
		"commit", "--no-gpg-sign", "-m", "initial",
	)
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(output))
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
