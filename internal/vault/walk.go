package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tpisel/memento/internal/ignore"
)

const IgnoreFileName = ".mementoignore"

// WalkMarkdown visits markdown content files in deterministic vault-relative order.
func WalkMarkdown(vault Vault, visit func(relPath, absPath string) error) error {
	patterns, err := loadIgnorePatterns(vault.Root)
	if err != nil {
		return err
	}

	return filepath.WalkDir(vault.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := vaultRelative(vault.Root, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		if d.IsDir() {
			switch d.Name() {
			case MarkerDirName:
				return filepath.SkipDir
			default:
				if ignore.Matches(patterns, relPath, true) {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if relPath == IgnoreFileName {
			return nil
		}
		if ignore.Matches(patterns, relPath, false) {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		return visit(relPath, path)
	})
}

func loadIgnorePatterns(root string) ([]ignore.Pattern, error) {
	file, err := os.Open(filepath.Join(root, IgnoreFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", IgnoreFileName, err)
	}
	defer file.Close()

	patterns, err := ignore.Parse(file)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", IgnoreFileName, err)
	}
	return patterns, nil
}

func vaultRelative(root, path string) (string, error) {
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("resolve vault-relative path for %s: %w", path, err)
	}
	return filepath.ToSlash(relPath), nil
}
