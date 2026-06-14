package vault

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/tpisel/memento/internal/ignore"
)

func TestWalkMarkdownAppliesIgnoreRules(t *testing.T) {
	root := t.TempDir()
	vault := makeVault(t, root)
	writeFile(t, root, ".mementoignore", "ignored.md\ndrafts/\n")
	writeFile(t, root, "keep.md", "# Keep\n")
	writeFile(t, root, "notes/keep.md", "# Nested keep\n")
	writeFile(t, root, "ignored.md", "# Ignored\n")
	writeFile(t, root, "notes/ignored.md", "# Nested ignored\n")
	writeFile(t, root, "drafts/keep-out.md", "# Draft\n")
	writeFile(t, root, "notes/drafts/keep-out.md", "# Nested draft\n")
	writeFile(t, root, "asset.txt", "not markdown\n")

	got := walkPaths(t, vault)
	want := []string{
		"keep.md",
		"notes/keep.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WalkMarkdown() visited %v, want %v", got, want)
	}
}

func TestWalkMarkdownExcludesOperationalFiles(t *testing.T) {
	root := t.TempDir()
	vault := makeVault(t, root)
	writeFile(t, root, ".mementoignore", "")
	writeFile(t, root, ".memento/manifest.md", "# Not content\n")
	writeFile(t, root, ".memento/deep/config.md", "# Not content\n")
	writeFile(t, root, "writing_guide.md", "# Operational\n")
	writeFile(t, root, "content.md", "# Content\n")

	got := walkPaths(t, vault)
	want := []string{"content.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WalkMarkdown() visited %v, want %v", got, want)
	}
}

func TestWalkMarkdownWithoutIgnoreFile(t *testing.T) {
	root := t.TempDir()
	vault := makeVault(t, root)
	writeFile(t, root, "content.md", "# Content\n")

	got := walkPaths(t, vault)
	want := []string{"content.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WalkMarkdown() visited %v, want %v", got, want)
	}
}

func TestWalkMarkdownPreservesNFCAndNFDUnicodePaths(t *testing.T) {
	root := t.TempDir()
	vault := makeVault(t, root)
	nfc := "caf\u00e9.md"
	nfd := "cafe\u0301.md"
	writeFile(t, root, "nfc/"+nfc, "# NFC\n")
	writeFile(t, root, "nfd/"+nfd, "# NFD\n")

	got := walkPaths(t, vault)
	want := []string{
		"nfc/" + nfc,
		"nfd/" + nfd,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WalkMarkdown() visited %#v, want %#v", got, want)
	}
}

func TestWalkMarkdownPreservesCaseDistinctPathsWhenFilesystemSupportsThem(t *testing.T) {
	root := t.TempDir()
	vault := makeVault(t, root)
	writeFile(t, root, "Note.md", "# Upper\n")
	writeFile(t, root, "note.md", "# Lower\n")

	upperInfo, err := os.Stat(filepath.Join(root, "Note.md"))
	if err != nil {
		t.Fatalf("stat Note.md: %v", err)
	}
	lowerInfo, err := os.Stat(filepath.Join(root, "note.md"))
	if err != nil {
		t.Fatalf("stat note.md: %v", err)
	}
	if os.SameFile(upperInfo, lowerInfo) {
		t.Skip("case-folded filesystem cannot represent case-only filename twins")
	}

	got := walkPaths(t, vault)
	want := []string{"Note.md", "note.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WalkMarkdown() visited %v, want %v", got, want)
	}
}

func TestWalkMarkdownSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	vault := makeVault(t, root)
	writeFile(t, root, "content.md", "# Content\n")

	outside := t.TempDir()
	writeExternalFile(t, outside, "outside.md", "# Outside\n")
	writeExternalFile(t, outside, "outside-dir/nested.md", "# Nested\n")
	if err := os.Symlink(filepath.Join(outside, "outside.md"), filepath.Join(root, "linked.md")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside-dir"), filepath.Join(root, "linked-dir")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	got := walkPaths(t, vault)
	want := []string{"content.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WalkMarkdown() visited %v, want %v", got, want)
	}
}

func TestWalkMarkdownReturnsIgnoreParseError(t *testing.T) {
	root := t.TempDir()
	vault := makeVault(t, root)
	writeFile(t, root, ".mementoignore", "!unsupported.md\n")

	err := WalkMarkdown(vault, func(string, string) error {
		t.Fatal("WalkMarkdown() called visitor despite invalid .mementoignore")
		return nil
	})
	if err == nil {
		t.Fatal("WalkMarkdown() error = nil, want parse error")
	}
	if !errors.Is(err, ignore.ErrUnsupportedNegation) {
		t.Fatalf("WalkMarkdown() error = %v, want unsupported negation parse error", err)
	}
}

func TestWalkMarkdownPropagatesVisitorError(t *testing.T) {
	root := t.TempDir()
	vault := makeVault(t, root)
	writeFile(t, root, "content.md", "# Content\n")
	wantErr := errors.New("extract failed")

	err := WalkMarkdown(vault, func(string, string) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WalkMarkdown() error = %v, want %v", err, wantErr)
	}
}

func walkPaths(t *testing.T, vault Vault) []string {
	t.Helper()

	var got []string
	err := WalkMarkdown(vault, func(relPath, absPath string) error {
		if !filepath.IsAbs(absPath) {
			t.Fatalf("WalkMarkdown() absPath = %q, want absolute path", absPath)
		}
		got = append(got, relPath)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkMarkdown() error = %v, want nil", err)
	}
	sort.Strings(got)
	return got
}

func makeVault(t *testing.T, root string) Vault {
	t.Helper()

	marker := mkdir(t, root, MarkerDirName)
	return Vault{
		Root:         root,
		MarkerDir:    marker,
		ManifestPath: filepath.Join(marker, ManifestFileName),
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

func writeExternalFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for external %q: %v", relPath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write external %q: %v", relPath, err)
	}
}
