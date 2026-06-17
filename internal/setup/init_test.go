package setup

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
		"Before anything else, run `memento orient` then `memento brief`.",
		"`brief` is intentionally dense; no need to pipe it through `head`.",
		"Use `memento read <key|@N>#<heading>` or `memento read <key|@N>` instead of grep/cat: it emits link-graph metadata on stderr and supports section extraction.",
		"`@N` indexes come from `brief`; `memento read` writes `binding: ratified|unratified` plus non-empty role-flattened link lines to stderr before stdout content.",
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
		"Before anything else, run `memento orient` then `memento brief`.",
		"`brief` is intentionally dense; no need to pipe it through `head`.",
		"Use `memento read <key|@N>#<heading>` or `memento read <key|@N>` instead of grep/cat: it emits link-graph metadata on stderr and supports section extraction.",
		"`@N` indexes come from `brief`; `memento read` writes `binding: ratified|unratified` plus non-empty role-flattened link lines to stderr before stdout content.",
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

	mementoignore := readSetupFile(t, repo, "memory/.mementoignore")
	if !hasSetupLine(mementoignore, "_memento/brief.md") {
		t.Fatalf(".mementoignore = %q, want file-specific brief ignore entry", mementoignore)
	}
	if !hasSetupLine(mementoignore, "_memento/Using Memento.md") {
		t.Fatalf(".mementoignore = %q, want file-specific Using Memento guide ignore entry", mementoignore)
	}
	if hasSetupLine(mementoignore, "_memento/") || hasSetupLine(mementoignore, "/_memento/") {
		t.Fatalf(".mementoignore = %q, want no folder-wide _memento ignore entry", mementoignore)
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
	if !hasSetupLine(mementoignore, "_memento/brief.md") {
		t.Fatalf(".mementoignore = %q, want file-specific brief ignore entry", mementoignore)
	}
	if !hasSetupLine(mementoignore, "_memento/Using Memento.md") {
		t.Fatalf(".mementoignore = %q, want file-specific Using Memento guide ignore entry", mementoignore)
	}
	if hasSetupLine(mementoignore, "_memento/") || hasSetupLine(mementoignore, "/_memento/") {
		t.Fatalf(".mementoignore = %q, want no folder-wide _memento ignore entry", mementoignore)
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
	writeSetupFile(t, repo, "memory/.mementoignore", "drafts/\n\n_memento/brief.md\n_memento/Using Memento.md\n")

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
	if count := countSetupLine(secondMementoignore, "_memento/brief.md"); count != 1 {
		t.Fatalf(".mementoignore brief entry count = %d, want 1; contents = %q", count, secondMementoignore)
	}
	if count := countSetupLine(secondMementoignore, "_memento/Using Memento.md"); count != 1 {
		t.Fatalf(".mementoignore Using Memento guide entry count = %d, want 1; contents = %q", count, secondMementoignore)
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

func TestInitCreatesWritingGuideForGreenfieldVault(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "memory/_memento/writing.md")
	for _, want := range []string{
		"title:",
		"mode: read-only",
		"summary:",
		"hard-won learnings",
		"paths we decided not to take",
		"constraints that aren't visible in code",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("_memento/writing.md = %q, want it to contain %q", got, want)
		}
	}
	_, body, ok := strings.Cut(got, "---\n\n")
	if !ok || strings.TrimSpace(body) == "" {
		t.Fatalf("_memento/writing.md = %q, want non-empty body", got)
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
	if !hasSetupLine(mementoignore, "_memento/Using Memento.md") {
		t.Fatalf(".mementoignore = %q, want Using Memento guide ignore entry", mementoignore)
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
		"Before anything else, run `memento orient` then `memento brief`.",
		"`brief` is intentionally dense; no need to pipe it through `head`.",
		"Use `memento read <key|@N>#<heading>` or `memento read <key|@N>` instead of grep/cat: it emits link-graph metadata on stderr and supports section extraction.",
		"`@N` indexes come from `brief`; `memento read` writes `binding: ratified|unratified` plus non-empty role-flattened link lines to stderr before stdout content.",
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
		"Tool-read files such as `writing.md`, `review.md`, and `audit.md`",
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
