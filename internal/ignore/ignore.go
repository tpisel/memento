package ignore

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrUnsupportedNegation      = errors.New("unsupported negation pattern")
	ErrEmptyPattern             = errors.New("empty ignore pattern")
	ErrEmptySegment             = errors.New("empty ignore pattern segment")
	ErrInvalidRecursiveWildcard = errors.New("recursive wildcard must be its own path segment")
)

type PatternKind string

const (
	FilePattern      PatternKind = "file"
	DirectoryPattern PatternKind = "directory"
)

type SegmentKind string

const (
	LiteralSegment   SegmentKind = "literal"
	GlobSegment      SegmentKind = "glob"
	RecursiveSegment SegmentKind = "recursive"
)

type Pattern struct {
	Line         int
	Raw          string
	Pattern      string
	Kind         PatternKind
	RootRelative bool
	Segments     []Segment
}

type Segment struct {
	Raw  string
	Kind SegmentKind
}

type ParseError struct {
	Line    int
	Pattern string
	Err     error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf(".mementoignore:%d: %v: %q", e.Line, e.Err, e.Pattern)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

func Parse(r io.Reader) ([]Pattern, error) {
	scanner := bufio.NewScanner(r)
	var patterns []Pattern
	line := 0

	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}

		pattern, err := parseLine(line, raw)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, pattern)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read .mementoignore: %w", err)
	}

	return patterns, nil
}

func parseLine(line int, raw string) (Pattern, error) {
	if strings.HasPrefix(raw, "!") {
		return Pattern{}, lineError(line, raw, ErrUnsupportedNegation)
	}

	text := raw
	if strings.HasPrefix(text, `\#`) || strings.HasPrefix(text, `\!`) {
		text = text[1:]
	}

	rootRelative := strings.HasPrefix(text, "/")
	if rootRelative {
		text = strings.TrimPrefix(text, "/")
	}

	kind := FilePattern
	if strings.HasSuffix(text, "/") {
		kind = DirectoryPattern
		text = strings.TrimSuffix(text, "/")
	}
	if text == "" {
		return Pattern{}, lineError(line, raw, ErrEmptyPattern)
	}

	segments, err := parseSegments(text)
	if err != nil {
		return Pattern{}, lineError(line, raw, err)
	}

	return Pattern{
		Line:         line,
		Raw:          raw,
		Pattern:      text,
		Kind:         kind,
		RootRelative: rootRelative,
		Segments:     segments,
	}, nil
}

func parseSegments(pattern string) ([]Segment, error) {
	parts := strings.Split(pattern, "/")
	segments := make([]Segment, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			return nil, ErrEmptySegment
		}

		kind := LiteralSegment
		switch {
		case part == "**":
			kind = RecursiveSegment
		case strings.Contains(part, "**"):
			return nil, ErrInvalidRecursiveWildcard
		case strings.Contains(part, "*"):
			kind = GlobSegment
		}
		segments = append(segments, Segment{Raw: part, Kind: kind})
	}

	return segments, nil
}

func lineError(line int, pattern string, err error) error {
	return &ParseError{
		Line:    line,
		Pattern: pattern,
		Err:     err,
	}
}
