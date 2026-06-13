package manifest

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tpisel/memento/internal/vault"
)

func TestCompileProducesDeterministicManifest(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, ".mementoignore", "ignored.md\n")
	writeFile(t, root, "zeta.md", `---
title: Zeta
summary: Zeta summary.
tags: [v0, memento]
mode: read-only
updated: 2026-06-10
summary_hash: mismatch
---

# Ignored H1

## Context

Zeta body.
`)
	writeFile(t, root, "alpha.md", `# Alpha

Alpha summary.

## Decision
`)
	writeFile(t, root, "ignored.md", "# Ignored\n")
	writeFile(t, root, "writing_guide.md", "# Operational\n")

	v := vaultFromRoot(root)
	first, err := Compile(v)
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil", err)
	}
	second, err := Compile(v)
	if err != nil {
		t.Fatalf("Compile() second error = %v, want nil", err)
	}

	firstJSON, err := Marshal(first)
	if err != nil {
		t.Fatalf("Marshal(first) error = %v, want nil", err)
	}
	secondJSON, err := Marshal(second)
	if err != nil {
		t.Fatalf("Marshal(second) error = %v, want nil", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("manifest JSON changed between runs:\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}

	want := `{
  "entries": [
    {
      "key": "alpha.md",
      "title": "Alpha",
      "summary": "Alpha summary.",
      "tags": [],
      "headings": [
        {
          "level": 2,
          "text": "Decision",
          "slug": "decision"
        }
      ],
      "mode": "append-only",
      "updated": "",
      "summary_stale": true,
      "links": {
        "out": [],
        "in": []
      }
    },
    {
      "key": "zeta.md",
      "title": "Zeta",
      "summary": "Zeta summary.",
      "tags": [
        "memento",
        "v0"
      ],
      "headings": [
        {
          "level": 2,
          "text": "Context",
          "slug": "context"
        }
      ],
      "mode": "read-only",
      "updated": "2026-06-10T00:00:00Z",
      "summary_stale": true,
      "links": {
        "out": [],
        "in": []
      }
    }
  ],
  "tags": {
    "memento": 1,
    "v0": 1
  }
}
`
	if string(firstJSON) != want {
		t.Fatalf("manifest JSON =\n%s\nwant:\n%s", firstJSON, want)
	}
}

func TestWriteCreatesManifestFile(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "note.md", "# Note\n\nSummary.\n")

	v := vaultFromRoot(root)
	if err := Write(v); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	first, err := os.ReadFile(v.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if err := Write(v); err != nil {
		t.Fatalf("Write() second error = %v, want nil", err)
	}
	second, err := os.ReadFile(v.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest second: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("manifest file changed between unchanged writes:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestCompileEmptyVaultSerializesEntriesArray(t *testing.T) {
	root := makeVault(t)

	m, err := Compile(vaultFromRoot(root))
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil", err)
	}
	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}

	want := "{\n  \"entries\": []\n}\n"
	if string(data) != want {
		t.Fatalf("empty manifest JSON = %q, want %q", string(data), want)
	}
}

func TestMarshalDoesNotEscapeHTMLCharacters(t *testing.T) {
	m := Manifest{
		Entries: []Entry{
			{
				Key:     "angle.md",
				Title:   "Use <tags> & symbols",
				Summary: "Keep <, >, and & readable.",
				Tags:    []string{"a&b"},
				Mode:    "append-only",
				Links: Links{
					Out: []OutLink{},
					In:  []InLink{},
				},
			},
		},
	}

	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}
	for _, want := range []string{"Use <tags> & symbols", "Keep <, >, and & readable.", "a&b"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("Marshal() =\n%s\nwant unescaped %q", data, want)
		}
	}
	for _, forbidden := range []string{"\\u"} {
		if bytes.Contains(data, []byte(forbidden)) {
			t.Fatalf("Marshal() contains %q:\n%s", forbidden, data)
		}
	}
}

func TestCompileExtractsWikiLinkGraph(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "source.md", `# Source

Links to [[Target|the target]], [[Missing]], ![[Embeds/Thing]], [[Target]], and [[source]].
`)
	writeFile(t, root, "Target.md", "# Target\n")
	writeFile(t, root, "Embeds/Thing.md", "# Thing\n")

	m, err := Compile(vaultFromRoot(root))
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil", err)
	}

	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}

	want := `{
  "entries": [
    {
      "key": "Embeds/Thing.md",
      "title": "Thing",
      "summary": "",
      "tags": [],
      "headings": [],
      "mode": "append-only",
      "updated": "",
      "summary_stale": true,
      "links": {
        "out": [],
        "in": [
          {
            "source": "source.md",
            "type": "embed"
          }
        ]
      }
    },
    {
      "key": "Target.md",
      "title": "Target",
      "summary": "",
      "tags": [],
      "headings": [],
      "mode": "append-only",
      "updated": "",
      "summary_stale": true,
      "links": {
        "out": [],
        "in": [
          {
            "source": "source.md",
            "type": "wiki"
          }
        ]
      }
    },
    {
      "key": "source.md",
      "title": "Source",
      "summary": "Links to [[Target|the target]], [[Missing]], ![[Embeds/Thing]], [[Target]], and [[source]].",
      "tags": [],
      "headings": [],
      "mode": "append-only",
      "updated": "",
      "summary_stale": true,
      "links": {
        "out": [
          {
            "target": "Embeds/Thing.md",
            "type": "embed",
            "resolved": true
          },
          {
            "target": "Missing",
            "type": "wiki",
            "resolved": false
          },
          {
            "target": "Target.md",
            "type": "wiki",
            "resolved": true
          },
          {
            "target": "source.md",
            "type": "wiki",
            "resolved": true
          }
        ],
        "in": [
          {
            "source": "source.md",
            "type": "wiki"
          }
        ]
      }
    }
  ]
}
`
	if string(data) != want {
		t.Fatalf("manifest JSON =\n%s\nwant:\n%s", data, want)
	}
}

func TestCompileWarnsAndFallsBackForMalformedFrontmatter(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "broken.md", `---
title
---
# Fallback Title

Fallback summary.
`)

	m, warnings, err := CompileWithWarnings(vaultFromRoot(root))
	if err != nil {
		t.Fatalf("CompileWithWarnings() error = %v, want nil", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("CompileWithWarnings() warnings = %d, want 1: %#v", len(warnings), warnings)
	}
	if warnings[0].Path != "broken.md" {
		t.Fatalf("warning path = %q, want broken.md", warnings[0].Path)
	}

	if len(m.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.Entries))
	}
	entry := m.Entries[0]
	if entry.Title != "Fallback Title" {
		t.Fatalf("Title = %q, want fallback H1", entry.Title)
	}
	if entry.Summary != "Fallback summary." {
		t.Fatalf("Summary = %q, want fallback paragraph", entry.Summary)
	}

	if _, err := Compile(vaultFromRoot(root)); err != nil {
		t.Fatalf("Compile() error = %v, want nil despite malformed frontmatter", err)
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
