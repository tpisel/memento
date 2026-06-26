package orient

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tpisel/memento/internal/brief"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/markdown"
	"github.com/tpisel/memento/internal/vault"
)

func TestBaselineContainsRenderMarkers(t *testing.T) {
	for _, marker := range [][]byte{
		[]byte("<!-- memento:brief-disclosure -->"),
	} {
		if got := bytes.Count(Baseline(), marker); got != 1 {
			t.Fatalf("Baseline marker %q count = %d, want 1", marker, got)
		}
	}
}

func TestBaselineHasNoLegacyWritingGuidePrecondition(t *testing.T) {
	got := string(Baseline())
	for _, unwanted := range []string{
		"<!-- memento:triggered-preconditions -->",
		"Triggered Preconditions",
		"_memento/writing.md",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("Baseline() =\n%s\nwant no legacy writing-guide precondition %q", got, unwanted)
		}
	}
}

func TestBaselineFramesBriefAsOnDemand(t *testing.T) {
	got := string(Baseline())
	for _, want := range []string{
		"Use `memento brief` when you need the doc landscape",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Baseline() =\n%s\nwant it to contain %q", got, want)
		}
	}
	for _, unwanted := range []string{
		"run `memento brief` first",
		"Before anything else",
		"then `memento brief`",
		"before deciding which notes or sections to read",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("Baseline() =\n%s\nwant no mandatory brief framing containing %q", got, unwanted)
		}
	}
}

func TestRenderSubstitutesBriefDisclosure(t *testing.T) {
	v := testVault(t)
	m := manifest.Manifest{
		SchemaVersion: manifest.CurrentSchemaVersion,
		Entries: []manifest.Entry{
			{
				Key:     "alpha.md",
				Title:   "Alpha",
				Summary: "Alpha summary.",
				Mode:    markdown.ModeAppendOnly,
				Lines:   5,
			},
			{
				Key:     "beta.md",
				Title:   "Beta",
				Summary: "Beta summary.",
				Mode:    markdown.ModeReadOnly,
				Lines:   8,
			},
		},
	}

	out, _, err := Render(v, m)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	lineEstimate := bytes.Count(brief.Render(m), []byte("\n"))
	want := fmt.Sprintf("`memento brief` will print summaries of 2 notes (~%d lines); it is dense and pull-only.", lineEstimate)
	if !strings.Contains(string(out), want) {
		t.Fatalf("Render() output =\n%s\nwant brief disclosure containing %q", out, want)
	}
	if strings.Contains(string(out), "<!-- memento:brief-disclosure -->") {
		t.Fatalf("Render() output still contains brief disclosure marker:\n%s", out)
	}
}

func TestRenderBriefDisclosureForEmptyManifest(t *testing.T) {
	v := testVault(t)
	m := manifest.Manifest{SchemaVersion: manifest.CurrentSchemaVersion}

	out, _, err := Render(v, m)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "`memento brief` will report no notes yet."
	if !strings.Contains(string(out), want) {
		t.Fatalf("Render() output =\n%s\nwant empty-manifest disclosure %q", out, want)
	}
}

func writeConvention(t *testing.T, v vault.Vault, name, contents string) {
	t.Helper()
	dir := filepath.Join(v.Root, vault.ToolDirName, "conventions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir conventions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write convention %s: %v", name, err)
	}
}

func TestRenderListsValidConventionsSortedByName(t *testing.T) {
	v := testVault(t)
	// Written out of order to prove deterministic sorting by name.
	writeConvention(t, v, "writing", "---\ntitle: Writing\nwhen_to_read: before authoring a memento vault write\n---\nbody\n")
	writeConvention(t, v, "conventions", "---\ntitle: Conventions\nwhen_to_read: before adding or editing a convention\n---\nbody\n")
	writeConvention(t, v, "summarising", "---\ntitle: Summarising\nwhen_to_read: when writing a note summary\n---\nbody\n")

	out, warnings, err := Render(v, manifest.Manifest{SchemaVersion: manifest.CurrentSchemaVersion})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("Render() warnings = %v, want none", warnings)
	}

	want := "## When To Read Conventions\n\n" +
		"- before adding or editing a convention: `memento convention conventions`\n" +
		"- when writing a note summary: `memento convention summarising`\n" +
		"- before authoring a memento vault write: `memento convention writing`\n"
	if !strings.Contains(string(out), want) {
		t.Fatalf("Render() output =\n%s\nwant conventions block:\n%s", out, want)
	}
}

func TestRenderOmitsConventionsBlockWhenNone(t *testing.T) {
	v := testVault(t)

	out, warnings, err := Render(v, manifest.Manifest{SchemaVersion: manifest.CurrentSchemaVersion})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("Render() warnings = %v, want none", warnings)
	}
	if strings.Contains(string(out), "When To Read Conventions") {
		t.Fatalf("Render() output =\n%s\nwant no conventions block", out)
	}
}

func TestRenderWarnsAboutInvalidConventionButKeepsValid(t *testing.T) {
	v := testVault(t)
	writeConvention(t, v, "writing", "---\ntitle: Writing\nwhen_to_read: before authoring a memento vault write\n---\nbody\n")
	// Missing when_to_read makes this convention invalid.
	writeConvention(t, v, "broken", "---\ntitle: Broken\n---\nbody\n")

	out, warnings, err := Render(v, manifest.Manifest{SchemaVersion: manifest.CurrentSchemaVersion})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("Render() warnings = %v, want exactly one", warnings)
	}
	if !strings.Contains(warnings[0], "broken.md") {
		t.Fatalf("Render() warning = %q, want it to name broken.md", warnings[0])
	}

	want := "- before authoring a memento vault write: `memento convention writing`"
	if !strings.Contains(string(out), want) {
		t.Fatalf("Render() output =\n%s\nwant valid convention retained:\n%s", out, want)
	}
	if strings.Contains(string(out), "broken") {
		t.Fatalf("Render() output =\n%s\nwant invalid convention omitted", out)
	}
}

func testVault(t *testing.T) vault.Vault {
	t.Helper()
	root := t.TempDir()
	markerDir := filepath.Join(root, vault.MarkerDirName)
	if err := os.Mkdir(markerDir, 0o755); err != nil {
		t.Fatalf("create marker dir: %v", err)
	}
	return vault.Vault{
		Root:         root,
		MarkerDir:    markerDir,
		ManifestPath: filepath.Join(markerDir, vault.ManifestFileName),
	}
}
