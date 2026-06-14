package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
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
	SchemaVersion int            `json:"schema_version"`
	Entries       []Entry        `json:"entries"`
	Tags          map[string]int `json:"tags,omitempty"`
}

const CurrentSchemaVersion = 1

type Warning struct {
	Path string
	Err  error
}

func (w Warning) Error() string {
	return fmt.Sprintf("%s: %v", w.Path, w.Err)
}

type Entry struct {
	Key          string             `json:"key"`
	Title        string             `json:"title"`
	Summary      string             `json:"summary"`
	Bytes        int64              `json:"bytes"`
	Lines        int                `json:"lines"`
	Tags         []string           `json:"tags"`
	Headings     []Heading          `json:"headings"`
	Mode         markdown.WriteMode `json:"mode"`
	Orient       bool               `json:"orient"`
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
	m, _, err := compile(v)
	return m, err
}

func CompileWithWarnings(v vault.Vault) (Manifest, []Warning, error) {
	return compile(v)
}

func compile(v vault.Vault) (Manifest, []Warning, error) {
	entries := []Entry{}
	warnings := []Warning{}
	tagCounts := map[string]int{}
	sources := map[string][]byte{}

	err := vault.WalkMarkdown(v, func(relPath, absPath string) error {
		source, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", relPath, err)
		}

		meta, metadataWarnings, err := markdown.ExtractMetadataLenient(relPath, source)
		if err != nil {
			return fmt.Errorf("extract metadata from %s: %w", relPath, err)
		}
		for _, warning := range metadataWarnings {
			warnings = append(warnings, Warning{Path: relPath, Err: warning})
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
			Bytes:        int64(len(source)),
			Lines:        bytes.Count(source, []byte("\n")),
			Tags:         tags,
			Headings:     manifestHeadings(meta.Headings),
			Mode:         meta.Mode,
			Orient:       meta.Orient,
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
		return Manifest{}, nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	populateLinkGraph(entries, sources)

	manifest := Manifest{
		SchemaVersion: CurrentSchemaVersion,
		Entries:       entries,
	}
	if len(tagCounts) > 0 {
		manifest.Tags = tagCounts
	}
	return manifest, warnings, nil
}

func Decode(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: decode manifest: %v", ErrInvalid, err)
	}
	if m.SchemaVersion != CurrentSchemaVersion {
		return Manifest{}, fmt.Errorf("%w: schema_version %d is unsupported; supported schema_version is %d", ErrSchemaUnsupported, m.SchemaVersion, CurrentSchemaVersion)
	}
	return m, nil
}

func Load(v vault.Vault) (Manifest, error) {
	data, err := os.ReadFile(v.ManifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Manifest{}, fmt.Errorf("%w: missing at %s", ErrNotFound, v.ManifestPath)
		}
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	m, err := Decode(data)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			return Manifest{}, fmt.Errorf("%w: decode %s: %v", ErrInvalid, v.ManifestPath, err)
		}
		if errors.Is(err, ErrSchemaUnsupported) {
			return Manifest{}, fmt.Errorf("%w: %s", err, v.ManifestPath)
		}
		return Manifest{}, err
	}
	return m, nil
}

func Marshal(m Manifest) ([]byte, error) {
	var b strings.Builder
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(m); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func Write(v vault.Vault) error {
	_, err := WriteWithWarnings(v)
	return err
}

func WriteWithWarnings(v vault.Vault) ([]Warning, error) {
	m, warnings, err := CompileWithWarnings(v)
	if err != nil {
		return nil, err
	}
	data, err := Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.MkdirAll(v.MarkerDir, 0o755); err != nil {
		return nil, fmt.Errorf("create marker directory: %w", err)
	}
	if err := os.WriteFile(v.ManifestPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}
	return warnings, nil
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

type WikiLinkResolver struct {
	resolver *linkResolver
}

func NewWikiLinkResolver(entries []Entry) *WikiLinkResolver {
	return &WikiLinkResolver{resolver: newLinkResolver(entries)}
}

func (r *WikiLinkResolver) Resolve(currentKey, rawTarget string) (string, bool) {
	if r == nil || r.resolver == nil {
		return rawTarget, false
	}
	return r.resolver.resolve(currentKey, rawTarget)
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
