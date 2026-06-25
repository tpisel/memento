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
		[]byte("<!-- memento:triggered-preconditions -->"),
		[]byte("<!-- memento:brief-disclosure -->"),
	} {
		if got := bytes.Count(Baseline(), marker); got != 1 {
			t.Fatalf("Baseline marker %q count = %d, want 1", marker, got)
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

	out, err := Render(v, m)
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

	out, err := Render(v, m)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := "`memento brief` will report no notes yet."
	if !strings.Contains(string(out), want) {
		t.Fatalf("Render() output =\n%s\nwant empty-manifest disclosure %q", out, want)
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
