// Package convention reads operational convention files under
// _memento/conventions/. A convention is a markdown file whose frontmatter
// declares a non-empty when_to_read: string naming the circumstance in which an
// agent should read it (ADR-0029). Conventions are operational guidance, not
// part of the normal brief corpus, so this package reads them directly rather
// than through the manifest.
package convention

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/tpisel/memento/internal/markdown"
	"github.com/tpisel/memento/internal/vault"
)

// DirName is the convention subdirectory under the operational namespace.
const DirName = "conventions"

var (
	// ErrInvalidName reports a name that is not a bare lowercase filename stem.
	ErrInvalidName = errors.New("invalid convention name")
	// ErrNotFound reports that the named convention file does not exist.
	ErrNotFound = errors.New("convention not found")
	// ErrInvalid reports a convention file missing a non-empty when_to_read.
	ErrInvalid = errors.New("invalid convention")
)

// Convention is a parsed convention file.
type Convention struct {
	Name       string
	Title      string
	WhenToRead string
	Body       []byte
}

// RelPath returns the vault-relative path of the named convention, for use in
// error messages and pointers. It does not validate the name.
func RelPath(name string) string {
	return vault.ToolDirName + "/" + DirName + "/" + name + ".md"
}

// Path returns the absolute filesystem path of the named convention.
func Path(v vault.Vault, name string) string {
	return filepath.Join(v.Root, vault.ToolDirName, DirName, name+".md")
}

// ValidateName checks that name is a bare lowercase filename stem: no slash,
// extension, spaces, or path traversal.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidName)
	}
	if name != strings.ToLower(name) {
		return fmt.Errorf("%w: name must be lowercase: %q", ErrInvalidName, name)
	}
	for _, r := range name {
		switch {
		case r == '/' || r == '\\':
			return fmt.Errorf("%w: name must not contain a path separator: %q", ErrInvalidName, name)
		case r == '.':
			return fmt.Errorf("%w: name must be a bare filename stem with no extension or path: %q", ErrInvalidName, name)
		case unicode.IsSpace(r):
			return fmt.Errorf("%w: name must not contain spaces: %q", ErrInvalidName, name)
		}
	}
	return nil
}

// Read validates name, reads _memento/conventions/<name>.md, strips its
// frontmatter, and returns the parsed convention. It returns ErrNotFound when
// the file is absent and ErrInvalid when when_to_read is missing or empty.
func Read(v vault.Vault, name string) (Convention, error) {
	if err := ValidateName(name); err != nil {
		return Convention{}, err
	}

	data, err := os.ReadFile(Path(v, name))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Convention{}, fmt.Errorf("%w: %s", ErrNotFound, RelPath(name))
		}
		return Convention{}, fmt.Errorf("read convention %s: %w", RelPath(name), err)
	}

	front, body, _ := markdown.SplitFrontmatter(data)
	whenToRead := frontmatterScalar(front, "when_to_read")
	if whenToRead == "" {
		return Convention{}, fmt.Errorf("%w: %s is missing a non-empty when_to_read", ErrInvalid, RelPath(name))
	}

	return Convention{
		Name:       name,
		Title:      frontmatterScalar(front, "title"),
		WhenToRead: whenToRead,
		Body:       body,
	}, nil
}

// frontmatterScalar returns the trimmed, unquoted value of a single-line scalar
// field in a raw frontmatter block, or "" when the field is absent or empty.
func frontmatterScalar(front []byte, key string) string {
	for _, line := range strings.Split(string(front), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		return unquoteScalar(strings.TrimSpace(v))
	}
	return ""
}

func unquoteScalar(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
