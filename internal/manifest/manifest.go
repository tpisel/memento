package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
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
	Out []string `json:"out"`
	In  []string `json:"in"`
}

func Compile(v vault.Vault) (Manifest, error) {
	entries := []Entry{}
	tagCounts := map[string]int{}

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

		entries = append(entries, Entry{
			Key:          relPath,
			Title:        meta.Title,
			Summary:      meta.Summary,
			Tags:         tags,
			Headings:     manifestHeadings(meta.Headings),
			Mode:         meta.Mode,
			Updated:      formatUpdated(meta.Updated),
			SummaryStale: meta.SummaryStale,
			Links:        Links{Out: []string{}, In: []string{}},
		})
		return nil
	})
	if err != nil {
		return Manifest{}, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})

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
