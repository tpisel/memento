package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tpisel/memento/internal/markdown"
	"github.com/tpisel/memento/internal/vault"
)

type Manifest struct {
	Entries []Entry        `json:"entries"`
	Tags    map[string]int `json:"tags,omitempty"`
}

type Entry struct {
	Key          string             `json:"key"`
	Title        string             `json:"title"`
	Summary      string             `json:"summary"`
	Tags         []string           `json:"tags"`
	Headings     []Heading          `json:"headings"`
	Mode         markdown.WriteMode `json:"mode"`
	Updated      string             `json:"updated"`
	SummaryStale bool               `json:"summary_stale"`
	Links        Links              `json:"links"`
}

type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Slug  string `json:"slug"`
}

type Links struct {
	Out []OutLink `json:"out"`
	In  []InLink  `json:"in"`
}

type OutLink struct {
	Target     string                `json:"target"`
	Type       markdown.WikiLinkType `json:"type"`
	Resolved   bool                  `json:"resolved"`
	occurrence int
}

type InLink struct {
	Source string                `json:"source"`
	Type   markdown.WikiLinkType `json:"type"`
}

func Compile(v vault.Vault) (Manifest, error) {
	entries := []Entry{}
	tagCounts := map[string]int{}
	sources := map[string][]byte{}

	err := vault.WalkMarkdown(v, func(relPath, absPath string) error {
		source, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", relPath, err)
		}

		meta, err := markdown.ExtractMetadata(relPath, source)
		if err != nil {
			return fmt.Errorf("extract metadata from %s: %w", relPath, err)
		}

		tags := sortedUnique(meta.Tags)
		for _, tag := range tags {
			tagCounts[tag]++
		}

		sources[relPath] = source
		entries = append(entries, Entry{
			Key:          relPath,
			Title:        meta.Title,
			Summary:      meta.Summary,
			Tags:         tags,
			Headings:     manifestHeadings(meta.Headings),
			Mode:         meta.Mode,
			Updated:      formatUpdated(meta.Updated),
			SummaryStale: meta.SummaryStale,
			Links: Links{
				Out: []OutLink{},
				In:  []InLink{},
			},
		})
		return nil
	})
	if err != nil {
		return Manifest{}, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	populateLinkGraph(entries, sources)

	manifest := Manifest{Entries: entries}
	if len(tagCounts) > 0 {
		manifest.Tags = tagCounts
	}
	return manifest, nil
}

func Marshal(m Manifest) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func Write(v vault.Vault) error {
	m, err := Compile(v)
	if err != nil {
		return err
	}
	data, err := Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.MkdirAll(v.MarkerDir, 0o755); err != nil {
		return fmt.Errorf("create marker directory: %w", err)
	}
	if err := os.WriteFile(v.ManifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func sortedUnique(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}

	seen := map[string]bool{}
	unique := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		unique = append(unique, tag)
	}
	sort.Strings(unique)
	return unique
}

func manifestHeadings(headings []markdown.Heading) []Heading {
	out := make([]Heading, 0, len(headings))
	for _, heading := range headings {
		out = append(out, Heading{
			Level: heading.Level,
			Text:  heading.Text,
			Slug:  heading.Slug,
		})
	}
	return out
}

func formatUpdated(updated time.Time) string {
	if updated.IsZero() {
		return ""
	}
	return updated.UTC().Format(time.RFC3339)
}

func populateLinkGraph(entries []Entry, sources map[string][]byte) {
	resolver := newLinkResolver(entries)
	entryByKey := make(map[string]*Entry, len(entries))
	for i := range entries {
		entryByKey[entries[i].Key] = &entries[i]
	}

	for i := range entries {
		entries[i].Links.Out = wikiOutLinks(entries[i].Key, sources[entries[i].Key], resolver)
	}

	for i := range entries {
		for _, out := range entries[i].Links.Out {
			if !out.Resolved {
				continue
			}
			target := entryByKey[out.Target]
			if target == nil {
				continue
			}
			target.Links.In = append(target.Links.In, InLink{
				Source: entries[i].Key,
				Type:   out.Type,
			})
		}
	}

	for i := range entries {
		sort.Slice(entries[i].Links.In, func(a, b int) bool {
			if entries[i].Links.In[a].Source != entries[i].Links.In[b].Source {
				return entries[i].Links.In[a].Source < entries[i].Links.In[b].Source
			}
			return entries[i].Links.In[a].Type < entries[i].Links.In[b].Type
		})
	}
}

func wikiOutLinks(currentKey string, source []byte, resolver *linkResolver) []OutLink {
	if resolver == nil {
		return []OutLink{}
	}

	rawLinks := markdown.ExtractWikiLinks(source)
	links := make([]OutLink, 0, len(rawLinks))
	seen := map[string]bool{}
	for _, raw := range rawLinks {
		target, resolved := resolver.resolve(currentKey, raw.Target)
		link := OutLink{
			Target:     target,
			Type:       raw.Type,
			Resolved:   resolved,
			occurrence: raw.Occurrence,
		}
		key := fmt.Sprintf("%s\x00%s\x00%t", link.Target, link.Type, link.Resolved)
		if seen[key] {
			continue
		}
		seen[key] = true
		links = append(links, link)
	}

	sort.Slice(links, func(i, j int) bool {
		if links[i].Target != links[j].Target {
			return links[i].Target < links[j].Target
		}
		if links[i].occurrence != links[j].occurrence {
			return links[i].occurrence < links[j].occurrence
		}
		return links[i].Type < links[j].Type
	})
	return links
}

type linkResolver struct {
	exact     map[string]string
	basename  map[string]string
	ambiguous map[string]bool
}

func newLinkResolver(entries []Entry) *linkResolver {
	r := &linkResolver{
		exact:     map[string]string{},
		basename:  map[string]string{},
		ambiguous: map[string]bool{},
	}

	for _, entry := range entries {
		key := filepath.ToSlash(entry.Key)
		r.exact[key] = key
		if strings.HasSuffix(key, ".md") {
			r.exact[strings.TrimSuffix(key, ".md")] = key
		}

		base := path.Base(key)
		r.addBasename(base, key)
		if strings.HasSuffix(base, ".md") {
			r.addBasename(strings.TrimSuffix(base, ".md"), key)
		}
	}
	return r
}

func (r *linkResolver) addBasename(name, key string) {
	if existing, ok := r.basename[name]; ok && existing != key {
		r.ambiguous[name] = true
		delete(r.basename, name)
		return
	}
	if !r.ambiguous[name] {
		r.basename[name] = key
	}
}

func (r *linkResolver) resolve(currentKey, rawTarget string) (string, bool) {
	rawTarget = strings.TrimSpace(rawTarget)
	target := fileTarget(rawTarget)
	if target == "" {
		return currentKey, true
	}

	target = filepath.ToSlash(target)
	if key, ok := r.exact[target]; ok {
		return key, true
	}
	if !strings.Contains(target, "/") {
		if key, ok := r.basename[target]; ok {
			return key, true
		}
	}
	return rawTarget, false
}

func fileTarget(rawTarget string) string {
	target, _, _ := strings.Cut(rawTarget, "#")
	target, _, _ = strings.Cut(target, "^")
	return strings.TrimSpace(target)
}
