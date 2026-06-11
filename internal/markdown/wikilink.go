package markdown

import (
	"bytes"
	"strings"
)

type WikiLinkType string

const (
	WikiLinkTypeWiki  WikiLinkType = "wiki"
	WikiLinkTypeEmbed WikiLinkType = "embed"
)

type WikiLink struct {
	Target     string
	Type       WikiLinkType
	Occurrence int
}

func ExtractWikiLinks(source []byte) []WikiLink {
	_, body, err := splitAndParseFrontmatter(source)
	if err != nil {
		body = source
	}

	var links []WikiLink
	for pos := 0; pos < len(body); {
		start := bytes.Index(body[pos:], []byte("[["))
		if start < 0 {
			break
		}
		start += pos

		end := bytes.Index(body[start+2:], []byte("]]"))
		if end < 0 {
			break
		}
		end += start + 2

		raw := strings.TrimSpace(string(body[start+2 : end]))
		target, _, _ := strings.Cut(raw, "|")
		target = strings.TrimSpace(target)
		if target != "" {
			linkType := WikiLinkTypeWiki
			if start > 0 && body[start-1] == '!' {
				linkType = WikiLinkTypeEmbed
			}
			links = append(links, WikiLink{
				Target:     target,
				Type:       linkType,
				Occurrence: len(links),
			})
		}
		pos = end + 2
	}
	return links
}
