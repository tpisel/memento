package markdown

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var ErrSectionNotFound = errors.New("section not found")

type sectionRange struct {
	Level int
	Text  string
	Slug  string
	Start int
	End   int
}

func ExtractSection(source []byte, section string) ([]byte, error) {
	section = strings.TrimSpace(section)
	if section == "" {
		return nil, fmt.Errorf("%w: empty section", ErrSectionNotFound)
	}

	_, body, err := splitAndParseFrontmatter(source)
	if err != nil {
		return nil, err
	}
	bodyOffset := len(source) - len(body)
	sections := extractSectionRanges(body)

	for _, candidate := range sections {
		if section != candidate.Text && section != candidate.Slug {
			continue
		}
		start := bodyOffset + candidate.Start
		end := bodyOffset + candidate.End
		return source[start:end], nil
	}

	return nil, fmt.Errorf("%w: %s", ErrSectionNotFound, section)
}

func extractSectionRanges(source []byte) []sectionRange {
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))

	var ranges []sectionRange
	nextSuffix := map[string]int{}
	used := map[string]bool{}

	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		h, ok := node.(*ast.Heading)
		if !ok || h.Level < 2 || h.Level > 3 || h.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}

		headingText := strings.TrimSpace(nodeText(h, source))
		if headingText == "" {
			return ast.WalkContinue, nil
		}

		ranges = append(ranges, sectionRange{
			Level: h.Level,
			Text:  headingText,
			Slug:  uniqueHeadingSlug(headingSlug(headingText), nextSuffix, used),
			Start: lineStart(source, h.Lines().At(0).Start),
			End:   len(source),
		})
		return ast.WalkContinue, nil
	})

	for i := range ranges {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[j].Level <= ranges[i].Level {
				ranges[i].End = ranges[j].Start
				break
			}
		}
	}

	return ranges
}

func lineStart(source []byte, pos int) int {
	if pos > len(source) {
		pos = len(source)
	}
	for pos > 0 && source[pos-1] != '\n' {
		pos--
	}
	return pos
}
