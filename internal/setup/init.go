package setup

import (
	"encoding/json"
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
	claudeSettingsFile      = ".claude/settings.json"
	claudeOrientHookScript  = ".claude/memento-orient-session-start.sh"
	claudeWriteSkillFile    = ".claude/skills/memento-write/SKILL.md"
	bootloaderStartSentinel = "<!-- memento:start -->"
	bootloaderEndSentinel   = "<!-- memento:end -->"
	hookStartSentinel       = "# memento:start"
	hookEndSentinel         = "# memento:end"
	gitignoreStartSentinel  = "# memento:gitignore:start"
	gitignoreEndSentinel    = "# memento:gitignore:end"
)

const defaultConfig = `# memento vault configuration
`

const defaultIgnore = `# memento operational files
.memento/
.mementoignore

# memento operational namespace (conventions, skills, generated brief, onboarding)
_memento/

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
	"**What is this?** Memento is a markdown-based, in-repo memory substrate for AI coding agents. It keeps durable project knowledge — decisions, constraints, discoveries — in this vault as plain markdown, and gives agents a small CLI (`memento orient`, `brief`, `read`, `write`) to load only what is relevant to the task at hand. Obsidian is the human surface; the CLI is the agent's.",
	"",
	"**No `memento` command?** If you cloned this repo and the agent reports memento is not installed, install it from https://github.com/tpisel/memento — `brew install tpisel/tap/memento`, or `go install github.com/tpisel/memento/cmd/memento@latest` (Go 1.22+). Verify with `memento version`. The notes here stay readable as plain markdown without it, but `brief`/`orient`/`read` need the binary.",
	"",
	"Welcome. This note is here because this folder is a little different from the rest of your vault.",
	"",
	"`_memento/` is the human-readable tool namespace for this vault. It is where memento puts notes that are useful to people and agents, but are about the tool rather than your project knowledge itself.",
	"",
	"Memento also has a hidden machine namespace, `.memento/`. That folder holds structured files such as config and the manifest. You normally do not need to open it in Obsidian.",
	"",
	"`brief.md` is auto-regenerated from `.memento/manifest.json`. It is the short agent-facing view of your memory vault: titles, summaries, tags, headings, and modes. Because it is regenerated, edits to `brief.md` will be replaced the next time memento compiles the vault.",
	"",
	"`conventions/` holds operational guides written in normal markdown, such as `writing.md`. Each declares a `when_to_read:` line naming when it applies. `memento orient` surfaces those prompts, and an agent loads the body with `memento convention <name>` (for example `memento convention writing`) when the moment arrives.",
	"",
	"This guide is only a gentle starter. You can edit it, move ideas from it into your own notes, or remove it once it stops being useful.",
	"",
	"If you don't want this file, deleting it is fine - memento does not depend on it.",
	"",
}, "\n")

var defaultWriteSkill = strings.Join([]string{
	"---",
	"name: memento-write",
	"description: Use before creating or updating any note in the memento vault. Loads the project writing guide and the safe-write rules so durable knowledge lands correctly and read-only notes are not corrupted.",
	"---",
	"",
	"# Writing to the memento vault",
	"",
	"Before authoring a vault write:",
	"",
	"1. **Read the writing guide.** Run `memento convention writing` to load when/what to write, what to keep in the task store instead, and the expected note shape. Do this before composing, not after.",
	"2. **Write through memento, not native file edits.** Use `memento write` so the mode check (`append-only` / `living` / `read-only`) is applied. A native file edit of a vault note bypasses that check and can silently overwrite a `read-only` note - the read-only guarantee lives in the write verb, not in the file.",
	"3. **Keep it scannable.** Durable notes should read cleanly from `memento brief`; lead the summary with the load-bearing fact or decision.",
	"",
	"This skill is a delivery surface for the `writing` convention (`_memento/conventions/writing.md`) - that file is the source of truth. If the two ever disagree, the convention wins.",
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
	if err := ensureConventionTemplates(v); err != nil {
		return vault.Vault{}, err
	}
	if err := ensureMementoIgnore(v); err != nil {
		return vault.Vault{}, err
	}
	if err := ensureWriteSkillSource(v); err != nil {
		return vault.Vault{}, err
	}
	if err := ensureManifest(v); err != nil {
		return vault.Vault{}, err
	}
	if err := ensureBootloader(root, v, opts); err != nil {
		return vault.Vault{}, err
	}
	if err := ensureGitignore(root); err != nil {
		return vault.Vault{}, err
	}
	if err := ensurePreCommitHook(root, v); err != nil {
		return vault.Vault{}, err
	}
	if err := ensureClaudeAgentIntegration(root, v); err != nil {
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

	namespaceEntry := vault.ToolDirName + "/"
	if hasLine(string(data), namespaceEntry) {
		return nil
	}

	entryLines := []string{"# memento operational namespace", namespaceEntry}
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

// conventionTemplate is a default convention file installed under
// _memento/conventions/. Templates carry only title and when_to_read
// frontmatter (ADR-0029) and stay project-neutral (ADR-0030).
type conventionTemplate struct {
	stem    string
	content string
}

var defaultConventionTemplates = []conventionTemplate{
	{
		stem: "writing",
		content: strings.Join([]string{
			"---",
			"title: Writing guide",
			"when_to_read: before authoring a memento vault write",
			"---",
			"",
			"# Writing guide",
			"",
			"Write durable project knowledge that should survive a task: decisions, the paths you ruled out and why, and constraints that are not visible in the code itself.",
			"",
			"Do not record transient task progress, guesses, or details the code already makes clear. If a fact only matters to the task in hand, keep it in your task store, not the vault.",
			"",
			"Write through `memento write` so the note's mode check applies; a native file edit can silently overwrite a read-only note.",
			"",
		}, "\n"),
	},
	{
		stem: "summarising",
		content: strings.Join([]string{
			"---",
			"title: Summarising guide",
			"when_to_read: when writing or revising a note summary",
			"---",
			"",
			"# Summarising guide",
			"",
			"A summary is read from `memento brief` to decide whether to open the note. Lead with the load-bearing fact or decision, not a description of the topic.",
			"",
			"Prefer one or two dense sentences that state the conclusion. Do not restate the title, and avoid \"this note covers ...\" framing.",
			"",
		}, "\n"),
	},
	{
		stem: "conventions",
		content: strings.Join([]string{
			"---",
			"title: Conventions guide",
			"when_to_read: before adding or editing a convention file",
			"---",
			"",
			"# Conventions guide",
			"",
			"Conventions are operational guides under `_memento/conventions/`. Each declares `title:` and a non-empty `when_to_read:` in frontmatter; the workflow instructions live in the body.",
			"",
			"- Use a short lowercase filename stem with no spaces, such as `writing.md` or `summarising.md`. Use hyphens only when a single word is unclear.",
			"- Make `when_to_read:` complete the sentence \"Read this convention ...\".",
			"- Keep frontmatter to `title:` and `when_to_read:`; do not add `mode`, `summary`, or `tags`.",
			"- Put workflow instructions in the body, not in frontmatter.",
			"",
			"A convention without `when_to_read:` is invalid and will not be offered. Conventions are operational guidance, not project knowledge, so they stay out of the normal brief corpus.",
			"",
		}, "\n"),
	},
}

func ensureConventionTemplates(v vault.Vault) error {
	dir := filepath.Join(v.Root, vault.ToolDirName, "conventions")
	for _, template := range defaultConventionTemplates {
		path := filepath.Join(dir, template.stem+".md")
		if info, err := os.Stat(path); err == nil {
			if info.IsDir() {
				return fmt.Errorf("%s already exists as a directory", path)
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat _memento convention %s: %w", template.stem, err)
		}
		if err := writeNewFile(path, []byte(template.content), 0o644); err != nil {
			return fmt.Errorf("create _memento convention %s: %w", template.stem, err)
		}
	}
	return nil
}

func ensureGitignore(repoRoot string) error {
	path := filepath.Join(repoRoot, ".gitignore")
	block := gitignoreBlock()

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

func gitignoreBlock() string {
	return strings.Join([]string{
		gitignoreStartSentinel,
		"# Obsidian per-machine UI state",
		"**/.obsidian/workspace*",
		"**/.obsidian/cache",
		"# Memento generated artifacts",
		"**/" + vault.ToolDirName + "/" + vault.BriefFileName,
		gitignoreEndSentinel,
	}, "\n")
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

func ensureClaudeAgentIntegration(repoRoot string, v vault.Vault) error {
	if err := ensureClaudeOrientHookScript(repoRoot); err != nil {
		return err
	}
	if err := ensureClaudeSettings(repoRoot); err != nil {
		return err
	}
	if err := ensureClaudeWriteSkill(repoRoot, v); err != nil {
		return err
	}
	return nil
}

func ensureWriteSkillSource(v vault.Vault) error {
	path := filepath.Join(v.Root, vault.ToolDirName, "skills", "write.md")
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("%s already exists as a directory", path)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat _memento write skill source: %w", err)
	}
	if err := writeNewFile(path, []byte(defaultWriteSkill), 0o644); err != nil {
		return fmt.Errorf("create _memento write skill source: %w", err)
	}
	return nil
}

func ensureClaudeWriteSkill(repoRoot string, v vault.Vault) error {
	sourcePath := filepath.Join(v.Root, vault.ToolDirName, "skills", "write.md")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read _memento write skill source: %w", err)
	}

	path := filepath.Join(repoRoot, filepath.FromSlash(claudeWriteSkillFile))
	if err := writeFileIfChanged(path, source, 0o644); err != nil {
		return fmt.Errorf("install Claude write skill: %w", err)
	}
	return nil
}

func ensureClaudeOrientHookScript(repoRoot string) error {
	path := filepath.Join(repoRoot, filepath.FromSlash(claudeOrientHookScript))
	script := claudeOrientHookScriptContents(repoRoot)
	if err := writeFileIfChanged(path, []byte(script), 0o755); err != nil {
		return fmt.Errorf("install Claude orient hook script: %w", err)
	}
	if err := ensureExecutable(path); err != nil {
		return fmt.Errorf("make Claude orient hook script executable: %w", err)
	}
	return nil
}

func claudeOrientHookScriptContents(repoRoot string) string {
	return strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -u",
		"",
		"repo_root=" + shellQuote(repoRoot),
		"",
		"json_escape() {",
		"  local value=$1",
		"  value=${value//\\\\/\\\\\\\\}",
		"  value=${value//\\\"/\\\\\\\"}",
		"  value=${value//$'\\b'/\\\\b}",
		"  value=${value//$'\\f'/\\\\f}",
		"  value=${value//$'\\n'/\\\\n}",
		"  value=${value//$'\\r'/\\\\r}",
		"  value=${value//$'\\t'/\\\\t}",
		"  printf '%s' \"$value\"",
		"}",
		"",
		"emit_context() {",
		"  local context=$1",
		"  printf '{\"hookSpecificOutput\":{\"hookEventName\":\"SessionStart\",\"additionalContext\":\"%s\"}}\\n' \"$(json_escape \"$context\")\"",
		"}",
		"",
		"if ! command -v memento >/dev/null 2>&1; then",
		"  emit_context 'memento SessionStart hook: memento not on PATH; orient unavailable. Install from https://github.com/tpisel/memento (brew install tpisel/tap/memento, or go install github.com/tpisel/memento/cmd/memento@latest), then memento version to verify.'",
		"  exit 0",
		"fi",
		"",
		"compile_note=''",
		"if ! compile_output=\"$(cd \"$repo_root\" && memento compile 2>&1)\"; then",
		"  compile_note=$'memento compile failed; continuing with memento orient.\\n'",
		"fi",
		"",
		"orient_output=\"$(cd \"$repo_root\" && memento orient 2>&1)\"",
		"orient_status=$?",
		"if [ \"$orient_status\" -ne 0 ]; then",
		"  orient_output=$'memento orient failed.\\n'\"$orient_output\"",
		"fi",
		"",
		"emit_context \"$compile_note$orient_output\"",
		"",
	}, "\n")
}

func ensureClaudeSettings(repoRoot string) error {
	path := filepath.Join(repoRoot, filepath.FromSlash(claudeSettingsFile))
	settings := map[string]any{}

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read Claude settings: %w", err)
		}
	} else if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse Claude settings: %w", err)
		}
	}

	hooks, err := objectSetting(settings, "hooks")
	if err != nil {
		return err
	}
	sessionStart, err := arraySetting(hooks, "SessionStart")
	if err != nil {
		return err
	}

	command := filepath.Join(repoRoot, filepath.FromSlash(claudeOrientHookScript))
	hooks["SessionStart"] = upsertClaudeOrientHook(sessionStart, command)
	settings["hooks"] = hooks

	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("render Claude settings: %w", err)
	}
	updated = append(updated, '\n')
	if string(updated) == string(data) {
		return nil
	}
	if err := writeFileIfChanged(path, updated, 0o644); err != nil {
		return fmt.Errorf("write Claude settings: %w", err)
	}
	return nil
}

func objectSetting(parent map[string]any, key string) (map[string]any, error) {
	if raw, ok := parent[key]; ok {
		object, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Claude settings %s must be an object", key)
		}
		return object, nil
	}
	object := map[string]any{}
	parent[key] = object
	return object, nil
}

func arraySetting(parent map[string]any, key string) ([]any, error) {
	if raw, ok := parent[key]; ok {
		array, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("Claude settings hooks.%s must be an array", key)
		}
		return array, nil
	}
	array := []any{}
	parent[key] = array
	return array, nil
}

func upsertClaudeOrientHook(entries []any, command string) []any {
	managed := claudeOrientHookEntry(command)
	cleaned := make([]any, 0, len(entries)+1)
	for _, entry := range entries {
		object, ok := entry.(map[string]any)
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}
		hooks, ok := object["hooks"].([]any)
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}

		filtered := make([]any, 0, len(hooks))
		for _, hook := range hooks {
			if isMementoOrientHook(hook) {
				continue
			}
			filtered = append(filtered, hook)
		}
		if len(filtered) == len(hooks) {
			cleaned = append(cleaned, entry)
			continue
		}
		if len(filtered) == 0 {
			continue
		}
		object["hooks"] = filtered
		cleaned = append(cleaned, object)
	}
	cleaned = append(cleaned, managed)
	return cleaned
}

func claudeOrientHookEntry(command string) map[string]any {
	return map[string]any{
		"matcher": "startup|resume|compact",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": command,
			},
		},
	}
}

func isMementoOrientHook(hook any) bool {
	object, ok := hook.(map[string]any)
	if !ok {
		return false
	}
	command, ok := object["command"].(string)
	if !ok {
		return false
	}
	base := filepath.Base(command)
	return base == filepath.Base(claudeOrientHookScript) || base == "orient-session-start.sh"
}

func preCommitHookBlock(repoRoot string, v vault.Vault) string {
	manifestPath := displayPath(repoRoot, v.ManifestPath)

	return strings.Join([]string{
		hookStartSentinel,
		"if command -v memento >/dev/null 2>&1; then",
		"memento compile",
		"git add -- " + shellQuote(manifestPath),
		"else",
		"echo 'warn: memento not on PATH; skipping vault compile' >&2",
		"fi",
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
		fmt.Sprintf("Durable project knowledge lives in `%s`: curated design decisions, specs, constraints, and discoveries, not task state.", memoryPath),
		"Before any other memento action, run `memento orient`.",
		"Run `memento brief` when you need the doc landscape; it is pull-only, not a mandatory second step.",
		"Use `memento read <key|@N>#<heading>` or `memento read <key|@N>` instead of grep/cat: it emits link-graph metadata on stderr and supports section extraction.",
		"`@N` indexes come from `brief`; `memento read` writes `binding: ratified|unratified` plus non-empty role-flattened link lines to stderr before stdout content.",
		fmt.Sprintf("Discoveries that outlive a task belong in `%s`, not the task store.", memoryPath),
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

func writeFileIfChanged(path string, contents []byte, perm os.FileMode) error {
	if data, err := os.ReadFile(path); err == nil {
		if string(data) == string(contents) {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, contents, perm); err != nil {
		return err
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
