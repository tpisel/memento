package note

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tpisel/memento/internal/markdown"
	"github.com/tpisel/memento/internal/vault"
)

var (
	ErrInvalidKey      = errors.New("invalid key")
	ErrNotFound        = errors.New("not found")
	ErrSectionNotFound = errors.New("section not found")
)

var errFound = errors.New("found")

func Read(v vault.Vault, key string) ([]byte, error) {
	key, section, err := parseReadTarget(key)
	if err != nil {
		return nil, err
	}

	var data []byte
	err = vault.WalkMarkdown(v, func(relPath, absPath string) error {
		if relPath != key {
			return nil
		}

		data, err = os.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", relPath, err)
		}
		return errFound
	})
	if errors.Is(err, errFound) {
		if section != "" {
			sectionData, err := markdown.ExtractSection(data, section)
			if errors.Is(err, markdown.ErrSectionNotFound) {
				return nil, fmt.Errorf("%w: %s#%s", ErrSectionNotFound, key, section)
			}
			if err != nil {
				return nil, fmt.Errorf("extract section from %s: %w", key, err)
			}
			return sectionData, nil
		}
		return data, nil
	}
	if err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
}

func parseReadTarget(target string) (key string, section string, err error) {
	key, section, hasSection := strings.Cut(target, "#")
	key, err = normalizeKey(key)
	if err != nil {
		return "", "", err
	}
	if hasSection && strings.TrimSpace(section) == "" {
		return "", "", fmt.Errorf("%w: empty section in %s", ErrInvalidKey, target)
	}
	return key, section, nil
}

func normalizeKey(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("%w: empty key", ErrInvalidKey)
	}
	if strings.Contains(key, "\\") {
		return "", fmt.Errorf("%w: backslash path separators are not supported: %s", ErrInvalidKey, key)
	}
	if filepath.IsAbs(key) || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("%w: absolute paths are not vault-relative: %s", ErrInvalidKey, key)
	}

	parts := strings.Split(key, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("%w: %s", ErrInvalidKey, key)
		}
	}
	return key, nil
}
