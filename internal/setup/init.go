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
	ConfigFileName          = "config.toml"
	agentsInstructionsFile  = "AGENTS.md"
	claudeInstructionsFile  = "CLAUDE.md"
	bootloaderStartSentinel = "<!-- memento:start -->"
	bootloaderEndSentinel   = "<!-- memento:end -->"
	hookStartSentinel       = "# memento:start"
	hookEndSentinel         = "# memento:end"
	gitignoreStartSentinel  = "# memento:gitignore:start"
	gitignoreEndSentinel    = "# memento:gitignore:end"
)

const defaultConfig = `# memento vault configuration
manifest_path = ".memento/manifest.json"
`

const defaultIgnore = `# memento operational files
.memento/
.mementoignore

# memento generated artifacts
_memento/brief.md

# memento human onboarding artifacts
_memento/Using Memento.md

# macOS Finder metadata
.DS_Store
`

const defaultExampleNote = `---
title: Example memory note
summary: A short example showing the frontmatter memento indexes.
tags: [memento, example]
mode: append-only
---

# Example memory note

Use notes like this for durable project knowledge: decisions, constraints, and discoveries that should survive a task.
`

var defaultUsingMementoGuide = strings.Join([]string{
	"# Using Memento",
	"",
	"Welcome. This note is here because this folder is a little different from the rest of your vault.",
	"",
	"`_memento/` is the human-readable tool namespace for this vault. It is where memento puts notes that are useful to people and agents, but are about the tool rather than your project knowledge itself.",
	"",
	"Memento also has a hidden machine namespace, `.memento/`. That folder holds structured files such as config and the manifest. You normally do not need to open it in Obsidian.",
	"",
	"`brief.md` is auto-regenerated from `.memento/manifest.json`. It is the short agent-facing view of your memory vault: titles, summaries, tags, headings, and modes. Because it is regenerated, edits to `brief.md` will be replaced the next time memento compiles the vault.",
	"",
	"Future tool-read files such as `writing.md`, `review.md`, and `audit.md` will arrive with their corresponding verbs. Those files will let you describe local conventions in normal markdown when the tool grows those workflows.",
	"",
	"This guide is only a gentle starter. You can edit it, move ideas from it into your own notes, or remove it once it stops being useful.",
	"",
	"If you don't want this file, deleting it is fine - memento does not depend on it.",
	"",
}, "\n")

type InitOptions struct {
	AgentInstructionsPath string
}

// Init creates or adopts a memory vault under repoRoot.
func Init(repoRoot, dir string) (vault.Vault, error) {
	return InitWithOptions(repoRoot, dir, InitOptions{})
}

// InitWithOptions creates or adopts a memory vault under repoRoot.
func InitWithOptions(repoRoot, dir string, opts InitOptions) (vault.Vault, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return vault.Vault{}, fmt.Errorf("resolve repository root %q: %w", repoRoot, err)
	}
	root = filepath.Clean(root)

	vaultRoot, err := resolveInitRoot(root, dir)
	if err != nil {
		return vault.Vault{}, err
	}

	greenfield, err := isGreenfieldVault(vaultRoot)
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
	if greenfield {
		if err := ensureFile(filepath.Join(vaultRoot, "example.md"), []byte(defaultExampleNote), 0o644); err != nil {
			return vault.Vault{}, fmt.Errorf("create example note: %w", err)
		}
	}
	if err := ensureUsingMementoGuide(v); err != nil {
		return vault.Vault{}, err
	}
	if err := ensureMementoIgnore(v); err != nil {
		return vault.Vault{}, err
	}
	if err := ensureManifest(v); err != nil {
		return vault.Vault{}, err
	}
	if err := ensureBootloader(root, v, opts); err != nil {
		return vault.Vault{}, err
	}
	if err := ensureGitignore(root, v); err != nil {
		return vault.Vault{}, err
	}
	if err := ensurePreCommitHook(root, v); err != nil {
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

func isGreenfieldVault(vaultRoot string) (bool, error) {
	entries, err := os.ReadDir(vaultRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read memory vault directory: %w", err)
	}
	return len(entries) == 0, nil
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

func ensureMementoIgnore(v vault.Vault) error {
	path := filepath.Join(v.Root, vault.IgnoreFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := writeNewFile(path, []byte(defaultIgnore), 0o644); err != nil {
				return fmt.Errorf("create %s: %w", vault.IgnoreFileName, err)
			}
			return nil
		}
		return fmt.Errorf("read %s: %w", vault.IgnoreFileName, err)
	}

	requiredEntries := []string{
		vault.ToolDirName + "/" + vault.BriefFileName,
		vault.ToolDirName + "/Using Memento.md",
	}
	var missingEntries []string
	for _, entry := range requiredEntries {
		if !hasLine(string(data), entry) {
			missingEntries = append(missingEntries, entry)
		}
	}
	if len(missingEntries) == 0 {
		return nil
	}

	entryLines := append([]string{"# memento ignored artifacts"}, missingEntries...)
	updated := appendMementoIgnoreEntry(string(data), strings.Join(entryLines, "\n"))
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", vault.IgnoreFileName, err)
	}
	return nil
}

func ensureUsingMementoGuide(v vault.Vault) error {
	path := filepath.Join(v.Root, vault.ToolDirName, "Using Memento.md")
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("%s already exists as a directory", path)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat _memento Using Memento guide: %w", err)
	}

	if err := writeNewFile(path, []byte(defaultUsingMementoGuide), 0o644); err != nil {
		return fmt.Errorf("create _memento Using Memento guide: %w", err)
	}
	return nil
}

func ensureGitignore(repoRoot string, v vault.Vault) error {
	path := filepath.Join(repoRoot, ".gitignore")
	block := gitignoreBlock(repoRoot, v)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeNewFile(path, []byte(block+"\n"), 0o644)
		}
		return fmt.Errorf("read .gitignore: %w", err)
	}

	updated, err := insertOrReplaceGitignoreBlock(string(data), block)
	if err != nil {
		return err
	}
	if updated == string(data) {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	return nil
}

func gitignoreBlock(repoRoot string, v vault.Vault) string {
	prefix := gitignoreVaultPrefix(repoRoot, v.Root)

	return strings.Join([]string{
		gitignoreStartSentinel,
		"# Obsidian per-machine UI state",
		prefix + ".obsidian/workspace*",
		prefix + ".obsidian/cache",
		"# Memento generated artifacts",
		prefix + vault.ToolDirName + "/" + vault.BriefFileName,
		gitignoreEndSentinel,
	}, "\n")
}

func gitignoreVaultPrefix(repoRoot, vaultRoot string) string {
	rel, err := filepath.Rel(repoRoot, vaultRoot)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	if rel == "." {
		return ""
	}
	return filepath.ToSlash(rel) + "/"
}

func ensurePreCommitHook(repoRoot string, v vault.Vault) error {
	path := filepath.Join(repoRoot, ".git", "hooks", "pre-commit")
	block := preCommitHookBlock(repoRoot, v)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeNewFile(path, []byte("#!/bin/sh\nset -eu\n\n"+block+"\n"), 0o755)
		}
		return fmt.Errorf("read pre-commit hook: %w", err)
	}

	updated, err := insertOrReplaceHookBlock(string(data), block)
	if err != nil {
		return err
	}
	if updated != string(data) {
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return fmt.Errorf("write pre-commit hook: %w", err)
		}
	}
	if err := ensureExecutable(path); err != nil {
		return fmt.Errorf("make pre-commit hook executable: %w", err)
	}
	return nil
}

func preCommitHookBlock(repoRoot string, v vault.Vault) string {
	memoryPath := displayPath(repoRoot, v.Root)
	manifestPath := displayPath(repoRoot, v.ManifestPath)

	return strings.Join([]string{
		hookStartSentinel,
		"memento compile --dir " + shellQuote(memoryPath),
		"git add -- " + shellQuote(manifestPath),
		hookEndSentinel,
	}, "\n")
}

func ensureExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if mode&0o100 != 0 {
		return nil
	}
	return os.Chmod(path, mode|0o100)
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

func ensureBootloader(repoRoot string, v vault.Vault, opts InitOptions) error {
	paths, err := agentInstructionsPaths(repoRoot, opts.AgentInstructionsPath)
	if err != nil {
		return err
	}

	block := bootloaderBlock(repoRoot, v)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if err := writeNewFile(path, []byte(block+"\n"), 0o644); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("read agent instructions %s: %w", path, err)
		}

		updated, err := insertOrReplaceBootloader(string(data), block)
		if err != nil {
			return err
		}
		if updated == string(data) {
			continue
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return fmt.Errorf("write agent instructions %s: %w", path, err)
		}
	}
	return nil
}

func agentInstructionsPaths(repoRoot, configured string) ([]string, error) {
	if configured != "" {
		path, err := agentInstructionsPath(repoRoot, configured)
		if err != nil {
			return nil, err
		}
		return []string{path}, nil
	}

	var paths []string
	for _, relPath := range []string{agentsInstructionsFile, claudeInstructionsFile} {
		path := filepath.Join(repoRoot, relPath)
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat agent instructions %s: %w", path, err)
		}
	}
	if len(paths) == 0 {
		paths = append(paths, filepath.Join(repoRoot, agentsInstructionsFile))
	}
	return paths, nil
}

func agentInstructionsPath(repoRoot, configured string) (string, error) {
	if filepath.IsAbs(configured) {
		return filepath.Clean(configured), nil
	}
	if filepath.Clean(configured) == "." {
		return "", fmt.Errorf("agent instructions path must name a file")
	}
	return filepath.Join(repoRoot, configured), nil
}

func bootloaderBlock(repoRoot string, v vault.Vault) string {
	memoryPath := displayPath(repoRoot, v.Root)

	return strings.Join([]string{
		bootloaderStartSentinel,
		fmt.Sprintf("Durable project knowledge lives in `%s`.", memoryPath),
		"Run `memento orient` to load the tool's operating instructions, then `memento brief` to scan entries by title, summary, tags, and headings.",
		"Read entries by key or `@N` index with `memento read <key|@N>`.",
		"`memento read` writes `binding: ratified|unratified` to stderr before stdout content.",
		bootloaderEndSentinel,
	}, "\n")
}

func displayPath(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Clean(target))
}

func insertOrReplaceBootloader(contents, block string) (string, error) {
	return insertOrReplaceSentinelBlock(contents, block, bootloaderStartSentinel, bootloaderEndSentinel, "agent instructions", "bootloader")
}

func insertOrReplaceHookBlock(contents, block string) (string, error) {
	return insertOrReplaceSentinelBlock(contents, block, hookStartSentinel, hookEndSentinel, "pre-commit hook", "memento")
}

func insertOrReplaceGitignoreBlock(contents, block string) (string, error) {
	return insertOrReplaceSentinelBlock(contents, block, gitignoreStartSentinel, gitignoreEndSentinel, ".gitignore", "memento gitignore")
}

func insertOrReplaceSentinelBlock(contents, block, startSentinel, endSentinel, target, blockName string) (string, error) {
	start := strings.Index(contents, startSentinel)
	end := strings.Index(contents, endSentinel)
	startCount := strings.Count(contents, startSentinel)
	endCount := strings.Count(contents, endSentinel)

	switch {
	case start == -1 && end == -1:
		return appendSentinelBlock(contents, block), nil
	case start == -1 || end == -1 || end < start:
		return "", fmt.Errorf("%s contains an incomplete %s block", target, blockName)
	case startCount != 1 || endCount != 1:
		return "", fmt.Errorf("%s contains multiple %s blocks", target, blockName)
	}

	end += len(endSentinel)
	return contents[:start] + block + contents[end:], nil
}

func appendSentinelBlock(contents, block string) string {
	if contents == "" {
		return block + "\n"
	}
	separator := "\n\n"
	if strings.HasSuffix(contents, "\n\n") {
		separator = ""
	} else if strings.HasSuffix(contents, "\n") {
		separator = "\n"
	}
	return contents + separator + block + "\n"
}

func appendMementoIgnoreEntry(contents, entryBlock string) string {
	if contents == "" {
		return entryBlock + "\n"
	}
	separator := "\n\n"
	if strings.HasSuffix(contents, "\n\n") {
		separator = ""
	} else if strings.HasSuffix(contents, "\n") {
		separator = "\n"
	}
	return contents + separator + entryBlock + "\n"
}

func hasLine(contents, want string) bool {
	for _, line := range strings.Split(contents, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func writeNewFile(path string, contents []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create agent instructions directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("create agent instructions: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(contents); err != nil {
		return fmt.Errorf("write agent instructions: %w", err)
	}
	return nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
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
