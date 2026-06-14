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
	ErrUnsupportedWriteOperation = errors.New("unsupported write operation")
	ErrReadOnly                  = errors.New("mode rejects write")
)

type WriteOperation string

const (
	OperationAppend         WriteOperation = "append"
	OperationOverwrite      WriteOperation = "overwrite"
	OperationSectionReplace WriteOperation = "section-replace"
	OperationKeyedUpsert    WriteOperation = "keyed-upsert"
)

type WriteOptions struct {
	Operation WriteOperation
}

func Write(v vault.Vault, key string, content []byte, opts WriteOptions) error {
	if opts.Operation == "" {
		opts.Operation = OperationAppend
	}
	if opts.Operation != OperationAppend && opts.Operation != OperationOverwrite {
		return fmt.Errorf("%w: %s", ErrUnsupportedWriteOperation, opts.Operation)
	}

	key, err := normalizeWritableKey(v, key)
	if err != nil {
		return err
	}

	path, err := writablePath(v, key)
	if err != nil {
		return err
	}

	if err := validateWriteMode(v, key, path, opts.Operation); err != nil {
		return err
	}

	if opts.Operation == OperationOverwrite {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return fmt.Errorf("overwrite %s: %w", key, err)
		}
		return nil
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open %s for append: %w", key, err)
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("write %s: %w", key, err)
	}
	return nil
}

func normalizeWritableKey(v vault.Vault, key string) (string, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return "", err
	}
	if filepath.Ext(key) != ".md" {
		return "", fmt.Errorf("%w: write keys must name markdown files: %s", ErrInvalidKey, key)
	}
	if key == vault.IgnoreFileName || key == vault.WritingGuideFileName {
		return "", fmt.Errorf("%w: operational path is not writable through v0 write: %s", ErrInvalidKey, key)
	}

	parts := strings.Split(key, "/")
	if parts[0] == vault.MarkerDirName {
		return "", fmt.Errorf("%w: operational path is not writable through v0 write: %s", ErrInvalidKey, key)
	}

	for i := 0; i < len(parts)-1; i++ {
		dir := strings.Join(parts[:i+1], "/")
		ignored, err := vault.IsIgnored(v, dir, true)
		if err != nil {
			return "", err
		}
		if ignored {
			return "", fmt.Errorf("%w: ignored path is not writable: %s", ErrInvalidKey, key)
		}
	}

	ignored, err := vault.IsIgnored(v, key, false)
	if err != nil {
		return "", err
	}
	if ignored {
		return "", fmt.Errorf("%w: ignored path is not writable: %s", ErrInvalidKey, key)
	}
	return key, nil
}

func validateWriteMode(v vault.Vault, key, path string, op WriteOperation) error {
	source, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s metadata: %w", key, err)
	}

	meta, err := markdown.ExtractMetadata(key, source)
	if err != nil {
		return fmt.Errorf("extract metadata from %s: %w", key, err)
	}
	ratified, err := isRatified(v, key)
	if err != nil {
		return err
	}
	if !ratified {
		return nil
	}

	if meta.Mode == markdown.ModeReadOnly {
		return fmt.Errorf("%w: %s is %s", ErrReadOnly, key, meta.Mode)
	}
	if meta.Mode == markdown.ModeAppendOnly && op == OperationOverwrite {
		return fmt.Errorf("%w: %s", ErrReadOnly, key)
	}
	return nil
}

func writablePath(v vault.Vault, key string) (string, error) {
	root, err := filepath.EvalSymlinks(v.Root)
	if err != nil {
		return "", fmt.Errorf("resolve vault root: %w", err)
	}
	root = filepath.Clean(root)

	path := filepath.Join(v.Root, filepath.FromSlash(key))
	if info, err := os.Lstat(path); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("%w: key names a directory: %s", ErrInvalidKey, key)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: symlink targets are not writable: %s", ErrInvalidKey, key)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat %s: %w", key, err)
	}

	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create parent directory for %s: %w", key, err)
	}

	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("resolve parent directory for %s: %w", key, err)
	}
	realParent = filepath.Clean(realParent)

	rel, err := filepath.Rel(root, realParent)
	if err != nil {
		return "", fmt.Errorf("resolve parent directory for %s: %w", key, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: path resolves outside vault: %s", ErrInvalidKey, key)
	}
	return path, nil
}
