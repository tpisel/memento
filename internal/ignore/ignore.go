package ignore

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path"
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

// Matches reports whether any parsed ignore pattern matches a vault-relative path.
func Matches(patterns []Pattern, vaultRelativePath string, isDir bool) bool {
	normalized, ok := NormalizePath(vaultRelativePath)
	if !ok {
		return false
	}

	for _, pattern := range patterns {
		if pattern.Matches(normalized, isDir) {
			return true
		}
	}
	return false
}

// Matches reports whether a single ignore pattern matches a normalized vault-relative path.
func (p Pattern) Matches(vaultRelativePath string, isDir bool) bool {
	normalized, ok := NormalizePath(vaultRelativePath)
	if !ok {
		return false
	}
	pathSegments := strings.Split(normalized, "/")

	if p.RootRelative {
		return p.matchesFrom(pathSegments, isDir)
	}

	for start := range pathSegments {
		if p.matchesFrom(pathSegments[start:], isDir) {
			return true
		}
	}
	return false
}

// NormalizePath converts a path to memento's deterministic vault-relative slash form.
func NormalizePath(vaultRelativePath string) (string, bool) {
	if vaultRelativePath == "" {
		return "", false
	}

	slashed := strings.ReplaceAll(vaultRelativePath, `\`, "/")
	if strings.HasPrefix(slashed, "/") {
		return "", false
	}

	normalized := path.Clean(slashed)
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", false
	}
	return normalized, true
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

func (p Pattern) matchesFrom(pathSegments []string, isDir bool) bool {
	matchLengths := segmentMatchLengths(p.Segments, pathSegments)
	switch p.Kind {
	case FilePattern:
		for _, length := range matchLengths {
			if length == len(pathSegments) && !isDir {
				return true
			}
		}
	case DirectoryPattern:
		for _, length := range matchLengths {
			if length == 0 {
				continue
			}
			if length < len(pathSegments) || isDir {
				return true
			}
		}
	}
	return false
}

func segmentMatchLengths(patternSegments []Segment, pathSegments []string) []int {
	if len(patternSegments) == 0 {
		return []int{0}
	}

	head := patternSegments[0]
	tail := patternSegments[1:]
	if head.Kind == RecursiveSegment {
		var lengths []int
		for consumed := 0; consumed <= len(pathSegments); consumed++ {
			for _, tailLength := range segmentMatchLengths(tail, pathSegments[consumed:]) {
				lengths = append(lengths, consumed+tailLength)
			}
		}
		return lengths
	}

	if len(pathSegments) == 0 || !segmentMatches(head, pathSegments[0]) {
		return nil
	}

	var lengths []int
	for _, tailLength := range segmentMatchLengths(tail, pathSegments[1:]) {
		lengths = append(lengths, 1+tailLength)
	}
	return lengths
}

func segmentMatches(pattern Segment, pathSegment string) bool {
	switch pattern.Kind {
	case LiteralSegment:
		return pattern.Raw == pathSegment
	case GlobSegment:
		matched, err := path.Match(pattern.Raw, pathSegment)
		return err == nil && matched
	case RecursiveSegment:
		return true
	default:
		return false
	}
}

func lineError(line int, pattern string, err error) error {
	return &ParseError{
		Line:    line,
		Pattern: pattern,
		Err:     err,
	}
}
