package markdown

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var (
	ErrUnterminatedFrontmatter = errors.New("unterminated frontmatter")
	ErrMalformedFrontmatter    = errors.New("malformed frontmatter")
	ErrInvalidMode             = errors.New("invalid write mode")
	ErrInvalidUpdated          = errors.New("invalid updated metadata")
)

type WriteMode string

const (
	ModeAppendOnly WriteMode = "append-only"
	ModeLiving     WriteMode = "living"
	ModeReadOnly   WriteMode = "read-only"

	DefaultWriteMode = ModeAppendOnly

	ModeSectionReplace WriteMode = "section-replace"
	ModeKeyedUpsert    WriteMode = "keyed-upsert"
)

type Metadata struct {
	Title        string
	Summary      string
	Tags         []string
	Headings     []Heading
	Mode         WriteMode
	Orient       bool
	Updated      time.Time
	SummaryHash  string
	BodyHash     string
	SummaryState SummaryState
}

type Heading struct {
	Level int
	Text  string
	Slug  string
}

type SummaryState string

const (
	SummaryCurrent SummaryState = "current"
	SummaryStale   SummaryState = "stale"
	SummaryMissing SummaryState = "missing"
)

type frontmatter struct {
	title       string
	summary     string
	description string
	tags        []string
	mode        WriteMode
	orient      bool
	updated     time.Time
	summaryHash string
}

const frontmatterFenceLookaheadLines = 64

func ExtractMetadata(relPath string, source []byte) (Metadata, error) {
	fm, body, err := splitAndParseFrontmatter(source)
	if err != nil {
		return Metadata{}, err
	}

	return metadataFromParts(relPath, fm, body), nil
}

func ExtractMetadataLenient(relPath string, source []byte) (Metadata, []error, error) {
	fm, body, err := splitAndParseFrontmatter(source)
	if err != nil {
		return metadataFromParts(relPath, frontmatter{}, body), []error{err}, nil
	}

	return metadataFromParts(relPath, fm, body), nil, nil
}

func metadataFromParts(relPath string, fm frontmatter, body []byte) Metadata {
	doc := goldmark.DefaultParser().Parse(text.NewReader(body))
	bodyHash := hashBody(body)

	title := fm.title
	if title == "" {
		title = firstHeadingText(doc, body, 1)
	}
	if title == "" {
		title = filenameTitle(relPath)
	}

	committedSummary := fm.summary
	if committedSummary == "" {
		committedSummary = fm.description
	}
	summary := committedSummary
	if summary == "" {
		summary = firstParagraphText(doc, body)
	}
	summaryHash := ""
	summaryState := SummaryMissing
	if committedSummary != "" {
		summaryHash = hashSummary(committedSummary)
		summaryState = SummaryCurrent
	}

	mode := fm.mode
	if mode == "" {
		mode = DefaultWriteMode
	}

	return Metadata{
		Title:        title,
		Summary:      summary,
		Tags:         fm.tags,
		Headings:     extractHeadings(doc, body),
		Mode:         mode,
		Orient:       fm.orient,
		Updated:      fm.updated,
		SummaryHash:  summaryHash,
		BodyHash:     bodyHash,
		SummaryState: summaryState,
	}
}

func hashSummary(summary string) string {
	sum := sha256.Sum256([]byte(summary))
	return hex.EncodeToString(sum[:])
}

func splitAndParseFrontmatter(source []byte) (frontmatter, []byte, error) {
	raw, body, ok := splitFrontmatterBlock(source)
	if !ok {
		return frontmatter{}, source, nil
	}

	fm, err := parseFrontmatter(string(raw))
	if err != nil {
		return frontmatter{}, body, err
	}
	return fm, body, nil
}

func splitFrontmatterBlock(source []byte) ([]byte, []byte, bool) {
	if !hasOpeningFrontmatterFence(source) {
		return nil, nil, false
	}

	lineEnd := bytes.IndexByte(source, '\n')
	if lineEnd == -1 {
		return nil, nil, false
	}

	start := lineEnd + 1
	pos := start
	for scanned := 0; pos <= len(source) && scanned < frontmatterFenceLookaheadLines; scanned++ {
		next := bytes.IndexByte(source[pos:], '\n')
		lineEnd := len(source)
		if next >= 0 {
			lineEnd = pos + next
		}

		line := strings.TrimSpace(strings.TrimSuffix(string(source[pos:lineEnd]), "\r"))
		if line == "---" {
			bodyStart := lineEnd
			if next >= 0 {
				bodyStart++
			}
			raw := source[start:pos]
			if !hasFrontmatterContent(raw) {
				return nil, nil, false
			}
			return raw, source[bodyStart:], true
		}
		if next < 0 {
			break
		}
		pos = lineEnd + 1
	}

	return nil, nil, false
}

func hasOpeningFrontmatterFence(source []byte) bool {
	if len(source) < 3 || string(source[:3]) != "---" {
		return false
	}
	return len(source) == 3 || source[3] == '\n' || source[3] == '\r'
}

func hasFrontmatterContent(raw []byte) bool {
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return true
	}
	return false
}

func parseFrontmatter(raw string) (frontmatter, error) {
	var fm frontmatter
	lines := strings.Split(raw, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(strings.TrimSuffix(lines[i], "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return frontmatter{}, fmt.Errorf("%w: expected key-value metadata on line %d", ErrMalformedFrontmatter, i+1)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return frontmatter{}, fmt.Errorf("%w: empty key on line %d", ErrMalformedFrontmatter, i+1)
		}

		if key == "tags" && value == "" {
			tags, next, err := parseBlockTags(lines, i+1)
			if err != nil {
				return frontmatter{}, err
			}
			fm.tags = tags
			i = next - 1
			continue
		}

		if err := applyFrontmatterField(&fm, key, value); err != nil {
			return frontmatter{}, err
		}
	}

	return fm, nil
}

func parseBlockTags(lines []string, start int) ([]string, int, error) {
	var tags []string
	i := start
	for ; i < len(lines); i++ {
		raw := strings.TrimSuffix(lines[i], "\r")
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") {
			break
		}
		tag := cleanScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		if tag == "" {
			return nil, 0, fmt.Errorf("%w: empty tag on line %d", ErrMalformedFrontmatter, i+1)
		}
		tags = append(tags, tag)
	}
	return tags, i, nil
}

func applyFrontmatterField(fm *frontmatter, key, value string) error {
	switch key {
	case "title":
		fm.title = cleanScalar(value)
	case "summary":
		fm.summary = cleanScalar(value)
	case "description":
		fm.description = cleanScalar(value)
	case "tags":
		tags, err := parseInlineTags(value)
		if err != nil {
			return err
		}
		fm.tags = tags
	case "mode":
		mode := WriteMode(cleanScalar(value))
		if !validMode(mode) {
			return fmt.Errorf("%w: %q", ErrInvalidMode, value)
		}
		fm.mode = mode
	case "orient":
		orient, err := parseBool(value)
		if err != nil {
			return err
		}
		fm.orient = orient
	case "updated":
		updated, err := parseUpdated(cleanScalar(value))
		if err != nil {
			return err
		}
		fm.updated = updated
	case "summary_hash":
		fm.summaryHash = cleanScalar(value)
	case "type", "resource", "timestamp", "okf_version":
		// OKF convention fields are accepted and ignored by design.
	default:
		// Unknown frontmatter is durable human metadata; preserve compatibility by ignoring it.
	}
	return nil
}

func parseInlineTags(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	if strings.HasPrefix(value, "[") {
		if !strings.HasSuffix(value, "]") {
			return nil, fmt.Errorf("%w: malformed inline tags", ErrMalformedFrontmatter)
		}
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	}

	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := cleanScalar(strings.TrimSpace(part))
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

func parseUpdated(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.DateOnly, value); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidUpdated, value)
}

func parseBool(value string) (bool, error) {
	switch cleanScalar(value) {
	case "true":
		return true, nil
	case "false", "":
		return false, nil
	default:
		return false, fmt.Errorf("%w: invalid boolean %q", ErrMalformedFrontmatter, value)
	}
}

func validMode(mode WriteMode) bool {
	switch mode {
	case ModeAppendOnly, ModeLiving, ModeSectionReplace, ModeKeyedUpsert, ModeReadOnly:
		return true
	default:
		return false
	}
}

func cleanScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func firstHeadingText(doc ast.Node, source []byte, level int) string {
	var heading string
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || heading != "" {
			return ast.WalkContinue, nil
		}
		h, ok := node.(*ast.Heading)
		if !ok || h.Level != level {
			return ast.WalkContinue, nil
		}
		heading = strings.TrimSpace(nodeText(h, source))
		return ast.WalkStop, nil
	})
	return heading
}

func firstParagraphText(doc ast.Node, source []byte) string {
	var paragraph string
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || paragraph != "" {
			return ast.WalkContinue, nil
		}
		p, ok := node.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		paragraph = strings.TrimSpace(nodeText(p, source))
		if paragraph == "" {
			return ast.WalkContinue, nil
		}
		return ast.WalkStop, nil
	})
	return paragraph
}

func extractHeadings(doc ast.Node, source []byte) []Heading {
	var headings []Heading
	nextSuffix := map[string]int{}
	used := map[string]bool{}

	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := node.(*ast.Heading)
		if !ok || h.Level < 2 || h.Level > 3 {
			return ast.WalkContinue, nil
		}

		text := strings.TrimSpace(nodeText(h, source))
		if text == "" {
			return ast.WalkContinue, nil
		}
		headings = append(headings, Heading{
			Level: h.Level,
			Text:  text,
			Slug:  uniqueHeadingSlug(headingSlug(text), nextSuffix, used),
		})
		return ast.WalkContinue, nil
	})

	return headings
}

func uniqueHeadingSlug(base string, nextSuffix map[string]int, used map[string]bool) string {
	if !used[base] {
		used[base] = true
		return base
	}

	for {
		nextSuffix[base]++
		slug := fmt.Sprintf("%s-%d", base, nextSuffix[base])
		if !used[slug] {
			used[slug] = true
			return slug
		}
	}
}

func headingSlug(text string) string {
	var b strings.Builder
	lastWasSeparator := false
	for _, r := range strings.TrimSpace(text) {
		switch {
		case unicode.IsSpace(r), r == '-':
			if b.Len() == 0 || lastWasSeparator {
				continue
			}
			b.WriteByte('-')
			lastWasSeparator = true
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_':
			b.WriteRune(unicode.ToLower(r))
			lastWasSeparator = false
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

func nodeText(node ast.Node, source []byte) string {
	var b strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		appendNodeText(&b, child, source)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func appendNodeText(b *strings.Builder, node ast.Node, source []byte) {
	switch n := node.(type) {
	case *ast.Text:
		b.Write(n.Text(source))
		if n.SoftLineBreak() || n.HardLineBreak() {
			b.WriteByte(' ')
		}
	case *ast.String:
		b.Write(n.Value)
	case *ast.CodeSpan:
		b.Write(n.Text(source))
	default:
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			appendNodeText(b, child, source)
		}
	}
}

func filenameTitle(relPath string) string {
	base := filepath.Base(filepath.ToSlash(relPath))
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func hashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
