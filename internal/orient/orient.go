package orient

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

//go:embed baseline.md
var baselineFS embed.FS

const overlaySeparator = "\n---\n\n"
const triggeredPreconditionsMarker = "<!-- memento:triggered-preconditions -->"

// Baseline returns the binary-shipped orientation baseline verbatim.
func Baseline() []byte {
	data, err := baselineFS.ReadFile("baseline.md")
	if err != nil {
		panic(fmt.Sprintf("read embedded orient baseline: %v", err))
	}
	return data
}

// Render composes the baseline with manifest-selected orient overlay docs.
func Render(v vault.Vault, m manifest.Manifest) ([]byte, error) {
	entries := orientEntries(m)
	out, err := baselineForVault(v)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return out, nil
	}

	out = bytes.TrimRight(out, "\n")
	for _, entry := range entries {
		path, err := entryPath(v, entry.Key)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read orient overlay %s: %w", entry.Key, err)
		}
		out = append(out, overlaySeparator...)
		out = append(out, data...)
		out = bytes.TrimRight(out, "\n")
	}
	out = append(out, '\n')
	return out, nil
}

func baselineForVault(v vault.Vault) ([]byte, error) {
	out := append([]byte(nil), Baseline()...)
	hasWritingGuide, err := hasWritingGuide(v)
	if err != nil {
		return nil, err
	}

	replacement := "None yet."
	if hasWritingGuide {
		replacement = "- `memento write`: before authoring, run `memento read _memento/writing.md`."
	}

	marker := []byte(triggeredPreconditionsMarker)
	if bytes.Count(out, marker) != 1 {
		return nil, fmt.Errorf("orient baseline must contain exactly one triggered preconditions marker")
	}
	return bytes.Replace(out, marker, []byte(replacement), 1), nil
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
