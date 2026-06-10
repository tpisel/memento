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
