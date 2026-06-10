package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

const (
	ConfigFileName = "config.toml"
)

const defaultConfig = `# memento vault configuration
manifest_path = ".memento/manifest.json"
`

const defaultIgnore = `# memento operational files
.memento/
.mementoignore

# Obsidian per-machine state
.obsidian/workspace*
.obsidian/cache/

# macOS Finder metadata
.DS_Store
`

// Init creates or adopts a memory vault under repoRoot.
func Init(repoRoot, dir string) (vault.Vault, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return vault.Vault{}, fmt.Errorf("resolve repository root %q: %w", repoRoot, err)
	}
	root = filepath.Clean(root)

	vaultRoot, err := resolveInitRoot(root, dir)
	if err != nil {
		return vault.Vault{}, err
	}

	marker := filepath.Join(vaultRoot, vault.MarkerDirName)
	if err := os.MkdirAll(marker, 0o755); err != nil {
		return vault.Vault{}, fmt.Errorf("create memento marker directory: %w", err)
	}

	v := vault.Vault{
		Root:         vaultRoot,
		MarkerDir:    marker,
		ManifestPath: filepath.Join(marker, vault.ManifestFileName),
	}

	if err := ensureFile(filepath.Join(marker, ConfigFileName), []byte(defaultConfig), 0o644); err != nil {
		return vault.Vault{}, fmt.Errorf("create config: %w", err)
	}
	if err := ensureFile(filepath.Join(vaultRoot, vault.IgnoreFileName), []byte(defaultIgnore), 0o644); err != nil {
		return vault.Vault{}, fmt.Errorf("create %s: %w", vault.IgnoreFileName, err)
	}
	if err := ensureManifest(v); err != nil {
		return vault.Vault{}, err
	}

	return v, nil
}

func resolveInitRoot(repoRoot, dir string) (string, error) {
	if dir != "" {
		if filepath.IsAbs(dir) {
			return filepath.Clean(dir), nil
		}
		return filepath.Clean(filepath.Join(repoRoot, dir)), nil
	}

	discovered, err := vault.Discover(repoRoot)
	if err == nil {
		return discovered.Root, nil
	}
	if !errors.Is(err, vault.ErrVaultNotFound) {
		return "", err
	}

	return filepath.Join(repoRoot, defaultVaultDirName(repoRoot)), nil
}

func ensureFile(path string, contents []byte, perm os.FileMode) error {
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("%s already exists as a directory", path)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	if _, err := file.Write(contents); err != nil {
		return err
	}
	return nil
}

func ensureManifest(v vault.Vault) error {
	if info, err := os.Stat(v.ManifestPath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("manifest path %s already exists as a directory", v.ManifestPath)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat manifest: %w", err)
	}

	if err := manifest.Write(v); err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}
	return nil
}

func defaultVaultDirName(repoRoot string) string {
	project := projectNameFromGitConfig(repoRoot)
	if project == "" {
		project = filepath.Base(repoRoot)
	}
	project = sanitizeProjectName(project)
	if project == "" {
		return "memory"
	}
	return project + "-memory"
}

func projectNameFromGitConfig(repoRoot string) string {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".git", "config"))
	if err != nil {
		return ""
	}

	var firstRemote string
	inOrigin := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inOrigin = trimmed == `[remote "origin"]`
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(key) != "url" {
			continue
		}
		name := projectNameFromRemoteURL(strings.TrimSpace(value))
		if name == "" {
			continue
		}
		if inOrigin {
			return name
		}
		if firstRemote == "" {
			firstRemote = name
		}
	}
	return firstRemote
}

func projectNameFromRemoteURL(raw string) string {
	text := strings.TrimSuffix(strings.TrimSpace(raw), "/")
	if text == "" {
		return ""
	}
	if idx := strings.LastIndexAny(text, "/:"); idx >= 0 {
		text = text[idx+1:]
	}
	text = strings.TrimSuffix(text, ".git")
	return text
}

func sanitizeProjectName(name string) string {
	var b strings.Builder
	previousDash := false
	for _, r := range strings.TrimSpace(name) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '.', r == '_':
			b.WriteRune(r)
			previousDash = false
		case r == '-':
			if !previousDash {
				b.WriteRune(r)
				previousDash = true
			}
		default:
			if !previousDash {
				b.WriteRune('-')
				previousDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
