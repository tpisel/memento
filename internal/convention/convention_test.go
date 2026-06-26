package convention

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tpisel/memento/internal/vault"
)

func newVault(t *testing.T) vault.Vault {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, vault.ToolDirName, DirName), 0o755); err != nil {
		t.Fatalf("mkdir conventions: %v", err)
	}
	return vault.Vault{Root: root}
}

func writeConvention(t *testing.T, v vault.Vault, name, contents string) {
	t.Helper()
	if err := os.WriteFile(Path(v, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("write convention %s: %v", name, err)
	}
}

func TestReadStripsFrontmatterAndReturnsBody(t *testing.T) {
	v := newVault(t)
	writeConvention(t, v, "writing", "---\ntitle: Writing guide\nwhen_to_read: before authoring a memento vault write\n---\n\n# Writing guide\n\nWrite durable knowledge.\n")

	c, err := Read(v, "writing")
	if err != nil {
		t.Fatalf("Read(writing) error = %v", err)
	}
	if c.Title != "Writing guide" {
		t.Fatalf("Title = %q, want %q", c.Title, "Writing guide")
	}
	if c.WhenToRead != "before authoring a memento vault write" {
		t.Fatalf("WhenToRead = %q", c.WhenToRead)
	}
	want := "\n# Writing guide\n\nWrite durable knowledge.\n"
	if string(c.Body) != want {
		t.Fatalf("Body = %q, want %q", string(c.Body), want)
	}
}

func TestReadUnquotesScalars(t *testing.T) {
	v := newVault(t)
	writeConvention(t, v, "writing", "---\ntitle: \"Writing guide\"\nwhen_to_read: 'before a write'\n---\nbody\n")

	c, err := Read(v, "writing")
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if c.Title != "Writing guide" || c.WhenToRead != "before a write" {
		t.Fatalf("Title=%q WhenToRead=%q", c.Title, c.WhenToRead)
	}
}

func TestReadInvalidNames(t *testing.T) {
	v := newVault(t)
	for _, name := range []string{"", "Writing", "writing.md", "sub/writing", "sub\\writing", "..", "with space"} {
		if _, err := Read(v, name); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Read(%q) error = %v, want ErrInvalidName", name, err)
		}
	}
}

func TestReadMissingFile(t *testing.T) {
	v := newVault(t)
	_, err := Read(v, "absent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Read(absent) error = %v, want ErrNotFound", err)
	}
	if got := err.Error(); !strings.Contains(got, RelPath("absent")) {
		t.Fatalf("error %q does not name %q", got, RelPath("absent"))
	}
}

func TestReadMissingWhenToRead(t *testing.T) {
	v := newVault(t)
	writeConvention(t, v, "writing", "---\ntitle: Writing guide\n---\nbody\n")
	if _, err := Read(v, "writing"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Read error = %v, want ErrInvalid", err)
	}
}

func TestReadEmptyWhenToRead(t *testing.T) {
	v := newVault(t)
	writeConvention(t, v, "writing", "---\ntitle: Writing guide\nwhen_to_read:   \n---\nbody\n")
	if _, err := Read(v, "writing"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Read error = %v, want ErrInvalid", err)
	}
}

func TestReadBlockScalarWhenToReadIsInvalid(t *testing.T) {
	// A YAML block scalar (when_to_read: | or >, then an indented body) is not
	// supported: the single-line scalar reader cannot capture the block body, so
	// the convention is rejected rather than silently bound to the bare "|"/">"
	// indicator. This pins ADR-0029's single-line-scalar intent.
	for _, indicator := range []string{"|", ">", "|-", ">+", "|2"} {
		contents := "---\ntitle: Writing guide\nwhen_to_read: " + indicator + "\n  read me before a write\n---\nbody\n"
		v := newVault(t)
		writeConvention(t, v, "writing", contents)
		if _, err := Read(v, "writing"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Read with when_to_read: %q error = %v, want ErrInvalid", indicator, err)
		}
	}
}

func TestReadNoFrontmatterIsInvalid(t *testing.T) {
	v := newVault(t)
	writeConvention(t, v, "writing", "# Writing guide\n\nbody\n")
	if _, err := Read(v, "writing"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Read error = %v, want ErrInvalid", err)
	}
}

func TestListReturnsValidConventionsSortedByName(t *testing.T) {
	v := newVault(t)
	writeConvention(t, v, "writing", "---\nwhen_to_read: before a write\n---\nbody\n")
	writeConvention(t, v, "beads", "---\nwhen_to_read: before touching beads\n---\nbody\n")
	writeConvention(t, v, "summarising", "---\nwhen_to_read: when summarising\n---\nbody\n")

	valid, warnings, err := List(v)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("List() warnings = %v, want none", warnings)
	}
	got := []string{valid[0].Name, valid[1].Name, valid[2].Name}
	want := []string{"beads", "summarising", "writing"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List() names = %v, want %v", got, want)
		}
	}
}

func TestListWarnsAboutInvalidFilesAndSkipsNonMarkdown(t *testing.T) {
	v := newVault(t)
	writeConvention(t, v, "writing", "---\nwhen_to_read: before a write\n---\nbody\n")
	writeConvention(t, v, "broken", "---\ntitle: Broken\n---\nbody\n")
	if err := os.WriteFile(filepath.Join(v.Root, vault.ToolDirName, DirName, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write notes.txt: %v", err)
	}

	valid, warnings, err := List(v)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(valid) != 1 || valid[0].Name != "writing" {
		t.Fatalf("List() valid = %v, want only writing", valid)
	}
	if len(warnings) != 1 {
		t.Fatalf("List() warnings = %v, want exactly one", warnings)
	}
	if !strings.Contains(warnings[0], "broken.md") {
		t.Fatalf("List() warning = %q, want it to name broken.md", warnings[0])
	}
}

func TestListMissingDirIsEmpty(t *testing.T) {
	root := t.TempDir()
	v := vault.Vault{Root: root}

	valid, warnings, err := List(v)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(valid) != 0 || len(warnings) != 0 {
		t.Fatalf("List() = (%v, %v), want empty", valid, warnings)
	}
}
