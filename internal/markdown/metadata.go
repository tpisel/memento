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

	// ModeUnparsed is the sentinel an unparseable frontmatter resolves to. It is
	// NOT a declarable mode (validMode rejects it) and NEVER the append-only
	// default: a parse error must not quietly make a note more locked-down — nor
	// invert a declared `mode: living` — than its author wrote (memento-o0a).
	// Enforcement treats it as the most-restrictive read-only so a note of unknown
	// intent cannot be edited or lost until its frontmatter is fixed, while brief
	// and compile surface it loudly rather than burying it behind a green compile.
	ModeUnparsed WriteMode = "unparsed"

	DefaultWriteMode = ModeAppendOnly
)

type Metadata struct {
	Title           string
	Summary         string
	Tags            []string
	Headings        []Heading
	Mode            WriteMode
	Orient          bool
	Updated         time.Time
	SummaryTextHash string
	BodyHash        string
	SummaryState    SummaryState
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
		// A parse error discards the entire frontmatter, including any declared
		// mode. Do NOT let metadataFromParts apply the append-only default: that
		// silently tightens (or inverts) the author's intent and couples a parser
		// bug to the enforcement layer (memento-o0a). Flag the mode unparsed so the
		// verdict engine fails closed to read-only and brief/compile surface it.
		meta := metadataFromParts(relPath, frontmatter{}, body)
		meta.Mode = ModeUnparsed
		return meta, []error{err}, nil
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
	summaryTextHash := ""
	summaryState := SummaryMissing
	if committedSummary != "" {
		summaryTextHash = hashSummary(committedSummary)
		summaryState = SummaryCurrent
	}

	mode := fm.mode
	if mode == "" {
		mode = DefaultWriteMode
	}

	return Metadata{
		Title:           title,
		Summary:         summary,
		Tags:            fm.tags,
		Headings:        extractHeadings(doc, body),
		Mode:            mode,
		Orient:          fm.orient,
		Updated:         fm.updated,
		SummaryTextHash: summaryTextHash,
		BodyHash:        bodyHash,
		SummaryState:    summaryState,
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

// SplitFrontmatter separates a leading YAML frontmatter block from the body.
// It reports whether a well-formed frontmatter fence was found; when false the
// full source is returned as the body and the frontmatter slice is nil.
func SplitFrontmatter(source []byte) (front []byte, body []byte, ok bool) {
	front, body, ok = splitFrontmatterBlock(source)
	if !ok {
		return nil, source, false
	}
	return front, body, true
}

// FrontmatterScalar returns the trimmed, unquoted value of a single-line scalar
// field in a raw frontmatter block (as returned by SplitFrontmatter), or "" when
// the field is absent or empty. It shares the comment-skipping line scan and
// scalar cleaning used by the metadata parser, so callers reading ad-hoc keys
// (e.g. the convention package) need not reimplement either.
//
// Only single-line scalars are read. A YAML block scalar header (a lone "|" or
// ">" indicator, with optional chomping/indentation modifiers) carries its value
// on indented lines this reader does not gather, so it is reported absent (""):
// returning the bare indicator would bind the field to "|"/">", which no caller
// wants. Convention files depend on this so a block-scalar when_to_read is
// rejected as invalid (ADR-0029) rather than silently accepted.
func FrontmatterScalar(front []byte, key string) string {
	for _, line := range strings.Split(string(front), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		v = cleanScalar(v)
		if isBlockScalarHeader(v) {
			return ""
		}
		return v
	}
	return ""
}

// isBlockScalarHeader reports whether value is a lone YAML block scalar
// indicator: a leading "|" or ">" followed only by chomping ("+"/"-") or
// explicit-indentation (1-9) modifiers. Such a header introduces a multi-line
// block body that the single-line scalar reader cannot capture.
func isBlockScalarHeader(value string) bool {
	if value == "" || (value[0] != '|' && value[0] != '>') {
		return false
	}
	for _, r := range value[1:] {
		if r != '+' && r != '-' && !(r >= '1' && r <= '9') {
			return false
		}
	}
	return true
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

		if value == "" {
			items, next, found, err := parseBlockSequence(lines, i+1)
			if err != nil {
				return frontmatter{}, err
			}
			if found {
				applyFrontmatterListField(&fm, key, items)
				i = next - 1
				continue
			}
		}

		if err := applyFrontmatterField(&fm, key, value); err != nil {
			return frontmatter{}, err
		}
	}

	return fm, nil
}

// parseBlockSequence consumes a YAML block sequence ("- item" lines) under a key
// whose inline value was empty, beginning at line index start. It returns the
// captured items, the index of the first line past the sequence, and whether any
// item was found. found=false means the key carried no block sequence (the next
// line is another key or the fence), so the caller treats the empty value as a
// scalar. Block sequences are standard YAML and the form Obsidian's property
// editor emits, so they must parse under any key, not just tags (memento-dl5).
func parseBlockSequence(lines []string, start int) (items []string, next int, found bool, err error) {
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
		item := cleanScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		if item == "" {
			return nil, 0, false, fmt.Errorf("%w: empty block-sequence item on line %d", ErrMalformedFrontmatter, i+1)
		}
		items = append(items, item)
	}
	return items, i, len(items) > 0, nil
}

// applyFrontmatterListField stores a block-sequence value. tags is the only typed
// list field; a block sequence under any other key is durable human metadata with
// no typed home, so it is captured-and-ignored, mirroring how unknown scalar keys
// are preserved-by-ignoring. The point is to accept standard YAML list syntax, not
// to reject it (memento-dl5).
func applyFrontmatterListField(fm *frontmatter, key string, items []string) {
	switch key {
	case "tags":
		fm.tags = items
	default:
		// Non-tags list metadata: accepted and ignored by design.
	}
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
	case ModeAppendOnly, ModeLiving, ModeReadOnly:
		return true
	default:
		return false
	}
}

// ParseWriteMode validates s as one of the three write modes (ADR-0015) and
// returns it. Unknown values are rejected with ErrInvalidMode rather than
// defaulted, closing ADR-0015's typo open-question for the write-mode verb.
func ParseWriteMode(s string) (WriteMode, error) {
	mode := WriteMode(strings.TrimSpace(s))
	if !validMode(mode) {
		return "", fmt.Errorf("%w: %q", ErrInvalidMode, s)
	}
	return mode, nil
}

// SetMode returns source with its frontmatter mode: line set to mode. An
// existing mode line is rewritten in place; otherwise a mode line is inserted
// (creating a frontmatter block when source has none). Every other frontmatter
// line and the body are preserved verbatim. It is the durable mutation behind
// the write-mode verb, the only path that may change an existing note's mode.
func SetMode(source []byte, mode WriteMode) []byte {
	front, body, ok := SplitFrontmatter(source)
	if !ok {
		var b bytes.Buffer
		b.WriteString("---\nmode: ")
		b.WriteString(string(mode))
		b.WriteString("\n---\n")
		b.Write(source)
		return b.Bytes()
	}

	lines := strings.Split(string(front), "\n")
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, found := strings.Cut(trimmed, ":")
		if !found || strings.TrimSpace(key) != "mode" {
			continue
		}
		lines[i] = "mode: " + string(mode)
		replaced = true
		break
	}
	if !replaced {
		// front always ends with a trailing newline, so the last split element is
		// an empty string; insert the mode line before it to keep the block clean.
		insertAt := len(lines)
		if insertAt > 0 && lines[insertAt-1] == "" {
			insertAt--
		}
		lines = append(lines[:insertAt], append([]string{"mode: " + string(mode)}, lines[insertAt:]...)...)
	}

	var b bytes.Buffer
	b.WriteString("---\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("---\n")
	b.Write(body)
	return b.Bytes()
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
