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

func TestMatchesAnywhereFilePattern(t *testing.T) {
	patterns := mustParse(t, "foo.md")

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{name: "root file", path: "foo.md", want: true},
		{name: "nested file", path: "notes/foo.md", want: true},
		{name: "deep nested file", path: "notes/archive/foo.md", want: true},
		{name: "different filename", path: "notes/bar.md", want: false},
		{name: "directory with same name", path: "foo.md", isDir: true, want: false},
		{name: "descendant under same name", path: "foo.md/child.md", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(patterns, tt.path, tt.isDir); got != tt.want {
				t.Fatalf("Matches(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestMatchesRootRelativeFilePattern(t *testing.T) {
	patterns := mustParse(t, "/foo.md")

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{name: "root file", path: "foo.md", want: true},
		{name: "normalized root file", path: "./foo.md", want: true},
		{name: "nested file", path: "notes/foo.md", want: false},
		{name: "directory with same name", path: "foo.md", isDir: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(patterns, tt.path, tt.isDir); got != tt.want {
				t.Fatalf("Matches(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestMatchesAnywhereDirectoryPatternRecursively(t *testing.T) {
	patterns := mustParse(t, "drafts/")

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{name: "root directory", path: "drafts", isDir: true, want: true},
		{name: "root descendant file", path: "drafts/note.md", want: true},
		{name: "nested directory", path: "notes/drafts", isDir: true, want: true},
		{name: "nested descendant file", path: "notes/drafts/note.md", want: true},
		{name: "file with directory name", path: "drafts", want: false},
		{name: "partial segment", path: "old-drafts/note.md", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(patterns, tt.path, tt.isDir); got != tt.want {
				t.Fatalf("Matches(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestMatchesRootRelativeDirectoryPatternRecursively(t *testing.T) {
	patterns := mustParse(t, "/private/")

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{name: "root directory", path: "private", isDir: true, want: true},
		{name: "root descendant file", path: "private/note.md", want: true},
		{name: "nested directory", path: "notes/private", isDir: true, want: false},
		{name: "nested descendant file", path: "notes/private/note.md", want: false},
		{name: "file with directory name", path: "private", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(patterns, tt.path, tt.isDir); got != tt.want {
				t.Fatalf("Matches(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestMatchesSegmentGlobs(t *testing.T) {
	patterns := mustParse(t, "*.tmp\nnotes/*-draft.md")

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "root suffix glob", path: "scratch.tmp", want: true},
		{name: "nested suffix glob", path: "notes/scratch.tmp", want: true},
		{name: "multi segment glob", path: "notes/spec-draft.md", want: true},
		{name: "multi segment glob nested anywhere", path: "archive/notes/spec-draft.md", want: true},
		{name: "glob does not cross segment", path: "notes/deep/spec-draft.md", want: false},
		{name: "wrong suffix", path: "notes/spec-final.md", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(patterns, tt.path, false); got != tt.want {
				t.Fatalf("Matches(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchesRecursiveWildcard(t *testing.T) {
	patterns := mustParse(t, "archive/**/scratch.md")

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "zero recursive segments", path: "archive/scratch.md", want: true},
		{name: "one recursive segment", path: "archive/2026/scratch.md", want: true},
		{name: "many recursive segments", path: "archive/2026/june/scratch.md", want: true},
		{name: "unrooted pattern matches nested suffix", path: "notes/archive/2026/scratch.md", want: true},
		{name: "different leaf", path: "archive/2026/other.md", want: false},
		{name: "different prefix", path: "archives/2026/scratch.md", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(patterns, tt.path, false); got != tt.want {
				t.Fatalf("Matches(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchesNormalizesVaultRelativePaths(t *testing.T) {
	patterns := mustParse(t, "/notes/scratch.md")

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "clean path", path: "notes/./daily/../scratch.md", want: true},
		{name: "windows separators", path: `notes\scratch.md`, want: true},
		{name: "absolute path rejected", path: "/notes/scratch.md", want: false},
		{name: "parent escape rejected", path: "../notes/scratch.md", want: false},
		{name: "empty path rejected", path: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(patterns, tt.path, false); got != tt.want {
				t.Fatalf("Matches(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func mustParse(t *testing.T, input string) []Pattern {
	t.Helper()

	patterns, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse(%q) error = %v, want nil", input, err)
	}
	return patterns
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
