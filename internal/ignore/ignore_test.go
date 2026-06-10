package ignore

import (
	"errors"
	"strings"
	"testing"
)

func TestParseSkipsCommentsAndBlankLines(t *testing.T) {
	got, err := Parse(strings.NewReader(`
# generated notes

scratch.md

# daily notes
daily/
`))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if len(got) != 2 {
		t.Fatalf("Parse() returned %d patterns, want 2: %#v", len(got), got)
	}
	if got[0].Raw != "scratch.md" || got[1].Raw != "daily/" {
		t.Fatalf("Parse() patterns = %#v, want comment and blank lines skipped", got)
	}
}

func TestParseEscapedLiteralLeadingHash(t *testing.T) {
	got, err := Parse(strings.NewReader(`\#literal-heading.md`))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	want := Pattern{
		Line:         1,
		Raw:          `\#literal-heading.md`,
		Pattern:      "#literal-heading.md",
		Kind:         FilePattern,
		RootRelative: false,
		Segments: []Segment{
			{Raw: "#literal-heading.md", Kind: LiteralSegment},
		},
	}
	assertPattern(t, got, want)
}

func TestParsePatternForms(t *testing.T) {
	input := strings.Join([]string{
		"foo.md",
		"/root.md",
		"drafts/",
		"/private/",
		"*.tmp",
		"archive/**/scratch.md",
	}, "\n")

	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	want := []Pattern{
		{
			Line:    1,
			Raw:     "foo.md",
			Pattern: "foo.md",
			Kind:    FilePattern,
			Segments: []Segment{
				{Raw: "foo.md", Kind: LiteralSegment},
			},
		},
		{
			Line:         2,
			Raw:          "/root.md",
			Pattern:      "root.md",
			Kind:         FilePattern,
			RootRelative: true,
			Segments: []Segment{
				{Raw: "root.md", Kind: LiteralSegment},
			},
		},
		{
			Line:    3,
			Raw:     "drafts/",
			Pattern: "drafts",
			Kind:    DirectoryPattern,
			Segments: []Segment{
				{Raw: "drafts", Kind: LiteralSegment},
			},
		},
		{
			Line:         4,
			Raw:          "/private/",
			Pattern:      "private",
			Kind:         DirectoryPattern,
			RootRelative: true,
			Segments: []Segment{
				{Raw: "private", Kind: LiteralSegment},
			},
		},
		{
			Line:    5,
			Raw:     "*.tmp",
			Pattern: "*.tmp",
			Kind:    FilePattern,
			Segments: []Segment{
				{Raw: "*.tmp", Kind: GlobSegment},
			},
		},
		{
			Line:    6,
			Raw:     "archive/**/scratch.md",
			Pattern: "archive/**/scratch.md",
			Kind:    FilePattern,
			Segments: []Segment{
				{Raw: "archive", Kind: LiteralSegment},
				{Raw: "**", Kind: RecursiveSegment},
				{Raw: "scratch.md", Kind: LiteralSegment},
			},
		},
	}

	if len(got) != len(want) {
		t.Fatalf("Parse() returned %d patterns, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if !patternEqual(got[i], want[i]) {
			t.Fatalf("Parse()[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestParseRejectsUnsupportedNegation(t *testing.T) {
	_, err := Parse(strings.NewReader("keep.md\n!important.md\n"))
	if !errors.Is(err, ErrUnsupportedNegation) {
		t.Fatalf("Parse() error = %v, want ErrUnsupportedNegation", err)
	}

	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("Parse() error = %T, want *ParseError", err)
	}
	if parseErr.Line != 2 {
		t.Fatalf("ParseError.Line = %d, want 2", parseErr.Line)
	}
}

func TestParseRejectsEmptyRootPattern(t *testing.T) {
	_, err := Parse(strings.NewReader("/"))
	if !errors.Is(err, ErrEmptyPattern) {
		t.Fatalf("Parse() error = %v, want ErrEmptyPattern", err)
	}
}

func TestParseRejectsPartialRecursiveWildcardSegment(t *testing.T) {
	_, err := Parse(strings.NewReader("foo**bar.md"))
	if !errors.Is(err, ErrInvalidRecursiveWildcard) {
		t.Fatalf("Parse() error = %v, want ErrInvalidRecursiveWildcard", err)
	}
}

func assertPattern(t *testing.T, got []Pattern, want Pattern) {
	t.Helper()

	if len(got) != 1 {
		t.Fatalf("Parse() returned %d patterns, want 1: %#v", len(got), got)
	}
	if !patternEqual(got[0], want) {
		t.Fatalf("Parse()[0] = %#v, want %#v", got[0], want)
	}
}

func patternEqual(a, b Pattern) bool {
	if a.Line != b.Line ||
		a.Raw != b.Raw ||
		a.Pattern != b.Pattern ||
		a.Kind != b.Kind ||
		a.RootRelative != b.RootRelative ||
		len(a.Segments) != len(b.Segments) {
		return false
	}
	for i := range a.Segments {
		if a.Segments[i] != b.Segments[i] {
			return false
		}
	}
	return true
}
