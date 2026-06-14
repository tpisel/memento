package markdown

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestExtractMetadataFromFrontmatterRichMarkdown(t *testing.T) {
	source := []byte(`---
title: Frontmatter Title
summary: A concise summary from frontmatter.
tags: [memento, markdown, v0]
mode: section-replace
orient: true
updated: 2026-06-10
summary_hash: abc123
---

# Heading Title

Body paragraph that should not replace frontmatter summary.
`)

	got, err := ExtractMetadata("notes/rich.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	wantUpdated := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	if got.Title != "Frontmatter Title" {
		t.Fatalf("Title = %q, want frontmatter title", got.Title)
	}
	if got.Summary != "A concise summary from frontmatter." {
		t.Fatalf("Summary = %q, want frontmatter summary", got.Summary)
	}
	if !reflect.DeepEqual(got.Tags, []string{"memento", "markdown", "v0"}) {
		t.Fatalf("Tags = %v, want ordered frontmatter tags", got.Tags)
	}
	if got.Mode != ModeSectionReplace {
		t.Fatalf("Mode = %q, want %q", got.Mode, ModeSectionReplace)
	}
	if !got.Orient {
		t.Fatal("Orient = false, want true from frontmatter")
	}
	if !got.Updated.Equal(wantUpdated) {
		t.Fatalf("Updated = %v, want %v", got.Updated, wantUpdated)
	}
	if got.SummaryHash != "abc123" {
		t.Fatalf("SummaryHash = %q, want abc123", got.SummaryHash)
	}
	if got.BodyHash == "" {
		t.Fatal("BodyHash is empty, want deterministic body hash")
	}
	if !got.SummaryStale {
		t.Fatal("SummaryStale = false, want true when stored summary hash differs from body hash")
	}
}

func TestExtractMetadataFallbacksForSparseMarkdown(t *testing.T) {
	source := []byte(`
# H1 Title

First useful paragraph becomes the summary.

Second paragraph.
`)

	got, err := ExtractMetadata("notes/sparse-file.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	if got.Title != "H1 Title" {
		t.Fatalf("Title = %q, want H1 fallback", got.Title)
	}
	if got.Summary != "First useful paragraph becomes the summary." {
		t.Fatalf("Summary = %q, want first paragraph fallback", got.Summary)
	}
	if got.Mode != ModeAppendOnly {
		t.Fatalf("Mode = %q, want default mode %q", got.Mode, ModeAppendOnly)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("Tags = %v, want empty", got.Tags)
	}
	if !got.Updated.IsZero() {
		t.Fatalf("Updated = %v, want zero time", got.Updated)
	}
	if !got.SummaryStale {
		t.Fatal("SummaryStale = false, want true for missing summary hash")
	}
}

func TestExtractMetadataDefaultsMissingModeToAppendOnly(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "no frontmatter",
			src:  "# Title\n\nBody.\n",
		},
		{
			name: "frontmatter without mode",
			src:  "---\ntitle: Title\n---\nBody.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractMetadata("note.md", []byte(tt.src))
			if err != nil {
				t.Fatalf("ExtractMetadata() error = %v, want nil", err)
			}

			if got.Mode != DefaultWriteMode {
				t.Fatalf("Mode = %q, want default mode %q", got.Mode, DefaultWriteMode)
			}
		})
	}
}

func TestExtractMetadataDefaultsMissingOrientToFalse(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "no frontmatter",
			src:  "# Title\n\nBody.\n",
		},
		{
			name: "frontmatter without orient",
			src:  "---\ntitle: Title\n---\nBody.\n",
		},
		{
			name: "explicit false",
			src:  "---\norient: false\n---\nBody.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractMetadata("note.md", []byte(tt.src))
			if err != nil {
				t.Fatalf("ExtractMetadata() error = %v, want nil", err)
			}

			if got.Orient {
				t.Fatal("Orient = true, want false")
			}
		})
	}
}

func TestExtractMetadataTreatsHorizontalRuleSandwichAsBareMarkdown(t *testing.T) {
	source := []byte(`---
# Foo
---
Body.
`)

	got, err := ExtractMetadata("notes/foo.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	if got.Title != "Foo" {
		t.Fatalf("Title = %q, want H1 fallback", got.Title)
	}
	if got.Summary != "Body." {
		t.Fatalf("Summary = %q, want body paragraph fallback", got.Summary)
	}
	if got.BodyHash != hashBody(source) {
		t.Fatalf("BodyHash = %q, want hash of whole source %q", got.BodyHash, hashBody(source))
	}
}

func TestTitleFallbackUsesFilenameWhenNoFrontmatterOrH1(t *testing.T) {
	got, err := ExtractMetadata("design/long-lived-note.md", []byte("A paragraph without a heading.\n"))
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	if got.Title != "long-lived-note" {
		t.Fatalf("Title = %q, want filename stem fallback", got.Title)
	}
}

func TestSummaryFallbackSkipsHeadingAndBlankLines(t *testing.T) {
	source := []byte(`# Title

## Context

The first paragraph should be used.
`)

	got, err := ExtractMetadata("note.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	if got.Summary != "The first paragraph should be used." {
		t.Fatalf("Summary = %q, want first non-heading paragraph", got.Summary)
	}
}

func TestSummaryResolutionUsesSummaryThenDescriptionThenFirstParagraph(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "summary wins",
			src: `---
summary: Native summary.
description: OKF description.
---
# Title

First paragraph.
`,
			want: "Native summary.",
		},
		{
			name: "description fallback",
			src: `---
description: OKF description.
---
# Title

First paragraph.
`,
			want: "OKF description.",
		},
		{
			name: "first paragraph fallback",
			src: `# Title

First paragraph.
`,
			want: "First paragraph.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractMetadata("note.md", []byte(tt.src))
			if err != nil {
				t.Fatalf("ExtractMetadata() error = %v, want nil", err)
			}
			if got.Summary != tt.want {
				t.Fatalf("Summary = %q, want %q", got.Summary, tt.want)
			}
		})
	}
}

func TestExtractMetadataExtractsH2H3HeadingsInSourceOrder(t *testing.T) {
	source := []byte(`# Document Title

## Context

### Prior Art

#### Too Deep

## Decision

# Ignored H1

### Consequences
`)

	got, err := ExtractMetadata("headings.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	want := []Heading{
		{Level: 2, Text: "Context", Slug: "context"},
		{Level: 3, Text: "Prior Art", Slug: "prior-art"},
		{Level: 2, Text: "Decision", Slug: "decision"},
		{Level: 3, Text: "Consequences", Slug: "consequences"},
	}
	if !reflect.DeepEqual(got.Headings, want) {
		t.Fatalf("Headings = %#v, want %#v", got.Headings, want)
	}
}

func TestExtractMetadataNormalizesHeadingSlugs(t *testing.T) {
	source := []byte(`## API, Read & Write!

### Use ` + "`Code Spans`" + ` Here
`)

	got, err := ExtractMetadata("slugs.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	want := []Heading{
		{Level: 2, Text: "API, Read & Write!", Slug: "api-read-write"},
		{Level: 3, Text: "Use Code Spans Here", Slug: "use-code-spans-here"},
	}
	if !reflect.DeepEqual(got.Headings, want) {
		t.Fatalf("Headings = %#v, want %#v", got.Headings, want)
	}
}

func TestExtractMetadataCollapsesConsecutiveHeadingSlugSeparators(t *testing.T) {
	source := []byte(`## Foo  Bar

## Foo - Bar

## Foo  - -- Bar!!! Baz
`)

	got, err := ExtractMetadata("separators.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	want := []Heading{
		{Level: 2, Text: "Foo Bar", Slug: "foo-bar"},
		{Level: 2, Text: "Foo - Bar", Slug: "foo-bar-1"},
		{Level: 2, Text: "Foo - -- Bar!!! Baz", Slug: "foo-bar-baz"},
	}
	if !reflect.DeepEqual(got.Headings, want) {
		t.Fatalf("Headings = %#v, want %#v", got.Headings, want)
	}
}

func TestExtractMetadataAddsDuplicateHeadingSlugSuffixes(t *testing.T) {
	source := []byte(`## Context

### Context

## Context!

## Context-1

## Context
`)

	got, err := ExtractMetadata("duplicates.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	want := []Heading{
		{Level: 2, Text: "Context", Slug: "context"},
		{Level: 3, Text: "Context", Slug: "context-1"},
		{Level: 2, Text: "Context!", Slug: "context-2"},
		{Level: 2, Text: "Context-1", Slug: "context-1-1"},
		{Level: 2, Text: "Context", Slug: "context-3"},
	}
	if !reflect.DeepEqual(got.Headings, want) {
		t.Fatalf("Headings = %#v, want %#v", got.Headings, want)
	}
}

func TestExtractMetadataParsesBlockTags(t *testing.T) {
	source := []byte(`---
tags:
  - memento
  - markdown
---

# Title
`)

	got, err := ExtractMetadata("tags.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	if !reflect.DeepEqual(got.Tags, []string{"memento", "markdown"}) {
		t.Fatalf("Tags = %v, want block tags", got.Tags)
	}
}

func TestSummaryHashUsesBodyExcludingFrontmatter(t *testing.T) {
	body := []byte("# Title\n\nBody text.\n")
	bodyHash := hashBody(body)

	first, err := ExtractMetadata("hash.md", []byte(`---
title: First title
summary: Summary
summary_hash: `+bodyHash+`
---
`+string(body)))
	if err != nil {
		t.Fatalf("ExtractMetadata(first) error = %v, want nil", err)
	}

	second, err := ExtractMetadata("hash.md", []byte(`---
title: Changed title
summary: Summary
summary_hash: `+bodyHash+`
---
`+string(body)))
	if err != nil {
		t.Fatalf("ExtractMetadata(second) error = %v, want nil", err)
	}

	if first.BodyHash != second.BodyHash {
		t.Fatalf("BodyHash changed with frontmatter: first %q, second %q", first.BodyHash, second.BodyHash)
	}
	if first.SummaryStale || second.SummaryStale {
		t.Fatalf("SummaryStale = %v/%v, want both false for matching body hash", first.SummaryStale, second.SummaryStale)
	}
}

func TestBodyHashUsesEntireSourceWhenFrontmatterAbsent(t *testing.T) {
	source := []byte("# Title\n\nBody text.\n")

	got, err := ExtractMetadata("hash.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	if got.BodyHash != hashBody(source) {
		t.Fatalf("BodyHash = %q, want hash of markdown source %q", got.BodyHash, hashBody(source))
	}
}

func TestSummaryStaleChangesWhenBodyNoLongerMatchesStoredHash(t *testing.T) {
	originalBody := []byte("# Title\n\nOriginal body.\n")
	storedHash := hashBody(originalBody)

	got, err := ExtractMetadata("hash.md", []byte(`---
summary: Summary
summary_hash: `+storedHash+`
---
# Title

Changed body.
`))
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	if got.BodyHash == storedHash {
		t.Fatalf("BodyHash = stored hash %q, want changed body to hash differently", storedHash)
	}
	if !got.SummaryStale {
		t.Fatal("SummaryStale = false, want true when body hash differs from stored summary_hash")
	}
}

func TestExtractMetadataRejectsMalformedFrontmatter(t *testing.T) {
	tests := []struct {
		name string
		src  string
		err  error
	}{
		{
			name: "missing colon",
			src:  "---\ntitle\n---\n# Title\n",
			err:  ErrMalformedFrontmatter,
		},
		{
			name: "bad updated",
			src:  "---\nupdated: yesterday\n---\n# Title\n",
			err:  ErrInvalidUpdated,
		},
		{
			name: "bad mode",
			src:  "---\nmode: rewrite-history\n---\n# Title\n",
			err:  ErrInvalidMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExtractMetadata("bad.md", []byte(tt.src))
			if !errors.Is(err, tt.err) {
				t.Fatalf("ExtractMetadata() error = %v, want %v", err, tt.err)
			}
		})
	}
}
