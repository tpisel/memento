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

func TestInitInstallsClaudeWriteSkillFromVaultSource(t *testing.T) {
	repo := t.TempDir()
	source := "---\nname: memento-write\n---\n# Custom write skill\n\nRead the local guide first.\n"
	writeSetupFile(t, repo, "memory/_memento/skills/write.md", source)
	writeSetupFile(t, repo, ".claude/skills/other/SKILL.md", "# Other skill\n")
	writeSetupFile(t, repo, ".claude/skills/memento-write/SKILL.md", "# Old generated skill\n")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, ".claude/skills/memento-write/SKILL.md")
	if got != source {
		t.Fatalf("installed write skill = %q, want vault source %q", got, source)
	}
	if other := readSetupFile(t, repo, ".claude/skills/other/SKILL.md"); other != "# Other skill\n" {
		t.Fatalf("unrelated skill changed to %q", other)
	}
}

func TestInitScaffoldsDefaultWriteSkillSource(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	source := readSetupFile(t, repo, "memory/_memento/skills/write.md")
	installed := readSetupFile(t, repo, ".claude/skills/memento-write/SKILL.md")
	if source != installed {
		t.Fatalf("installed write skill differs from source\nsource:\n%s\ninstalled:\n%s", source, installed)
	}
	for _, want := range []string{
		"name: memento-write",
		"Before authoring a vault write:",
		"memento convention writing",
		"_memento/conventions/writing.md",
		"memento write",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("_memento/skills/write.md = %q, want it to contain %q", source, want)
		}
	}
	for _, unwanted := range []string{
		"memento read _memento/writing",
		"`_memento/writing.md`",
	} {
		if strings.Contains(source, unwanted) {
			t.Fatalf("_memento/skills/write.md = %q, want no legacy writing-guide reference %q", source, unwanted)
		}
	}
	manifest := readSetupFile(t, repo, "memory/.memento/manifest.json")
	if strings.Contains(manifest, `_memento/skills/write.md`) {
		t.Fatalf("manifest = %q, want write skill source excluded from the normal manifest", manifest)
	}
}

func TestInitWriteSkillInstallIsIdempotent(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("first Init() error = %v, want nil", err)
	}
	firstSource := readSetupFile(t, repo, "memory/_memento/skills/write.md")
	firstInstalled := readSetupFile(t, repo, ".claude/skills/memento-write/SKILL.md")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("second Init() error = %v, want nil", err)
	}
	secondSource := readSetupFile(t, repo, "memory/_memento/skills/write.md")
	secondInstalled := readSetupFile(t, repo, ".claude/skills/memento-write/SKILL.md")

	if secondSource != firstSource {
		t.Fatalf("_memento/skills/write.md changed on rerun:\nfirst:\n%s\nsecond:\n%s", firstSource, secondSource)
	}
	if secondInstalled != firstInstalled {
		t.Fatalf(".claude/skills/memento-write/SKILL.md changed on rerun:\nfirst:\n%s\nsecond:\n%s", firstInstalled, secondInstalled)
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
