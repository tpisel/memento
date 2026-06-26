package orient

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tpisel/memento/internal/brief"
	"github.com/tpisel/memento/internal/convention"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

//go:embed baseline.md
var baselineFS embed.FS

const overlaySeparator = "\n---\n\n"
const triggeredPreconditionsMarker = "<!-- memento:triggered-preconditions -->"
const briefDisclosureMarker = "<!-- memento:brief-disclosure -->"

// Baseline returns the binary-shipped orientation baseline verbatim.
func Baseline() []byte {
	data, err := baselineFS.ReadFile("baseline.md")
	if err != nil {
		panic(fmt.Sprintf("read embedded orient baseline: %v", err))
	}
	return data
}

// Render composes the baseline with the When To Read Conventions block and
// manifest-selected orient overlay docs. It returns one warning string per
// invalid convention file; warnings never suppress valid conventions or fail
// the render.
func Render(v vault.Vault, m manifest.Manifest) (out []byte, warnings []string, err error) {
	out, err = baselineForVault(v, m)
	if err != nil {
		return nil, nil, err
	}

	conventions, warnings, err := convention.List(v)
	if err != nil {
		return nil, nil, err
	}
	if section := conventionsSection(conventions); section != "" {
		out = bytes.TrimRight(out, "\n")
		out = append(out, "\n\n"...)
		out = append(out, section...)
		out = append(out, '\n')
	}

	entries := orientEntries(m)
	if len(entries) == 0 {
		return out, warnings, nil
	}

	out = bytes.TrimRight(out, "\n")
	for _, entry := range entries {
		path, err := entryPath(v, entry.Key)
		if err != nil {
			return nil, nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("read orient overlay %s: %w", entry.Key, err)
		}
		out = append(out, overlaySeparator...)
		out = append(out, data...)
		out = bytes.TrimRight(out, "\n")
	}
	out = append(out, '\n')
	return out, warnings, nil
}

// conventionsSection renders the When To Read Conventions block, or "" when
// there are no valid conventions. Conventions arrive sorted by name, so the
// output is deterministic.
func conventionsSection(conventions []convention.Convention) string {
	if len(conventions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## When To Read Conventions\n\n")
	for _, c := range conventions {
		fmt.Fprintf(&b, "- %s: `memento convention %s`\n", c.WhenToRead, c.Name)
	}
	return strings.TrimRight(b.String(), "\n")
}

func baselineForVault(v vault.Vault, m manifest.Manifest) ([]byte, error) {
	out := append([]byte(nil), Baseline()...)
	hasWritingGuide, err := hasWritingGuide(v)
	if err != nil {
		return nil, err
	}

	replacement := "None yet."
	if hasWritingGuide {
		replacement = "- `memento write`: before authoring, run `memento read _memento/writing.md`."
	}

	replacements := map[string]string{
		triggeredPreconditionsMarker: replacement,
		briefDisclosureMarker:        briefDisclosure(m),
	}
	for marker, replacement := range replacements {
		markerBytes := []byte(marker)
		if bytes.Count(out, markerBytes) != 1 {
			return nil, fmt.Errorf("orient baseline must contain exactly one %s marker", markerName(marker))
		}
		out = bytes.Replace(out, markerBytes, []byte(replacement), 1)
	}
	return out, nil
}

func briefDisclosure(m manifest.Manifest) string {
	count := len(m.Entries)
	if count == 0 {
		return "`memento brief` will report no notes yet."
	}

	lines := bytes.Count(brief.Render(m), []byte("\n"))
	return fmt.Sprintf(
		"`memento brief` will print summaries of %d %s (~%d lines); it is dense and pull-only.",
		count,
		plural(count, "note", "notes"),
		lines,
	)
}

func markerName(marker string) string {
	switch marker {
	case triggeredPreconditionsMarker:
		return "triggered preconditions"
	case briefDisclosureMarker:
		return "brief disclosure"
	default:
		return marker
	}
}

func plural(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func hasWritingGuide(v vault.Vault) (bool, error) {
	path := filepath.Join(v.Root, vault.ToolDirName, "writing.md")
	info, err := os.Stat(path)
	if err == nil {
		return !info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat writing guide: %w", err)
}

func orientEntries(m manifest.Manifest) []manifest.Entry {
	entries := make([]manifest.Entry, 0)
	for _, entry := range m.Entries {
		if entry.Orient {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	return entries
}

func entryPath(v vault.Vault, key string) (string, error) {
	if key == "" || strings.Contains(key, "\\") || filepath.IsAbs(key) || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("manifest entry has invalid key: %s", key)
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("manifest entry has invalid key: %s", key)
		}
	}
	return filepath.Join(v.Root, filepath.FromSlash(key)), nil
}
