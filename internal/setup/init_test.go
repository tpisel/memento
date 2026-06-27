package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tpisel/memento/internal/enforce"
)

func TestInitDerivesDefaultVaultDirFromGitRemote(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, ".git/config", `[remote "origin"]
	url = git@github.com:tpisel/remote-name.git
`)

	v, err := Init(repo, "")
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	want := filepath.Join(repo, "remote-name-memory")
	if v.Root != want {
		t.Fatalf("Init().Root = %q, want %q", v.Root, want)
	}
}

func TestInitWritesHeaderOnlyDefaultConfig(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "memory/.memento/config.toml")
	want := "# memento vault configuration\n"
	if got != want {
		t.Fatalf("config.toml = %q, want %q", got, want)
	}
}

func TestInitCreatesAgentInstructionsWhenAbsent(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "sample-app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	if _, err := Init(repo, ""); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "AGENTS.md")
	for _, want := range []string{
		"<!-- memento:start -->",
		"Durable project knowledge lives in `sample-app-memory`: curated design decisions, specs, constraints, and discoveries, not task state.",
		"Before any other memento action, run `memento orient`.",
		"Run `memento brief` when you need the doc landscape; it is pull-only, not a mandatory second step.",
		"Use `memento read <key|@N>#<heading>` or `memento read <key|@N>` instead of grep/cat: it emits link-graph metadata on stderr and supports section extraction.",
		"`@N` indexes come from `brief`; `memento read` writes `binding: ratified|unratified` plus non-empty role-flattened link lines to stderr before stdout content.",
		"Discoveries that outlive a task belong in `sample-app-memory`, not the task store.",
		"<!-- memento:end -->",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("AGENTS.md = %q, want it to contain %q", got, want)
		}
	}
}

func TestInitAppendsBootloaderToExistingAgentInstructions(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "AGENTS.md", "# Agent Instructions\n\nKeep existing rules.\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "AGENTS.md")
	if !strings.HasPrefix(got, "# Agent Instructions\n\nKeep existing rules.\n") {
		t.Fatalf("AGENTS.md = %q, want existing content preserved at start", got)
	}
	if count := strings.Count(got, "<!-- memento:start -->"); count != 1 {
		t.Fatalf("AGENTS.md start sentinel count = %d, want 1; contents = %q", count, got)
	}
	assertPointerBootloader(t, "AGENTS.md", got, "memory")
}

func TestInitAppendsBootloaderToExistingClaudeInstructions(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "CLAUDE.md", "# Claude Instructions\n\nKeep existing rules.\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "CLAUDE.md")
	if !strings.HasPrefix(got, "# Claude Instructions\n\nKeep existing rules.\n") {
		t.Fatalf("CLAUDE.md = %q, want existing content preserved at start", got)
	}
	if count := strings.Count(got, "<!-- memento:start -->"); count != 1 {
		t.Fatalf("CLAUDE.md start sentinel count = %d, want 1; contents = %q", count, got)
	}
	assertPointerBootloader(t, "CLAUDE.md", got, "memory")
	if _, err := os.Stat(filepath.Join(repo, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md stat err = %v, want file not to exist", err)
	}
}

func TestInitInjectsBootloaderIntoAgentsAndClaudeWhenBothExist(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "AGENTS.md", "# Agent Instructions\n\nKeep agent rules.\n")
	writeSetupFile(t, repo, "CLAUDE.md", "# Claude Instructions\n\nKeep Claude rules.\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	for _, relPath := range []string{"AGENTS.md", "CLAUDE.md"} {
		got := readSetupFile(t, repo, relPath)
		if count := strings.Count(got, "<!-- memento:start -->"); count != 1 {
			t.Fatalf("%s start sentinel count = %d, want 1; contents = %q", relPath, count, got)
		}
		assertPointerBootloader(t, relPath, got, "memory")
	}
}

func TestInitReplacesExistingBootloaderBlock(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "AGENTS.md", "# Rules\n\n<!-- memento:start -->\nold block\n<!-- memento:end -->\n\nKeep this too.\n")

	if _, err := Init(repo, "project-memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "AGENTS.md")
	for _, want := range []string{"# Rules\n\n", "\n\nKeep this too.\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("AGENTS.md = %q, want it to contain %q", got, want)
		}
	}
	assertPointerBootloader(t, "AGENTS.md", got, "project-memory")
	if strings.Contains(got, "old block") {
		t.Fatalf("AGENTS.md = %q, want old bootloader removed", got)
	}
	if count := strings.Count(got, "<!-- memento:start -->"); count != 1 {
		t.Fatalf("AGENTS.md start sentinel count = %d, want 1; contents = %q", count, got)
	}
}

func TestInitReplacesExistingBootloaderBlockInEveryPresentInstructionFile(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "AGENTS.md", "# Agent Rules\n\n<!-- memento:start -->\nold agents block\n<!-- memento:end -->\n\nKeep this too.\n")
	writeSetupFile(t, repo, "CLAUDE.md", "# Claude Rules\n\n<!-- memento:start -->\nold claude block\n<!-- memento:end -->\n\nKeep this too.\n")

	if _, err := Init(repo, "project-memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	for _, relPath := range []string{"AGENTS.md", "CLAUDE.md"} {
		got := readSetupFile(t, repo, relPath)
		assertPointerBootloader(t, relPath, got, "project-memory")
		if strings.Contains(got, "old agents block") || strings.Contains(got, "old claude block") {
			t.Fatalf("%s = %q, want old bootloader removed", relPath, got)
		}
		if count := strings.Count(got, "<!-- memento:start -->"); count != 1 {
			t.Fatalf("%s start sentinel count = %d, want 1; contents = %q", relPath, count, got)
		}
	}
}

func TestInitBootloaderUsesCustomMemoryDirectoryPath(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "docs/project-memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "AGENTS.md")
	for _, want := range []string{
		"Durable project knowledge lives in `docs/project-memory`: curated design decisions, specs, constraints, and discoveries, not task state.",
		"Before any other memento action, run `memento orient`.",
		"Run `memento brief` when you need the doc landscape; it is pull-only, not a mandatory second step.",
		"Use `memento read <key|@N>#<heading>` or `memento read <key|@N>` instead of grep/cat: it emits link-graph metadata on stderr and supports section extraction.",
		"`@N` indexes come from `brief`; `memento read` writes `binding: ratified|unratified` plus non-empty role-flattened link lines to stderr before stdout content.",
		"Discoveries that outlive a task belong in `docs/project-memory`, not the task store.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("AGENTS.md = %q, want it to contain %q", got, want)
		}
	}
	assertNoWritingGuideReference(t, "AGENTS.md", got)
}

func TestInitBootloaderIsIdempotent(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "AGENTS.md", "# Agent Instructions\n")
	writeSetupFile(t, repo, "CLAUDE.md", "# Claude Instructions\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("first Init() error = %v, want nil", err)
	}
	firstAgents := readSetupFile(t, repo, "AGENTS.md")
	firstClaude := readSetupFile(t, repo, "CLAUDE.md")
	firstHook := readSetupFile(t, repo, ".git/hooks/pre-commit")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("second Init() error = %v, want nil", err)
	}
	secondAgents := readSetupFile(t, repo, "AGENTS.md")
	secondClaude := readSetupFile(t, repo, "CLAUDE.md")
	secondHook := readSetupFile(t, repo, ".git/hooks/pre-commit")

	if secondAgents != firstAgents {
		t.Fatalf("AGENTS.md changed on rerun:\nfirst:\n%s\nsecond:\n%s", firstAgents, secondAgents)
	}
	if secondClaude != firstClaude {
		t.Fatalf("CLAUDE.md changed on rerun:\nfirst:\n%s\nsecond:\n%s", firstClaude, secondClaude)
	}
	if secondHook != firstHook {
		t.Fatalf("pre-commit hook changed on rerun:\nfirst:\n%s\nsecond:\n%s", firstHook, secondHook)
	}
	if count := strings.Count(secondAgents, "<!-- memento:start -->"); count != 1 {
		t.Fatalf("AGENTS.md start sentinel count = %d, want 1; contents = %q", count, secondAgents)
	}
	if count := strings.Count(secondClaude, "<!-- memento:start -->"); count != 1 {
		t.Fatalf("CLAUDE.md start sentinel count = %d, want 1; contents = %q", count, secondClaude)
	}
	if count := strings.Count(secondHook, "# memento:start"); count != 1 {
		t.Fatalf("pre-commit hook start sentinel count = %d, want 1; contents = %q", count, secondHook)
	}
}

func TestInitInstallsClaudeOrientSessionStartHook(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, ".claude/settings.json", `{
  "permissions": {"allow": ["Bash"]},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Write", "hooks": [{"type": "command", "command": "/keep/pre-write.sh"}]}
    ],
    "SessionStart": [
      {"matcher": "startup", "hooks": [{"type": "command", "command": "/keep/session.sh"}]}
    ]
  }
}
`)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	settings := readClaudeSettings(t, repo)
	hooks := settingsObject(t, settings, "hooks")
	sessionStart := settingsArray(t, hooks, "SessionStart")
	if len(sessionStart) != 2 {
		t.Fatalf("SessionStart hooks = %#v, want existing plus memento hook", sessionStart)
	}
	if !settingsJSONContainsCommand(sessionStart, "/keep/session.sh") {
		t.Fatalf("SessionStart hooks = %#v, want existing hook preserved", sessionStart)
	}

	command := filepath.Join(repo, ".claude", "memento-orient-session-start.sh")
	if !settingsJSONContainsCommand(sessionStart, command) {
		t.Fatalf("SessionStart hooks = %#v, want memento command %q", sessionStart, command)
	}
	if !settingsJSONContainsCommand(settingsArray(t, hooks, "PreToolUse"), "/keep/pre-write.sh") {
		t.Fatalf("hooks = %#v, want unrelated PreToolUse hook preserved", hooks)
	}

	script := readSetupFile(t, repo, ".claude/memento-orient-session-start.sh")
	if !strings.Contains(script, "memento compile") || !strings.Contains(script, "memento orient") {
		t.Fatalf("Claude orient script = %q, want compile and orient commands", script)
	}
}

func TestInitUpdatesExistingClaudeOrientHookWithoutDuplicating(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, ".claude/settings.json", `{
  "hooks": {
    "SessionStart": [
      {"matcher": "startup", "hooks": [{"type": "command", "command": "/old/memento-orient-session-start.sh"}]}
    ]
  }
}
`)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	settings := readClaudeSettings(t, repo)
	sessionStart := settingsArray(t, settingsObject(t, settings, "hooks"), "SessionStart")
	command := filepath.Join(repo, ".claude", "memento-orient-session-start.sh")
	if count := settingsJSONCommandCount(sessionStart, command); count != 1 {
		t.Fatalf("memento SessionStart command count = %d, want 1; hooks = %#v", count, sessionStart)
	}
	if settingsJSONContainsCommand(sessionStart, "/old/memento-orient-session-start.sh") {
		t.Fatalf("SessionStart hooks = %#v, want old memento command replaced", sessionStart)
	}
}

func TestClaudeOrientHookRunsCompileBeforeOrient(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Claude hook is a bash script")
	}

	repo := t.TempDir()
	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "calls.log")
	writeSetupFile(t, binDir, "memento", `#!/bin/sh
printf '%s\n' "$1" >> "$MEMENTO_TEST_LOG"
case "$1" in
  compile) exit 0 ;;
  orient) printf 'ORIENT OK\n'; exit 0 ;;
  *) exit 2 ;;
esac
`)
	if err := os.Chmod(filepath.Join(binDir, "memento"), 0o755); err != nil {
		t.Fatalf("chmod fake memento: %v", err)
	}

	stdout := runClaudeOrientHook(t, repo, binDir, logPath, "")
	gotLog := readSetupFile(t, filepath.Dir(logPath), filepath.Base(logPath))
	if gotLog != "compile\norient\n" {
		t.Fatalf("memento call log = %q, want compile before orient", gotLog)
	}
	if !strings.Contains(hookAdditionalContext(t, stdout), "ORIENT OK") {
		t.Fatalf("hook stdout = %q, want orient output in additionalContext", stdout)
	}
}

func TestClaudeOrientHookRunsOrientWhenCompileFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Claude hook is a bash script")
	}

	repo := t.TempDir()
	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "calls.log")
	writeSetupFile(t, binDir, "memento", `#!/bin/sh
printf '%s\n' "$1" >> "$MEMENTO_TEST_LOG"
case "$1" in
  compile) printf 'compile broke\n' >&2; exit 42 ;;
  orient) printf 'ORIENT AFTER FAILURE\n'; exit 0 ;;
  *) exit 2 ;;
esac
`)
	if err := os.Chmod(filepath.Join(binDir, "memento"), 0o755); err != nil {
		t.Fatalf("chmod fake memento: %v", err)
	}

	stdout := runClaudeOrientHook(t, repo, binDir, logPath, "")
	gotLog := readSetupFile(t, filepath.Dir(logPath), filepath.Base(logPath))
	if gotLog != "compile\norient\n" {
		t.Fatalf("memento call log = %q, want compile failure followed by orient", gotLog)
	}
	context := hookAdditionalContext(t, stdout)
	if !strings.Contains(context, "memento compile failed; continuing with memento orient.") {
		t.Fatalf("additionalContext = %q, want concise compile failure note", context)
	}
	if !strings.Contains(context, "ORIENT AFTER FAILURE") {
		t.Fatalf("additionalContext = %q, want orient output after compile failure", context)
	}
}

func TestInitInstallsClaudeWriteEnforcementHooks(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	hooks := settingsObject(t, readClaudeSettings(t, repo), "hooks")

	preCommand := filepath.Join(repo, ".claude", "memento-pre-write-vault-guard.sh")
	assertManagedHook(t, hooks, "PreToolUse", "Write|Edit|MultiEdit|Bash", preCommand)
	postCommand := filepath.Join(repo, ".claude", "memento-post-write-compile.sh")
	assertManagedHook(t, hooks, "PostToolUse", "Write|Edit|MultiEdit|Bash", postCommand)

	// The SessionStart orient hook survives the ADR-0031 rework.
	orientCommand := filepath.Join(repo, ".claude", "memento-orient-session-start.sh")
	if !settingsJSONContainsCommand(settingsArray(t, hooks, "SessionStart"), orientCommand) {
		t.Fatalf("SessionStart hooks = %#v, want orient command %q preserved", hooks["SessionStart"], orientCommand)
	}

	pre := readSetupFile(t, repo, ".claude/memento-pre-write-vault-guard.sh")
	if !strings.Contains(pre, "check-write") || !strings.Contains(pre, "fail-closed") {
		t.Fatalf("pre-write hook script = %q, want a fail-closed check-write delegate", pre)
	}
	post := readSetupFile(t, repo, ".claude/memento-post-write-compile.sh")
	if !strings.Contains(post, "memento compile") || !strings.Contains(post, "DRIFT ALARM") {
		t.Fatalf("post-write hook script = %q, want a compile + drift-alarm wrapper", post)
	}

	if runtime.GOOS != "windows" {
		for _, rel := range []string{".claude/memento-pre-write-vault-guard.sh", ".claude/memento-post-write-compile.sh"} {
			info, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatalf("stat %s: %v", rel, err)
			}
			if info.Mode().Perm()&0o111 == 0 {
				t.Fatalf("%s mode = %v, want executable bit set", rel, info.Mode().Perm())
			}
		}
	}
}

func TestInitInstalledWriteHooksMatchCanonicalScripts(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	repoRoot := setupRepoRoot(t)
	cases := map[string]string{
		".claude/memento-pre-write-vault-guard.sh": "scripts/agent-hooks/pre-write-vault-guard.sh",
		".claude/memento-post-write-compile.sh":    "scripts/agent-hooks/post-write-compile.sh",
	}
	for installed, canonical := range cases {
		got := readSetupFile(t, repo, installed)
		want := readSetupFile(t, repoRoot, canonical)
		if got != want {
			t.Fatalf("%s drifted from canonical %s:\ninstalled:\n%s\ncanonical:\n%s", installed, canonical, got, want)
		}
	}
}

func TestInitDoesNotInstallWriteSkill(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	// ADR-0031 retired the write verb the skill routed through; init must not
	// scaffold the skill source nor install the Claude skill.
	for _, rel := range []string{
		"memory/_memento/skills/write.md",
		".claude/skills/memento-write/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("%s stat err = %v, want file not to exist", rel, err)
		}
	}
}

func TestInitWriteEnforcementHooksAreIdempotent(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, ".claude/settings.json", `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Write", "hooks": [{"type": "command", "command": "/keep/pre.sh"}]}
    ],
    "PostToolUse": [
      {"matcher": "Edit", "hooks": [{"type": "command", "command": "/keep/post.sh"}]}
    ]
  }
}
`)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("first Init() error = %v, want nil", err)
	}
	first := readSetupFile(t, repo, ".claude/settings.json")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("second Init() error = %v, want nil", err)
	}
	second := readSetupFile(t, repo, ".claude/settings.json")

	if second != first {
		t.Fatalf(".claude/settings.json changed on rerun:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	hooks := settingsObject(t, readClaudeSettings(t, repo), "hooks")
	preCommand := filepath.Join(repo, ".claude", "memento-pre-write-vault-guard.sh")
	if count := settingsJSONCommandCount(settingsArray(t, hooks, "PreToolUse"), preCommand); count != 1 {
		t.Fatalf("PreToolUse memento command count = %d, want 1; hooks = %#v", count, hooks["PreToolUse"])
	}
	postCommand := filepath.Join(repo, ".claude", "memento-post-write-compile.sh")
	if count := settingsJSONCommandCount(settingsArray(t, hooks, "PostToolUse"), postCommand); count != 1 {
		t.Fatalf("PostToolUse memento command count = %d, want 1; hooks = %#v", count, hooks["PostToolUse"])
	}
	// Unrelated hooks the user already had are preserved.
	if !settingsJSONContainsCommand(settingsArray(t, hooks, "PreToolUse"), "/keep/pre.sh") {
		t.Fatalf("PreToolUse hooks = %#v, want unrelated /keep/pre.sh preserved", hooks["PreToolUse"])
	}
	if !settingsJSONContainsCommand(settingsArray(t, hooks, "PostToolUse"), "/keep/post.sh") {
		t.Fatalf("PostToolUse hooks = %#v, want unrelated /keep/post.sh preserved", hooks["PostToolUse"])
	}
}

func markCodexRepo(t *testing.T, repo string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repo, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
}

func readCodexHooks(t *testing.T, repo string) map[string]any {
	t.Helper()
	var hooks map[string]any
	if err := json.Unmarshal([]byte(readSetupFile(t, repo, ".codex/hooks.json")), &hooks); err != nil {
		t.Fatalf("unmarshal .codex/hooks.json: %v", err)
	}
	return hooks
}

func TestInitInstallsCodexHooksWhenCodexDetected(t *testing.T) {
	repo := t.TempDir()
	markCodexRepo(t, repo)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	// config.toml uses the path-indirection form so the install is a JSON file
	// parallel to Claude's settings.json.
	config := readSetupFile(t, repo, ".codex/config.toml")
	for _, want := range []string{"# memento:start", `hooks = "hooks.json"`, "# memento:end"} {
		if !strings.Contains(config, want) {
			t.Fatalf(".codex/config.toml = %q, want it to contain %q", config, want)
		}
	}

	hooks := readCodexHooks(t, repo)
	preCommand := filepath.Join(repo, ".codex", "memento-pre-write-vault-guard.sh")
	assertManagedHook(t, hooks, "PreToolUse", codexWriteHookMatcher, preCommand)
	postCommand := filepath.Join(repo, ".codex", "memento-post-write-compile.sh")
	assertManagedHook(t, hooks, "PostToolUse", codexWriteHookMatcher, postCommand)
	orientCommand := filepath.Join(repo, ".codex", "memento-orient-session-start.sh")
	assertManagedHook(t, hooks, "SessionStart", codexOrientHookMatcher, orientCommand)

	// timeout_sec rides on every codex handler (b15 schema): the gate gets the
	// tight budget, the compile-backed hooks a wider one.
	rawHooks := readSetupFile(t, repo, ".codex/hooks.json")
	if !strings.Contains(rawHooks, `"timeout_sec": 5`) {
		t.Fatalf(".codex/hooks.json = %q, want the PreToolUse gate timeout_sec 5", rawHooks)
	}
	if !strings.Contains(rawHooks, `"timeout_sec": 30`) {
		t.Fatalf(".codex/hooks.json = %q, want a compile-hook timeout_sec 30", rawHooks)
	}

	if runtime.GOOS != "windows" {
		for _, rel := range []string{
			".codex/memento-pre-write-vault-guard.sh",
			".codex/memento-post-write-compile.sh",
			".codex/memento-orient-session-start.sh",
		} {
			info, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatalf("stat %s: %v", rel, err)
			}
			if info.Mode().Perm()&0o111 == 0 {
				t.Fatalf("%s mode = %v, want executable bit set", rel, info.Mode().Perm())
			}
		}
	}
}

func TestInitSkipsCodexWhenUndetected(t *testing.T) {
	repo := t.TempDir()

	// No .codex dir: the additive invariant says an undetectable family degrades
	// discoverability (no codex gate) but never the CLI nor the baseline Claude
	// install.
	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	for _, rel := range []string{".codex/hooks.json", ".codex/config.toml"} {
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("%s stat err = %v, want file not to exist", rel, err)
		}
	}
	// Claude (the baseline family) is still wired.
	hooks := settingsObject(t, readClaudeSettings(t, repo), "hooks")
	preCommand := filepath.Join(repo, ".claude", "memento-pre-write-vault-guard.sh")
	assertManagedHook(t, hooks, "PreToolUse", "Write|Edit|MultiEdit|Bash", preCommand)
}

func TestInitCodexScriptsMatchCanonicalScripts(t *testing.T) {
	repo := t.TempDir()
	markCodexRepo(t, repo)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	// codex reuses the dumb-pipe scripts verbatim — check-write's PreToolUse verdict
	// JSON is byte-identical-valid on codex (b15 spike), so the installed codex copy
	// must equal both the canonical script and the Claude-installed one.
	repoRoot := setupRepoRoot(t)
	cases := map[string]string{
		".codex/memento-pre-write-vault-guard.sh": "scripts/agent-hooks/pre-write-vault-guard.sh",
		".codex/memento-post-write-compile.sh":    "scripts/agent-hooks/post-write-compile.sh",
	}
	for installed, canonical := range cases {
		got := readSetupFile(t, repo, installed)
		want := readSetupFile(t, repoRoot, canonical)
		if got != want {
			t.Fatalf("%s drifted from canonical %s:\ninstalled:\n%s\ncanonical:\n%s", installed, canonical, got, want)
		}
		claudeName := strings.Replace(installed, ".codex/", ".claude/", 1)
		if claudeGot := readSetupFile(t, repo, claudeName); claudeGot != got {
			t.Fatalf("%s differs from %s; want both families to install the same script", installed, claudeName)
		}
	}
}

func TestInitCodexHooksAreIdempotent(t *testing.T) {
	repo := t.TempDir()
	markCodexRepo(t, repo)
	writeSetupFile(t, repo, ".codex/hooks.json", `{
  "PreToolUse": [
    {"matcher": "Shell", "hooks": [{"type": "command", "command": "/keep/codex-pre.sh"}]}
  ]
}
`)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("first Init() error = %v, want nil", err)
	}
	firstHooks := readSetupFile(t, repo, ".codex/hooks.json")
	firstConfig := readSetupFile(t, repo, ".codex/config.toml")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("second Init() error = %v, want nil", err)
	}
	secondHooks := readSetupFile(t, repo, ".codex/hooks.json")
	secondConfig := readSetupFile(t, repo, ".codex/config.toml")

	if secondHooks != firstHooks {
		t.Fatalf(".codex/hooks.json changed on rerun:\nfirst:\n%s\nsecond:\n%s", firstHooks, secondHooks)
	}
	if secondConfig != firstConfig {
		t.Fatalf(".codex/config.toml changed on rerun:\nfirst:\n%s\nsecond:\n%s", firstConfig, secondConfig)
	}

	hooks := readCodexHooks(t, repo)
	preCommand := filepath.Join(repo, ".codex", "memento-pre-write-vault-guard.sh")
	if count := settingsJSONCommandCount(settingsArray(t, hooks, "PreToolUse"), preCommand); count != 1 {
		t.Fatalf("PreToolUse memento command count = %d, want 1; hooks = %#v", count, hooks["PreToolUse"])
	}
	// The user's unrelated codex hook is preserved.
	if !settingsJSONContainsCommand(settingsArray(t, hooks, "PreToolUse"), "/keep/codex-pre.sh") {
		t.Fatalf("PreToolUse hooks = %#v, want unrelated /keep/codex-pre.sh preserved", hooks["PreToolUse"])
	}
	if count := strings.Count(secondConfig, "# memento:start"); count != 1 {
		t.Fatalf(".codex/config.toml start sentinel count = %d, want 1; contents = %q", count, secondConfig)
	}
}

func TestInitSurfacesCodexHookTrustStep(t *testing.T) {
	repo := t.TempDir()
	markCodexRepo(t, repo)

	var notices bytes.Buffer
	if _, err := InitWithOptions(repo, "memory", InitOptions{NoticeWriter: &notices}); err != nil {
		t.Fatalf("InitWithOptions() error = %v, want nil", err)
	}

	got := notices.String()
	for _, want := range []string{"untrusted", "fail-open", "trust", "--dangerously-bypass-hook-trust"} {
		if !strings.Contains(got, want) {
			t.Fatalf("codex notice = %q, want it to mention %q", got, want)
		}
	}
}

func TestInitDoesNotSurfaceCodexTrustWhenCodexUndetected(t *testing.T) {
	repo := t.TempDir()

	var notices bytes.Buffer
	if _, err := InitWithOptions(repo, "memory", InitOptions{NoticeWriter: &notices}); err != nil {
		t.Fatalf("InitWithOptions() error = %v, want nil", err)
	}

	if strings.Contains(notices.String(), "codex") {
		t.Fatalf("notice = %q, want no codex guidance without a codex repo", notices.String())
	}
}

func TestInitPreservesForeignCodexHooksDeclaration(t *testing.T) {
	repo := t.TempDir()
	markCodexRepo(t, repo)
	existing := "[model]\nname = \"gpt\"\n\nhooks = \"./mine.json\"\n"
	writeSetupFile(t, repo, ".codex/config.toml", existing)

	var notices bytes.Buffer
	if _, err := InitWithOptions(repo, "memory", InitOptions{NoticeWriter: &notices}); err != nil {
		t.Fatalf("InitWithOptions() error = %v, want nil", err)
	}

	// memento must not corrupt a user-authored hooks declaration; it leaves the
	// config untouched and tells the user to wire the gate themselves.
	if got := readSetupFile(t, repo, ".codex/config.toml"); got != existing {
		t.Fatalf(".codex/config.toml changed to %q, want it left untouched", got)
	}
	if !strings.Contains(notices.String(), "already declares hooks") {
		t.Fatalf("notice = %q, want a manual-wiring notice", notices.String())
	}
	// hooks.json is still staged so a manual wiring has a target.
	if _, err := os.Stat(filepath.Join(repo, ".codex", "hooks.json")); err != nil {
		t.Fatalf("stat .codex/hooks.json: %v", err)
	}
}

func TestInitPreservesUnrelatedCodexConfig(t *testing.T) {
	repo := t.TempDir()
	markCodexRepo(t, repo)
	// A real config opens with a [model] table; appending the bare hooks key at
	// the end of the file would scope it under [model] (model.hooks), so the
	// top-level hooks pointer codex needs would never exist. The block must land
	// above the first table header.
	existing := "[model]\nname = \"gpt\"\n"
	writeSetupFile(t, repo, ".codex/config.toml", existing)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, ".codex/config.toml")
	if !strings.Contains(got, "[model]\nname = \"gpt\"") {
		t.Fatalf(".codex/config.toml = %q, want existing table preserved", got)
	}
	if !strings.Contains(got, `hooks = "hooks.json"`) {
		t.Fatalf(".codex/config.toml = %q, want memento hooks wiring written", got)
	}
	assertCodexHooksTopLevel(t, got)
}

// assertCodexHooksTopLevel fails unless the memento `hooks = "hooks.json"` key
// sits before the first TOML table header, where it binds at top level rather
// than to a preceding [table]. memento has no TOML parser dependency, so this is
// a positional check over the written file.
func assertCodexHooksTopLevel(t *testing.T, contents string) {
	t.Helper()
	hooksAt := strings.Index(contents, `hooks = "hooks.json"`)
	if hooksAt == -1 {
		t.Fatalf(".codex/config.toml = %q, want a hooks key", contents)
	}
	firstTableAt := -1
	offset := 0
	for _, line := range strings.Split(contents, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			firstTableAt = offset
			break
		}
		offset += len(line) + 1
	}
	if firstTableAt != -1 && hooksAt > firstTableAt {
		t.Fatalf(".codex/config.toml = %q, want hooks key before the first table header so it stays top-level", contents)
	}
}

func TestInitPlacesCodexHooksAboveLeadingCommentedTable(t *testing.T) {
	repo := t.TempDir()
	markCodexRepo(t, repo)
	// Leading comments and blank lines must stay above the block, but the hooks
	// key must still precede the first table header.
	existing := "# my codex config\n\n[model]\nname = \"gpt-5\"\n\n[tools]\nshell = true\n"
	writeSetupFile(t, repo, ".codex/config.toml", existing)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, ".codex/config.toml")
	if !strings.HasPrefix(got, "# my codex config") {
		t.Fatalf(".codex/config.toml = %q, want leading comment preserved at start", got)
	}
	if !strings.Contains(got, "[tools]\nshell = true") {
		t.Fatalf(".codex/config.toml = %q, want unrelated tables preserved", got)
	}
	assertCodexHooksTopLevel(t, got)
}

func TestInitPlacesCodexHooksOutsideLeadingMultiLineArray(t *testing.T) {
	repo := t.TempDir()
	markCodexRepo(t, repo)
	// A config that opens with a multi-line array whose continuation lines start
	// with '[' (a nested array). The naive "first line starting with [" scan trips
	// on the continuation and splits the array, emitting invalid TOML. The block
	// must land outside the array and stay top-level (before the first real table).
	existing := "deny = [\n  [\"shell\"],\n]\n\n[model]\nname = \"gpt\"\n"
	writeSetupFile(t, repo, ".codex/config.toml", existing)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, ".codex/config.toml")
	if !strings.Contains(got, `hooks = "hooks.json"`) {
		t.Fatalf(".codex/config.toml = %q, want memento hooks wiring written", got)
	}
	// The block must not be inserted between `deny = [` and its closing `]`.
	denyOpen := strings.Index(got, "deny = [")
	arrayClose := strings.Index(got, "\n]")
	hooksAt := strings.Index(got, `hooks = "hooks.json"`)
	if denyOpen == -1 || arrayClose == -1 {
		t.Fatalf(".codex/config.toml = %q, want the deny array preserved", got)
	}
	if hooksAt > denyOpen && hooksAt < arrayClose {
		t.Fatalf(".codex/config.toml = %q, want hooks key outside the deny array, not split inside it", got)
	}
	// And it must remain top-level: before the [model] table, after the array.
	modelAt := strings.Index(got, "[model]")
	if modelAt == -1 || hooksAt > modelAt {
		t.Fatalf(".codex/config.toml = %q, want hooks key before the [model] table so it stays top-level", got)
	}
	if hooksAt < arrayClose {
		t.Fatalf(".codex/config.toml = %q, want hooks key after the closed deny array", got)
	}
}

func TestInitWritesObsidianGitignoreStanzaIdempotently(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, ".gitignore", "build/\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("first Init() error = %v, want nil", err)
	}
	first := readSetupFile(t, repo, ".gitignore")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("second Init() error = %v, want nil", err)
	}
	second := readSetupFile(t, repo, ".gitignore")

	if second != first {
		t.Fatalf(".gitignore changed on rerun:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if !strings.HasPrefix(second, "build/\n") {
		t.Fatalf(".gitignore = %q, want existing content preserved at start", second)
	}
	for _, want := range []string{
		"# memento:gitignore:start",
		"**/.obsidian/workspace*",
		"**/.obsidian/cache",
		"**/_memento/brief.md",
		"**/.memento/" + enforce.PendingFileName,
		"**/.memento/" + enforce.DecisionLogFileName,
		"# memento:gitignore:end",
	} {
		if !strings.Contains(second, want) {
			t.Fatalf(".gitignore = %q, want it to contain %q", second, want)
		}
	}
	if count := strings.Count(second, "# memento:gitignore:start"); count != 1 {
		t.Fatalf(".gitignore start sentinel count = %d, want 1; contents = %q", count, second)
	}

	ignore := readSetupFile(t, repo, "memory/.mementoignore")
	if strings.Contains(ignore, ".obsidian/workspace") || strings.Contains(ignore, ".obsidian/cache") {
		t.Fatalf(".mementoignore = %q, want Obsidian UI noise excluded from memento ignore", ignore)
	}
}

func TestInitWritesBriefIgnoreEntriesForGreenfieldVault(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	gitignore := readSetupFile(t, repo, ".gitignore")
	if !hasSetupLine(gitignore, "**/_memento/brief.md") {
		t.Fatalf(".gitignore = %q, want file-specific brief ignore entry", gitignore)
	}
	if hasSetupLine(gitignore, "memory/_memento/brief.md") || hasSetupLine(gitignore, "memory/_memento/") {
		t.Fatalf(".gitignore = %q, want no folder-wide _memento ignore entry", gitignore)
	}
	if !hasSetupLine(gitignore, "**/.memento/unlock-grants.json") {
		t.Fatalf(".gitignore = %q, want file-scoped unlock-grants sidecar ignore entry", gitignore)
	}
	if hasSetupLine(gitignore, "**/.memento/") || hasSetupLine(gitignore, "**/.memento/manifest.json") {
		t.Fatalf(".gitignore = %q, want manifest/config under .memento to stay tracked", gitignore)
	}

	mementoignore := readSetupFile(t, repo, "memory/.mementoignore")
	if !hasSetupLine(mementoignore, "_memento/") {
		t.Fatalf(".mementoignore = %q, want structural _memento/ namespace ignore entry", mementoignore)
	}
}

func TestInitAddsBriefIgnoreEntriesWhenAdoptingExistingVault(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, ".gitignore", "build/\n")
	writeSetupFile(t, repo, "memory/note.md", "# Existing note\n\nKeep this.\n")
	writeSetupFile(t, repo, "memory/.mementoignore", "drafts/\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	gitignore := readSetupFile(t, repo, ".gitignore")
	if !strings.HasPrefix(gitignore, "build/\n") {
		t.Fatalf(".gitignore = %q, want existing content preserved at start", gitignore)
	}
	if !hasSetupLine(gitignore, "**/_memento/brief.md") {
		t.Fatalf(".gitignore = %q, want file-specific brief ignore entry", gitignore)
	}
	if hasSetupLine(gitignore, "memory/_memento/brief.md") || hasSetupLine(gitignore, "memory/_memento/") {
		t.Fatalf(".gitignore = %q, want no folder-wide _memento ignore entry", gitignore)
	}

	mementoignore := readSetupFile(t, repo, "memory/.mementoignore")
	if !strings.HasPrefix(mementoignore, "drafts/\n") {
		t.Fatalf(".mementoignore = %q, want existing content preserved at start", mementoignore)
	}
	if !hasSetupLine(mementoignore, "_memento/") {
		t.Fatalf(".mementoignore = %q, want structural _memento/ namespace ignore entry", mementoignore)
	}
}

func TestInitBriefIgnoreEntriesAreIdempotentWhenPresent(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, ".gitignore", strings.Join([]string{
		"build/",
		"",
		"# memento:gitignore:start",
		"# Obsidian per-machine UI state",
		"**/.obsidian/workspace*",
		"**/.obsidian/cache",
		"# Memento generated artifacts",
		"**/_memento/brief.md",
		"# memento:gitignore:end",
		"",
	}, "\n"))
	writeSetupFile(t, repo, "memory/note.md", "# Existing note\n\nKeep this.\n")
	writeSetupFile(t, repo, "memory/.mementoignore", "drafts/\n\n_memento/\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("first Init() error = %v, want nil", err)
	}
	firstGitignore := readSetupFile(t, repo, ".gitignore")
	firstMementoignore := readSetupFile(t, repo, "memory/.mementoignore")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("second Init() error = %v, want nil", err)
	}
	secondGitignore := readSetupFile(t, repo, ".gitignore")
	secondMementoignore := readSetupFile(t, repo, "memory/.mementoignore")

	if secondGitignore != firstGitignore {
		t.Fatalf(".gitignore changed on rerun:\nfirst:\n%s\nsecond:\n%s", firstGitignore, secondGitignore)
	}
	if secondMementoignore != firstMementoignore {
		t.Fatalf(".mementoignore changed on rerun:\nfirst:\n%s\nsecond:\n%s", firstMementoignore, secondMementoignore)
	}
	if count := countSetupLine(secondGitignore, "**/_memento/brief.md"); count != 1 {
		t.Fatalf(".gitignore brief entry count = %d, want 1; contents = %q", count, secondGitignore)
	}
	if count := countSetupLine(secondMementoignore, "_memento/"); count != 1 {
		t.Fatalf(".mementoignore namespace entry count = %d, want 1; contents = %q", count, secondMementoignore)
	}
}

func TestInitCreatesUsingMementoGuideForGreenfieldVault(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	info, err := os.Stat(filepath.Join(repo, "memory", "_memento"))
	if err != nil {
		t.Fatalf("stat _memento: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("_memento mode = %v, want directory", info.Mode())
	}

	got := readSetupFile(t, repo, "memory/_memento/Using Memento.md")
	assertUsingMementoGuide(t, got)

	manifest := readSetupFile(t, repo, "memory/.memento/manifest.json")
	if strings.Contains(manifest, `_memento/Using Memento.md`) {
		t.Fatalf("manifest = %q, want no _memento/Using Memento.md entry", manifest)
	}
}

func TestInitDoesNotCreateLegacyWritingGuideForGreenfieldVault(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	// The writing guide now lives at _memento/conventions/writing.md; greenfield
	// init must not also scaffold the superseded _memento/writing.md path.
	if _, err := os.Stat(filepath.Join(repo, "memory", "_memento", "writing.md")); !os.IsNotExist(err) {
		t.Fatalf("_memento/writing.md stat err = %v, want file not to exist", err)
	}
	got := readSetupFile(t, repo, "memory/_memento/conventions/writing.md")
	if !strings.Contains(got, "when_to_read: before authoring a memento vault write") {
		t.Fatalf("_memento/conventions/writing.md = %q, want the writing convention scaffolded instead", got)
	}
}

func TestInitCreatesDefaultConventionTemplatesForGreenfieldVault(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	cases := map[string]string{
		"writing":     "when_to_read: before authoring a memento vault write",
		"summarising": "when_to_read: when writing or revising a note summary",
		"conventions": "when_to_read: before adding or editing a convention file",
	}
	for stem, wantWhen := range cases {
		got := readSetupFile(t, repo, "memory/_memento/conventions/"+stem+".md")
		front, body, ok := strings.Cut(got, "---\n\n")
		if !ok {
			t.Fatalf("%s.md = %q, want frontmatter terminated before body", stem, got)
		}
		if !strings.Contains(front, "title:") {
			t.Fatalf("%s.md = %q, want a title field", stem, got)
		}
		if !strings.Contains(front, wantWhen) {
			t.Fatalf("%s.md = %q, want %q", stem, got, wantWhen)
		}
		for _, forbidden := range []string{"mode:", "summary:", "tags:"} {
			if strings.Contains(front, forbidden) {
				t.Fatalf("%s.md frontmatter = %q, want no %q (title and when_to_read only)", stem, front, forbidden)
			}
		}
		if strings.TrimSpace(body) == "" {
			t.Fatalf("%s.md = %q, want a non-empty body", stem, got)
		}
	}
}

func TestInitConventionTemplatesAreProjectNeutral(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	for _, stem := range []string{"writing", "summarising", "conventions"} {
		got := readSetupFile(t, repo, "memory/_memento/conventions/"+stem+".md")
		for _, forbidden := range []string{"beads", "bd ", "ralph", "Ralph"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("%s.md = %q, want project-neutral template free of %q", stem, got, forbidden)
			}
		}
	}
}

func TestInitPreservesExistingConventionFilesWhenAdopting(t *testing.T) {
	repo := t.TempDir()
	existing := "---\ntitle: My writing convention\nwhen_to_read: whenever I say so\n---\n\n# Mine\n\nKeep this exactly.\n"
	writeSetupFile(t, repo, "memory/note.md", "# Existing note\n\nKeep this.\n")
	writeSetupFile(t, repo, "memory/_memento/conventions/writing.md", existing)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	if got := readSetupFile(t, repo, "memory/_memento/conventions/writing.md"); got != existing {
		t.Fatalf("_memento/conventions/writing.md changed to %q, want %q", got, existing)
	}
	// Missing default conventions are still scaffolded alongside the preserved one.
	if got := readSetupFile(t, repo, "memory/_memento/conventions/summarising.md"); !strings.Contains(got, "when_to_read:") {
		t.Fatalf("_memento/conventions/summarising.md = %q, want default scaffolded", got)
	}
}

func TestInitConventionTemplatesAreIdempotent(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("first Init() error = %v, want nil", err)
	}
	first := readSetupFile(t, repo, "memory/_memento/conventions/writing.md")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("second Init() error = %v, want nil", err)
	}
	second := readSetupFile(t, repo, "memory/_memento/conventions/writing.md")

	if second != first {
		t.Fatalf("_memento/conventions/writing.md changed on rerun:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestInitConventionFilesDoNotAppearInManifest(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	manifest := readSetupFile(t, repo, "memory/.memento/manifest.json")
	if strings.Contains(manifest, "_memento/conventions/") {
		t.Fatalf("manifest = %q, want convention files excluded from the normal manifest", manifest)
	}
}

func TestInitCreatesUsingMementoGuideWhenAdoptingExistingVault(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "memory/note.md", "# Existing note\n\nKeep this.\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "memory/_memento/Using Memento.md")
	assertUsingMementoGuide(t, got)
	if _, err := os.Stat(filepath.Join(repo, "memory", "example.md")); !os.IsNotExist(err) {
		t.Fatalf("example.md stat err = %v, want file not to exist", err)
	}
}

func TestInitDoesNotModifyExistingWritingGuideWhenAdopting(t *testing.T) {
	repo := t.TempDir()
	existing := "---\nmode: read-only\n---\n# My writing guide\n\nKeep this exactly.\n"
	writeSetupFile(t, repo, "memory/note.md", "# Existing note\n\nKeep this.\n")
	writeSetupFile(t, repo, "memory/_memento/writing.md", existing)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "memory/_memento/writing.md")
	if got != existing {
		t.Fatalf("_memento/writing.md changed to %q, want %q", got, existing)
	}
}

func TestInitDoesNotCreateWritingGuideWhenAdoptingWithoutOne(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "memory/note.md", "# Existing note\n\nKeep this.\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	if _, err := os.Stat(filepath.Join(repo, "memory", "_memento", "writing.md")); !os.IsNotExist(err) {
		t.Fatalf("_memento/writing.md stat err = %v, want file not to exist", err)
	}
}

func TestInitDoesNotModifyExistingUsingMementoGuide(t *testing.T) {
	repo := t.TempDir()
	existing := "# My local guide\n\nKeep this exactly.\n"
	writeSetupFile(t, repo, "memory/_memento/Using Memento.md", existing)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "memory/_memento/Using Memento.md")
	if got != existing {
		t.Fatalf("_memento/Using Memento.md changed to %q, want %q", got, existing)
	}
}

func TestInitLeavesExistingMementoReadmeUntouched(t *testing.T) {
	repo := t.TempDir()
	existing := "# Old README\n\nDelete manually if you do not want this.\n"
	writeSetupFile(t, repo, "memory/_memento/README.md", existing)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	if got := readSetupFile(t, repo, "memory/_memento/README.md"); got != existing {
		t.Fatalf("_memento/README.md changed to %q, want %q", got, existing)
	}
	guide := readSetupFile(t, repo, "memory/_memento/Using Memento.md")
	assertUsingMementoGuide(t, guide)
}

func TestInitUsingMementoGuideIsIdempotent(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("first Init() error = %v, want nil", err)
	}
	first := readSetupFile(t, repo, "memory/_memento/Using Memento.md")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("second Init() error = %v, want nil", err)
	}
	second := readSetupFile(t, repo, "memory/_memento/Using Memento.md")

	if second != first {
		t.Fatalf("_memento/Using Memento.md changed on rerun:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestInitDoesNotIndexUsingMementoGuideWhenAdoptingExistingIgnore(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "memory/.mementoignore", "drafts/\n")
	writeSetupFile(t, repo, "memory/note.md", "# Existing note\n\nKeep this.\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	mementoignore := readSetupFile(t, repo, "memory/.mementoignore")
	if !hasSetupLine(mementoignore, "_memento/") {
		t.Fatalf(".mementoignore = %q, want structural _memento/ namespace ignore entry", mementoignore)
	}

	manifest := readSetupFile(t, repo, "memory/.memento/manifest.json")
	if strings.Contains(manifest, `_memento/Using Memento.md`) {
		t.Fatalf("manifest = %q, want no _memento/Using Memento.md entry", manifest)
	}
}

func TestInitCreatesExampleNoteForGreenfieldVault(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "memory/example.md")
	for _, want := range []string{
		"title: Example memory note",
		"summary: A short example showing the frontmatter memento indexes.",
		"tags: [memento, example]",
		"mode: append-only",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("example.md = %q, want it to contain %q", got, want)
		}
	}

	manifest := readSetupFile(t, repo, "memory/.memento/manifest.json")
	if !strings.Contains(manifest, `"key": "example.md"`) {
		t.Fatalf("manifest = %q, want example.md entry", manifest)
	}
}

func TestInitDoesNotClobberExistingExampleWhenAdopting(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "memory/example.md", "# Existing example\n\nKeep this.\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	if got := readSetupFile(t, repo, "memory/example.md"); got != "# Existing example\n\nKeep this.\n" {
		t.Fatalf("example.md changed to %q", got)
	}
}

func TestInitDoesNotCreateExampleWhenAdoptingNonEmptyVault(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "memory/note.md", "# Existing note\n\nKeep this.\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	if _, err := os.Stat(filepath.Join(repo, "memory", "example.md")); !os.IsNotExist(err) {
		t.Fatalf("example.md stat err = %v, want file not to exist", err)
	}
}

func TestInitCanUseConfiguredAgentInstructionFile(t *testing.T) {
	repo := t.TempDir()

	if _, err := InitWithOptions(repo, "memory", InitOptions{AgentInstructionsPath: "CLAUDE.md"}); err != nil {
		t.Fatalf("InitWithOptions() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "CLAUDE.md")
	assertPointerBootloader(t, "CLAUDE.md", got, "memory")
	if _, err := os.Stat(filepath.Join(repo, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md stat err = %v, want file not to exist", err)
	}
}

func TestInitCreatesPreCommitHookWhenAbsent(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, ".git/hooks/pre-commit")
	if !strings.HasPrefix(got, "#!/bin/sh\nset -eu\n\n") {
		t.Fatalf("pre-commit hook = %q, want minimal shell header", got)
	}
	for _, want := range []string{
		"# memento:start",
		"if command -v memento >/dev/null 2>&1; then",
		"memento compile",
		`git add -- 'memory/.memento/manifest.json'`,
		"else",
		"echo 'warn: memento not on PATH; skipping vault compile' >&2",
		"fi",
		"# memento:end",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("pre-commit hook = %q, want it to contain %q", got, want)
		}
	}
	assertHookCommandsInsideMementoGuard(t, got)

	info, err := os.Stat(filepath.Join(repo, ".git/hooks/pre-commit"))
	if err != nil {
		t.Fatalf("stat pre-commit hook: %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("pre-commit hook mode = %v, want executable bit set", info.Mode().Perm())
	}
}

func TestInitAppendsMementoBlockToExistingPreCommitHook(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, ".git/hooks/pre-commit", "#!/bin/sh\nset -eu\n\necho existing\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, ".git/hooks/pre-commit")
	if !strings.HasPrefix(got, "#!/bin/sh\nset -eu\n\necho existing\n") {
		t.Fatalf("pre-commit hook = %q, want existing content preserved at start", got)
	}
	if count := strings.Count(got, "# memento:start"); count != 1 {
		t.Fatalf("pre-commit start sentinel count = %d, want 1; contents = %q", count, got)
	}
	if strings.Contains(got, "compile --dir") {
		t.Fatalf("pre-commit hook = %q, want supported compile command without --dir", got)
	}
	if !strings.Contains(got, `git add -- 'memory/.memento/manifest.json'`) {
		t.Fatalf("pre-commit hook = %q, want manifest staging command", got)
	}
	assertHookCommandsInsideMementoGuard(t, got)
}

func TestInitReplacesExistingMementoBlockInPreCommitHook(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, ".git/hooks/pre-commit", "#!/bin/sh\nset -eu\n\n# memento:start\nold block\n# memento:end\n\necho keep\n")

	if _, err := Init(repo, "project-memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, ".git/hooks/pre-commit")
	for _, want := range []string{
		"#!/bin/sh\nset -eu\n\n",
		"\n\necho keep\n",
		"if command -v memento >/dev/null 2>&1; then",
		"memento compile",
		`git add -- 'project-memory/.memento/manifest.json'`,
		"else",
		"echo 'warn: memento not on PATH; skipping vault compile' >&2",
		"fi",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("pre-commit hook = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "old block") {
		t.Fatalf("pre-commit hook = %q, want old memento block removed", got)
	}
	if count := strings.Count(got, "# memento:start"); count != 1 {
		t.Fatalf("pre-commit start sentinel count = %d, want 1; contents = %q", count, got)
	}
	assertHookCommandsInsideMementoGuard(t, got)
}

func TestInitPreCommitHookRunsCurrentCLICompileDuringCommit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pre-commit hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not found: %v", err)
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go not found: %v", err)
	}

	repo := t.TempDir()
	runSetupGit(t, repo, "init")
	binDir := t.TempDir()
	buildSetupMementoBinary(t, binDir)

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	writeSetupFile(t, repo, "memory/note.md", "# Note\n\nAdded after init.\n")
	runSetupGit(t, repo, "add", "AGENTS.md", ".gitignore", "memory")

	cmd := exec.Command(
		"git",
		"-c", "user.name=Memento Test",
		"-c", "user.email=memento@example.invalid",
		"commit", "--no-gpg-sign", "-m", "exercise memento hook",
	)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	manifest := runSetupGit(t, repo, "show", "HEAD:memory/.memento/manifest.json")
	if !strings.Contains(manifest, `"key": "note.md"`) {
		t.Fatalf("committed manifest = %s, want note.md entry", manifest)
	}
}

func TestPreCommitHookSoftSkipsWhenMementoIsAbsentFromPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pre-commit hook is a POSIX shell script")
	}

	repo := t.TempDir()
	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	cmd := exec.Command("/bin/sh", filepath.Join(repo, ".git/hooks/pre-commit"))
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("run pre-commit hook: %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("pre-commit stdout = %q, want empty", stdout.String())
	}
	wantStderr := "warn: memento not on PATH; skipping vault compile\n"
	if stderr.String() != wantStderr {
		t.Fatalf("pre-commit stderr = %q, want %q", stderr.String(), wantStderr)
	}
}

// TestPrepareCommitMsgHookLiftsUnlockTrailerAndClearsGrants exercises US7
// end-to-end: an unlock reopens a ratified read-only note's window (recording a
// grant), the next commit lifts that grant's justification into a Memento-Unlock
// trailer, and the grant sidecar is cleared — the grant deletion is what re-locks
// the read-only note.
func TestPrepareCommitMsgHookLiftsUnlockTrailerAndClearsGrants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("prepare-commit-msg hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not found: %v", err)
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go not found: %v", err)
	}

	repo := t.TempDir()
	runSetupGit(t, repo, "init")
	binDir := t.TempDir()
	buildSetupMementoBinary(t, binDir)
	pathEnv := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	// Ratify a read-only note so an unlock is the only way to reopen its window.
	writeSetupFile(t, repo, "memory/note.md", "---\nmode: read-only\n---\n\n# Note\n\nFrozen body.\n")
	runSetupGit(t, repo, "add", "AGENTS.md", ".gitignore", "memory")
	commitWithMemento(t, repo, pathEnv, "ratify note")

	// Unlock reopens the edit window and records the justification in the sidecar.
	runMementoInRepo(t, repo, filepath.Join(binDir, "memento"), pathEnv, "unlock", "note.md", "--justification", "fix a typo")
	grantsPath := filepath.Join(repo, "memory", ".memento", "unlock-grants.json")
	if _, err := os.Stat(grantsPath); err != nil {
		t.Fatalf("unlock-grants sidecar missing after unlock: %v", err)
	}

	// Any commit lifts the trailer and clears every grant.
	commitWithMemento(t, repo, pathEnv, "ordinary work")

	msg := runSetupGit(t, repo, "log", "-1", "--format=%B")
	if !strings.Contains(msg, "Memento-Unlock: note.md: fix a typo") {
		t.Fatalf("commit message = %q, want Memento-Unlock trailer", msg)
	}
	if _, err := os.Stat(grantsPath); !os.IsNotExist(err) {
		t.Fatalf("unlock-grants sidecar still present after commit (want cleared); stat err = %v", err)
	}
}

func TestPrepareCommitMsgHookLeavesMessageUntouchedWithoutGrants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("prepare-commit-msg hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not found: %v", err)
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go not found: %v", err)
	}

	repo := t.TempDir()
	runSetupGit(t, repo, "init")
	binDir := t.TempDir()
	buildSetupMementoBinary(t, binDir)
	pathEnv := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	runSetupGit(t, repo, "add", "AGENTS.md", ".gitignore", "memory")
	commitWithMemento(t, repo, pathEnv, "steady-state commit")

	msg := runSetupGit(t, repo, "log", "-1", "--format=%B")
	if strings.Contains(msg, "Memento-Unlock") {
		t.Fatalf("commit message = %q, want no Memento-Unlock trailer with no grants", msg)
	}
}

func commitWithMemento(t *testing.T, repo string, pathEnv []string, message string) {
	t.Helper()

	cmd := exec.Command(
		"git",
		"-c", "user.name=Memento Test",
		"-c", "user.email=memento@example.invalid",
		"commit", "--no-gpg-sign", "--allow-empty", "-m", message,
	)
	cmd.Dir = repo
	cmd.Env = pathEnv
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
}

func runMementoInRepo(t *testing.T, repo, mementoBin string, pathEnv []string, args ...string) {
	t.Helper()

	cmd := exec.Command(mementoBin, args...)
	cmd.Dir = repo
	cmd.Env = pathEnv
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("memento %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func assertHookCommandsInsideMementoGuard(t *testing.T, got string) {
	t.Helper()

	start := strings.Index(got, "if command -v memento >/dev/null 2>&1; then")
	compile := strings.Index(got, "\nmemento compile\n")
	add := strings.Index(got, "git add -- ")
	elseIdx := strings.Index(got, "\nelse\n")
	fi := strings.Index(got, "\nfi\n")

	if start == -1 || compile == -1 || add == -1 || elseIdx == -1 || fi == -1 {
		t.Fatalf("pre-commit hook = %q, want guarded compile/add block", got)
	}
	if !(start < compile && compile < add && add < elseIdx && elseIdx < fi) {
		t.Fatalf("pre-commit hook = %q, want compile and git add inside if-branch", got)
	}
}

func buildSetupMementoBinary(t *testing.T, binDir string) {
	t.Helper()

	repoRoot := setupRepoRoot(t)
	binary := filepath.Join(binDir, "memento")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/memento")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOCACHE="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build memento binary: %v\n%s", err, string(out))
	}
}

func setupRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat go.mod: %v", err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repository root from %s", dir)
		}
		dir = parent
	}
}

func runSetupGit(t *testing.T, repo string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func writeSetupFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %q: %v", relPath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", relPath, err)
	}
}

func readSetupFile(t *testing.T, root, relPath string) string {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", relPath, err)
	}
	return string(data)
}

func assertPointerBootloader(t *testing.T, relPath, got, memoryPath string) {
	t.Helper()

	for _, want := range []string{
		"Durable project knowledge lives in `" + memoryPath + "`: curated design decisions, specs, constraints, and discoveries, not task state.",
		"Before any other memento action, run `memento orient`.",
		"Run `memento brief` when you need the doc landscape; it is pull-only, not a mandatory second step.",
		"Use `memento read <key|@N>#<heading>` or `memento read <key|@N>` instead of grep/cat: it emits link-graph metadata on stderr and supports section extraction.",
		"`@N` indexes come from `brief`; `memento read` writes `binding: ratified|unratified` plus non-empty role-flattened link lines to stderr before stdout content.",
		"Discoveries that outlive a task belong in `" + memoryPath + "`, not the task store.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("%s = %q, want it to contain %q", relPath, got, want)
		}
	}

	for _, unwanted := range []string{
		"manifest.json",
		"scan the manifest",
		"agent-facing manifest projection",
		"Identify relevant entries",
		"read only the bodies",
		"Before anything else, run `memento orient` then `memento brief`.",
		"`brief` is intentionally dense",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("%s = %q, want no raw manifest guidance containing %q", relPath, got, unwanted)
		}
	}
	assertNoWritingGuideReference(t, relPath, got)
}

func assertNoWritingGuideReference(t *testing.T, relPath, got string) {
	t.Helper()

	for _, unwanted := range []string{"writing_guide.md", "writing.md"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("%s = %q, want no writing guide reference containing %q", relPath, got, unwanted)
		}
	}
}

func assertUsingMementoGuide(t *testing.T, got string) {
	t.Helper()

	for _, want := range []string{
		"# Using Memento",
		"`_memento/` is the human-readable tool namespace for this vault.",
		"`brief.md` is auto-regenerated from `.memento/manifest.json`",
		"`conventions/` holds operational guides",
		"`memento convention <name>`",
		"If you don't want this file, deleting it is fine",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("_memento/Using Memento.md = %q, want it to contain %q", got, want)
		}
	}
	for _, unwanted := range []string{
		"mode: read-only",
		"<!-- memento:readme:start -->",
		"<!-- memento:readme:end -->",
		"Tool-read files such as",
		"`review.md`",
		"`audit.md`",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("_memento/Using Memento.md = %q, want it not to contain %q", got, unwanted)
		}
	}
}

func hasSetupLine(contents, want string) bool {
	return countSetupLine(contents, want) > 0
}

func countSetupLine(contents, want string) int {
	count := 0
	for _, line := range strings.Split(contents, "\n") {
		if strings.TrimSpace(line) == want {
			count++
		}
	}
	return count
}

func readClaudeSettings(t *testing.T, repo string) map[string]any {
	t.Helper()

	var settings map[string]any
	if err := json.Unmarshal([]byte(readSetupFile(t, repo, ".claude/settings.json")), &settings); err != nil {
		t.Fatalf("unmarshal .claude/settings.json: %v", err)
	}
	return settings
}

func settingsObject(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", key, parent[key])
	}
	return value
}

func settingsArray(t *testing.T, parent map[string]any, key string) []any {
	t.Helper()

	value, ok := parent[key].([]any)
	if !ok {
		t.Fatalf("%s = %#v, want array", key, parent[key])
	}
	return value
}

// assertManagedHook checks that event holds a memento-managed hook entry whose
// matcher equals wantMatcher and whose nested command equals wantCommand.
func assertManagedHook(t *testing.T, hooks map[string]any, event, wantMatcher, wantCommand string) {
	t.Helper()

	for _, entry := range settingsArray(t, hooks, event) {
		object, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if object["matcher"] != wantMatcher {
			continue
		}
		if settingsJSONContainsCommand(object["hooks"], wantCommand) {
			return
		}
	}
	t.Fatalf("%s hooks = %#v, want entry with matcher %q and command %q", event, hooks[event], wantMatcher, wantCommand)
}

func settingsJSONContainsCommand(value any, command string) bool {
	return settingsJSONCommandCount(value, command) > 0
}

func settingsJSONCommandCount(value any, command string) int {
	switch typed := value.(type) {
	case []any:
		count := 0
		for _, item := range typed {
			count += settingsJSONCommandCount(item, command)
		}
		return count
	case map[string]any:
		count := 0
		if got, ok := typed["command"].(string); ok && got == command {
			count++
		}
		for _, item := range typed {
			count += settingsJSONCommandCount(item, command)
		}
		return count
	default:
		return 0
	}
}

func runClaudeOrientHook(t *testing.T, repo, binDir, logPath, extraEnv string) string {
	t.Helper()

	cmd := exec.Command(filepath.Join(repo, ".claude", "memento-orient-session-start.sh"))
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MEMENTO_TEST_LOG="+logPath,
	)
	if extraEnv != "" {
		cmd.Env = append(cmd.Env, extraEnv)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run Claude orient hook: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Claude orient hook stderr = %q, want empty", stderr.String())
	}
	return stdout.String()
}

func hookAdditionalContext(t *testing.T, stdout string) string {
	t.Helper()

	var payload struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal hook stdout %q: %v", stdout, err)
	}
	return payload.HookSpecificOutput.AdditionalContext
}
