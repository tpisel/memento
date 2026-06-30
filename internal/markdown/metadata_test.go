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
mode: living
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
	if got.Mode != ModeLiving {
		t.Fatalf("Mode = %q, want %q", got.Mode, ModeLiving)
	}
	if !got.Orient {
		t.Fatal("Orient = false, want true from frontmatter")
	}
	if !got.Updated.Equal(wantUpdated) {
		t.Fatalf("Updated = %v, want %v", got.Updated, wantUpdated)
	}
	if got.SummaryTextHash != hashSummary("A concise summary from frontmatter.") {
		t.Fatalf("SummaryTextHash = %q, want hash of committed summary", got.SummaryTextHash)
	}
	if got.BodyHash == "" {
		t.Fatal("BodyHash is empty, want deterministic body hash")
	}
	if got.SummaryState != SummaryCurrent {
		t.Fatalf("SummaryState = %q, want %q", got.SummaryState, SummaryCurrent)
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
	if got.SummaryTextHash != "" {
		t.Fatalf("SummaryTextHash = %q, want empty without summary or description", got.SummaryTextHash)
	}
	if got.SummaryState != SummaryMissing {
		t.Fatalf("SummaryState = %q, want %q", got.SummaryState, SummaryMissing)
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

func TestExtractMetadataParsesBlockSequenceUnderNonTagsKey(t *testing.T) {
	// Standard YAML block sequences must parse under ANY key, not just tags.
	// This is the form Obsidian's property editor emits for list fields; rejecting
	// it discarded the whole frontmatter and silently downgraded the note's mode
	// (memento-dl5). The block sequence must be fully consumed so a following key
	// (here mode) still parses.
	source := []byte(`---
scope:
  - theory
  - modelling doctrine
mode: living
---

# Title
`)

	got, err := ExtractMetadata("scope.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}
	if got.Mode != ModeLiving {
		t.Fatalf("Mode = %q, want %q (block sequence not consumed before following key)", got.Mode, ModeLiving)
	}
}

func TestExtractMetadataAcceptsInlineFlowUnderNonTagsKey(t *testing.T) {
	// The inline flow form already parsed; guard that it keeps doing so and that a
	// following key is still read.
	source := []byte(`---
scope: ["theory", "modelling doctrine"]
mode: living
---

# Title
`)

	got, err := ExtractMetadata("scope.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}
	if got.Mode != ModeLiving {
		t.Fatalf("Mode = %q, want %q", got.Mode, ModeLiving)
	}
}

func TestExtractMetadataEmptyTagsKeyWithNoSequenceItems(t *testing.T) {
	// `tags:` declared with an empty value and NO following `- ` items is the one
	// behavioral edge the parseBlockTags -> parseBlockSequence generalization
	// introduced: found=false routes the empty value to applyFrontmatterField
	// (parseInlineTags("")) rather than the old empty-list path (memento-dl5).
	// Lock in that this parses cleanly to no tags and still consumes the key so a
	// following key (mode) is read.
	source := []byte(`---
title: X
tags:
mode: living
---

# Title
`)

	got, err := ExtractMetadata("empty-tags.md", source)
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("Tags = %v, want empty", got.Tags)
	}
	if got.Mode != ModeLiving {
		t.Fatalf("Mode = %q, want %q (empty tags key not consumed before following key)", got.Mode, ModeLiving)
	}
}

func TestBodyHashUsesBodyExcludingFrontmatter(t *testing.T) {
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
	if first.BodyHash != bodyHash || second.BodyHash != bodyHash {
		t.Fatalf("BodyHash = %q/%q, want %q", first.BodyHash, second.BodyHash, bodyHash)
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

func TestSummaryTextHashUsesCommittedSummaryNotLegacyField(t *testing.T) {
	got, err := ExtractMetadata("hash.md", []byte(`---
summary: Summary
summary_hash: legacy-body-hash
---
# Title

Changed body.
`))
	if err != nil {
		t.Fatalf("ExtractMetadata() error = %v, want nil", err)
	}

	if got.SummaryTextHash != hashSummary("Summary") {
		t.Fatalf("SummaryTextHash = %q, want hash of summary text", got.SummaryTextHash)
	}
	if got.SummaryState != SummaryCurrent {
		t.Fatalf("SummaryState = %q, want %q", got.SummaryState, SummaryCurrent)
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
		{
			name: "retired section-replace mode",
			src:  "---\nmode: section-replace\n---\n# Title\n",
			err:  ErrInvalidMode,
		},
		{
			name: "retired keyed-upsert mode",
			src:  "---\nmode: keyed-upsert\n---\n# Title\n",
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

func TestExtractMetadataLenientFlagsUnparsedMode(t *testing.T) {
	// A note that declares mode: living but carries a malformed line. The whole
	// frontmatter fails to parse; lenient extraction must NOT silently fall back to
	// the append-only default (that would invert the author's intent and lock the
	// note tighter than written — memento-o0a). It resolves to the unparsed sentinel.
	src := []byte("---\nmode: living\ntitle\n---\n# Title\n\nBody.\n")

	meta, errs, err := ExtractMetadataLenient("notes/broken.md", src)
	if err != nil {
		t.Fatalf("ExtractMetadataLenient() error = %v, want nil", err)
	}
	if len(errs) != 1 {
		t.Fatalf("ExtractMetadataLenient() parse errors = %d, want 1: %v", len(errs), errs)
	}
	if meta.Mode != ModeUnparsed {
		t.Fatalf("Mode = %q, want %q (never silently %q)", meta.Mode, ModeUnparsed, ModeAppendOnly)
	}
	// The fallback title/summary still come through so the brief can render the note.
	if meta.Title != "Title" {
		t.Fatalf("Title = %q, want fallback H1 %q", meta.Title, "Title")
	}
}

func TestUnparsedModeIsNotDeclarable(t *testing.T) {
	// The sentinel must never be a value an author can write: declaring it is a
	// plain invalid mode, and the strict extractor rejects the whole note.
	if validMode(ModeUnparsed) {
		t.Fatalf("validMode(%q) = true, want false: the unparsed sentinel must not be declarable", ModeUnparsed)
	}
	if _, err := ExtractMetadata("bad.md", []byte("---\nmode: unparsed\n---\n# T\n")); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("ExtractMetadata() error = %v, want ErrInvalidMode for a declared unparsed mode", err)
	}
}

func TestFrontmatterScalar(t *testing.T) {
	source := []byte("---\ntitle: Plain value\nquoted: \"double quoted\"\nsingle: 'single quoted'\n# comment: ignored\nempty:   \nwhen_to_read: before a write\n---\nbody\n")
	front, _, ok := SplitFrontmatter(source)
	if !ok {
		t.Fatalf("SplitFrontmatter() ok = false, want true")
	}

	cases := []struct {
		key  string
		want string
	}{
		{"title", "Plain value"},
		{"quoted", "double quoted"},
		{"single", "single quoted"},
		{"comment", ""},
		{"empty", ""},
		{"when_to_read", "before a write"},
		{"absent", ""},
	}
	for _, tc := range cases {
		if got := FrontmatterScalar(front, tc.key); got != tc.want {
			t.Errorf("FrontmatterScalar(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}

	if got := FrontmatterScalar(nil, "title"); got != "" {
		t.Errorf("FrontmatterScalar(nil) = %q, want empty", got)
	}
}

func TestFrontmatterScalarRejectsBlockScalar(t *testing.T) {
	// A block scalar header (| or >, with optional chomping/indentation
	// indicators) has no inline value; the single-line reader reports it absent
	// rather than returning the bare indicator as the value.
	for _, indicator := range []string{"|", ">", "|-", "|+", ">-", "|2", "|2-"} {
		front := []byte("key: " + indicator + "\n  body line\n")
		if got := FrontmatterScalar(front, "key"); got != "" {
			t.Errorf("FrontmatterScalar(key: %q) = %q, want empty", indicator, got)
		}
	}

	// A plain value that merely starts with | or > but carries real text stays a
	// value; only a lone indicator is treated as a block-scalar header.
	if got := FrontmatterScalar([]byte("key: > redirect\n"), "key"); got != "> redirect" {
		t.Errorf("FrontmatterScalar(key: > redirect) = %q, want %q", got, "> redirect")
	}
}
