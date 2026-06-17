package manifest

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tpisel/memento/internal/markdown"
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
  "schema_version": 2,
  "entries": [
    {
      "key": "alpha.md",
      "title": "Alpha",
      "summary": "Alpha summary.",
      "bytes": 37,
      "lines": 5,
      "tags": [],
      "headings": [
        {
          "level": 2,
          "text": "Decision",
          "slug": "decision"
        }
      ],
      "mode": "append-only",
      "orient": false,
      "updated": "",
      "body_sha": "",
      "summary_sha": "",
      "summary_state": "missing",
      "links": {
        "out": [],
        "in": []
      }
    },
    {
      "key": "zeta.md",
      "title": "Zeta",
      "summary": "Zeta summary.",
      "bytes": 160,
      "lines": 14,
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
      "orient": false,
      "updated": "2026-06-10T00:00:00Z",
      "body_sha": "86c102dd358139b3fc87586d46859b8429855ce9854d018f4a89c25a0f2dd542",
      "summary_sha": "ca706f22337802a72e42656c3eaf84ec2601710b6b744c758ecd5f492d8e5082",
      "summary_state": "current",
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

func TestCompileDerivesSummaryStateFromPriorLedger(t *testing.T) {
	root := makeVault(t)
	v := vaultFromRoot(root)

	writeFile(t, root, "note.md", "---\nsummary: Summary.\n---\n# Note\n\nOriginal body.\n")
	first := compileAndStoreManifest(t, v)
	firstEntry := onlyEntry(t, first)
	if firstEntry.SummaryState != markdown.SummaryCurrent {
		t.Fatalf("first SummaryState = %q, want %q", firstEntry.SummaryState, markdown.SummaryCurrent)
	}
	if firstEntry.BodySHA == "" || firstEntry.SummarySHA == "" {
		t.Fatalf("first ledger hashes = body %q summary %q, want both populated", firstEntry.BodySHA, firstEntry.SummarySHA)
	}

	unchanged := compileAndStoreManifest(t, v)
	unchangedEntry := onlyEntry(t, unchanged)
	if unchangedEntry.SummaryState != markdown.SummaryCurrent {
		t.Fatalf("unchanged SummaryState = %q, want %q", unchangedEntry.SummaryState, markdown.SummaryCurrent)
	}
	if unchangedEntry.BodySHA != firstEntry.BodySHA || unchangedEntry.SummarySHA != firstEntry.SummarySHA {
		t.Fatalf("unchanged ledger hashes changed: got body %q summary %q, want body %q summary %q", unchangedEntry.BodySHA, unchangedEntry.SummarySHA, firstEntry.BodySHA, firstEntry.SummarySHA)
	}

	writeFile(t, root, "note.md", "---\nsummary: Summary.\n---\n# Note\n\nChanged body.\n")
	bodyOnly := compileAndStoreManifest(t, v)
	bodyOnlyEntry := onlyEntry(t, bodyOnly)
	if bodyOnlyEntry.SummaryState != markdown.SummaryStale {
		t.Fatalf("body-only SummaryState = %q, want %q", bodyOnlyEntry.SummaryState, markdown.SummaryStale)
	}
	if bodyOnlyEntry.BodySHA != firstEntry.BodySHA || bodyOnlyEntry.SummarySHA != firstEntry.SummarySHA {
		t.Fatalf("stale ledger hashes = body %q summary %q, want carried body %q summary %q", bodyOnlyEntry.BodySHA, bodyOnlyEntry.SummarySHA, firstEntry.BodySHA, firstEntry.SummarySHA)
	}

	writeFile(t, root, "note.md", "---\nsummary: Updated summary.\n---\n# Note\n\nChanged body.\n")
	summaryEdit := compileAndStoreManifest(t, v)
	summaryEditEntry := onlyEntry(t, summaryEdit)
	if summaryEditEntry.SummaryState != markdown.SummaryCurrent {
		t.Fatalf("summary-edit SummaryState = %q, want %q", summaryEditEntry.SummaryState, markdown.SummaryCurrent)
	}
	if summaryEditEntry.BodySHA == firstEntry.BodySHA || summaryEditEntry.SummarySHA == firstEntry.SummarySHA {
		t.Fatalf("summary-edit ledger hashes = body %q summary %q, want refreshed from first body %q summary %q", summaryEditEntry.BodySHA, summaryEditEntry.SummarySHA, firstEntry.BodySHA, firstEntry.SummarySHA)
	}

	storeManifest(t, v, first)
	writeFile(t, root, "note.md", "---\nsummary: Both changed.\n---\n# Note\n\nBoth changed body.\n")
	bothEdit := compileAndStoreManifest(t, v)
	bothEditEntry := onlyEntry(t, bothEdit)
	if bothEditEntry.SummaryState != markdown.SummaryCurrent {
		t.Fatalf("both-edit SummaryState = %q, want %q", bothEditEntry.SummaryState, markdown.SummaryCurrent)
	}

	writeFile(t, root, "note.md", "---\ndescription: Description fallback.\n---\n# Note\n\nBody.\n")
	description := compileAndStoreManifest(t, v)
	descriptionEntry := onlyEntry(t, description)
	if descriptionEntry.Summary != "Description fallback." || descriptionEntry.SummaryState != markdown.SummaryCurrent {
		t.Fatalf("description entry summary/state = %q/%q, want description current", descriptionEntry.Summary, descriptionEntry.SummaryState)
	}

	writeFile(t, root, "note.md", "# Note\n\nFirst paragraph fallback.\n")
	missing := compileAndStoreManifest(t, v)
	missingEntry := onlyEntry(t, missing)
	if missingEntry.Summary != "First paragraph fallback." {
		t.Fatalf("missing Summary = %q, want first paragraph fallback", missingEntry.Summary)
	}
	if missingEntry.SummaryState != markdown.SummaryMissing || missingEntry.BodySHA != "" || missingEntry.SummarySHA != "" {
		t.Fatalf("missing ledger = state %q body %q summary %q, want missing with empty hashes", missingEntry.SummaryState, missingEntry.BodySHA, missingEntry.SummarySHA)
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

	want := "{\n  \"schema_version\": 2,\n  \"entries\": []\n}\n"
	if string(data) != want {
		t.Fatalf("empty manifest JSON = %q, want %q", string(data), want)
	}
}

func TestCompileManifestMatchesSchemaFixture(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "alpha.md", "# Alpha\n\nAlpha summary.\n\n## Decision\n")
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

	m, err := Compile(vaultFromRoot(root))
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil", err)
	}
	got, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal(compiled) error = %v, want nil", err)
	}

	fixture, err := os.ReadFile(filepath.Join("testdata", "manifest_v2.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	fixtureManifest, err := Decode(fixture)
	if err != nil {
		t.Fatalf("Decode(fixture) error = %v, want nil", err)
	}
	want, err := Marshal(fixtureManifest)
	if err != nil {
		t.Fatalf("Marshal(fixture) error = %v, want nil", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("compiled manifest =\n%s\nwant fixture:\n%s", got, want)
	}
}

func TestDecodeRejectsUnsupportedSchemaVersion(t *testing.T) {
	tests := map[string]string{
		"missing": `{"entries":[]}`,
		"future":  `{"schema_version":3,"entries":[]}`,
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Decode([]byte(input))
			if !errors.Is(err, ErrSchemaUnsupported) {
				t.Fatalf("Decode() error = %v, want ErrSchemaUnsupported", err)
			}
			if !strings.Contains(err.Error(), "schema_version") {
				t.Fatalf("Decode() error = %v, want schema_version detail", err)
			}
		})
	}
}

func TestLoadRejectsUnsupportedSchemaVersion(t *testing.T) {
	root := makeVault(t)
	v := vaultFromRoot(root)
	writeFile(t, root, ".memento/manifest.json", `{"entries":[]}`)

	_, err := Load(v)
	if !errors.Is(err, ErrSchemaUnsupported) {
		t.Fatalf("Load() error = %v, want ErrSchemaUnsupported", err)
	}
}

func TestCompileEmitsOrientFrontmatterField(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "included.md", "---\norient: true\n---\n# Included\n")
	writeFile(t, root, "explicit-false.md", "---\norient: false\n---\n# Explicit False\n")
	writeFile(t, root, "absent.md", "# Absent\n")

	m, err := Compile(vaultFromRoot(root))
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil", err)
	}

	entries := make(map[string]Entry, len(m.Entries))
	for _, entry := range m.Entries {
		entries[entry.Key] = entry
	}

	if !entries["included.md"].Orient {
		t.Fatal("included.md Orient = false, want true")
	}
	for _, key := range []string{"explicit-false.md", "absent.md"} {
		if entries[key].Orient {
			t.Fatalf("%s Orient = true, want false", key)
		}
	}

	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}
	if !bytes.Contains(data, []byte(`"orient": true`)) {
		t.Fatalf("manifest JSON =\n%s\nwant orient true emitted", data)
	}
	if !bytes.Contains(data, []byte(`"orient": false`)) {
		t.Fatalf("manifest JSON =\n%s\nwant orient false emitted", data)
	}
}

func TestCompilePopulatesEntrySizes(t *testing.T) {
	root := makeVault(t)
	files := map[string]string{
		"empty.md":       "",
		"no-newline.md":  "# No Newline",
		"trailing-lf.md": "# Trailing LF\n",
		"crlf.md":        "# CRLF\r\n\r\nSummary.\r\n",
	}
	for relPath, content := range files {
		writeFile(t, root, relPath, content)
	}

	m, err := Compile(vaultFromRoot(root))
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil", err)
	}

	entries := make(map[string]Entry, len(m.Entries))
	for _, entry := range m.Entries {
		entries[entry.Key] = entry
	}

	for relPath, content := range files {
		entry, ok := entries[relPath]
		if !ok {
			t.Fatalf("missing manifest entry for %s", relPath)
		}
		if entry.Bytes != int64(len([]byte(content))) {
			t.Fatalf("%s Bytes = %d, want %d", relPath, entry.Bytes, len([]byte(content)))
		}
		if entry.Lines != bytes.Count([]byte(content), []byte("\n")) {
			t.Fatalf("%s Lines = %d, want %d", relPath, entry.Lines, bytes.Count([]byte(content), []byte("\n")))
		}
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
  "schema_version": 2,
  "entries": [
    {
      "key": "Embeds/Thing.md",
      "title": "Thing",
      "summary": "",
      "bytes": 8,
      "lines": 1,
      "tags": [],
      "headings": [],
      "mode": "append-only",
      "orient": false,
      "updated": "",
      "body_sha": "",
      "summary_sha": "",
      "summary_state": "missing",
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
      "bytes": 9,
      "lines": 1,
      "tags": [],
      "headings": [],
      "mode": "append-only",
      "orient": false,
      "updated": "",
      "body_sha": "",
      "summary_sha": "",
      "summary_state": "missing",
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
      "bytes": 102,
      "lines": 3,
      "tags": [],
      "headings": [],
      "mode": "append-only",
      "orient": false,
      "updated": "",
      "body_sha": "",
      "summary_sha": "",
      "summary_state": "missing",
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

func TestCompilePreservesAnchoredWikiLinksInManifest(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "source.md", `# Source

Links to [[Target#Decision|the target decision]], [[Target#Context]], [[#Local Heading]], and [[Missing#Anchor]].

## Local Heading
`)
	writeFile(t, root, "Target.md", `# Target

## Decision

Decision text.

## Context

Context text.
`)

	m, err := Compile(vaultFromRoot(root))
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil", err)
	}

	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}

	want := `{
  "schema_version": 2,
  "entries": [
    {
      "key": "Target.md",
      "title": "Target",
      "summary": "Decision text.",
      "bytes": 65,
      "lines": 9,
      "tags": [],
      "headings": [
        {
          "level": 2,
          "text": "Decision",
          "slug": "decision"
        },
        {
          "level": 2,
          "text": "Context",
          "slug": "context"
        }
      ],
      "mode": "append-only",
      "orient": false,
      "updated": "",
      "body_sha": "",
      "summary_sha": "",
      "summary_state": "missing",
      "links": {
        "out": [],
        "in": [
          {
            "source": "source.md",
            "type": "wiki",
            "anchor": "Context"
          },
          {
            "source": "source.md",
            "type": "wiki",
            "anchor": "Decision"
          }
        ]
      }
    },
    {
      "key": "source.md",
      "title": "Source",
      "summary": "Links to [[Target#Decision|the target decision]], [[Target#Context]], [[#Local Heading]], and [[Missing#Anchor]].",
      "bytes": 142,
      "lines": 5,
      "tags": [],
      "headings": [
        {
          "level": 2,
          "text": "Local Heading",
          "slug": "local-heading"
        }
      ],
      "mode": "append-only",
      "orient": false,
      "updated": "",
      "body_sha": "",
      "summary_sha": "",
      "summary_state": "missing",
      "links": {
        "out": [
          {
            "target": "Missing",
            "type": "wiki",
            "resolved": false,
            "anchor": "Anchor"
          },
          {
            "target": "Target.md",
            "type": "wiki",
            "resolved": true,
            "anchor": "Decision"
          },
          {
            "target": "Target.md",
            "type": "wiki",
            "resolved": true,
            "anchor": "Context"
          }
        ],
        "in": []
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

func TestCompileAcceptsOKFConventionFrontmatterWithoutWarnings(t *testing.T) {
	root := makeVault(t)
	writeFile(t, root, "okf.md", `---
title: OKF Note
type: BigQuery Table
resource: //bigquery.googleapis.com/projects/demo/datasets/core/tables/events
timestamp: 2026-06-14T10:30:00Z
okf_version: "0.1"
---
# Ignored H1

OKF convention fields should not warn.
`)

	m, warnings, err := CompileWithWarnings(vaultFromRoot(root))
	if err != nil {
		t.Fatalf("CompileWithWarnings() error = %v, want nil", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("CompileWithWarnings() warnings = %d, want 0: %#v", len(warnings), warnings)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.Entries))
	}
	if m.Entries[0].Summary != "OKF convention fields should not warn." {
		t.Fatalf("Summary = %q, want first paragraph fallback", m.Entries[0].Summary)
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

func compileAndStoreManifest(t *testing.T, v vault.Vault) Manifest {
	t.Helper()

	m, err := Compile(v)
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil", err)
	}
	storeManifest(t, v, m)
	return m
}

func storeManifest(t *testing.T, v vault.Vault, m Manifest) {
	t.Helper()

	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}
	if err := os.WriteFile(v.ManifestPath, data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func onlyEntry(t *testing.T, m Manifest) Entry {
	t.Helper()

	if len(m.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.Entries))
	}
	return m.Entries[0]
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
