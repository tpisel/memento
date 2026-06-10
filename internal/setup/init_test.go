package setup

import (
	"os"
	"path/filepath"
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
		"Durable project knowledge lives in `sample-app-memory`.",
		"The manifest is at `sample-app-memory/.memento/manifest.json`.",
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
	if !strings.Contains(got, "The manifest is at `memory/.memento/manifest.json`.") {
		t.Fatalf("AGENTS.md = %q, want memory manifest path", got)
	}
}

func TestInitReplacesExistingBootloaderBlock(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, repo, "AGENTS.md", "# Rules\n\n<!-- memento:start -->\nold block\n<!-- memento:end -->\n\nKeep this too.\n")

	if _, err := Init(repo, "project-memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "AGENTS.md")
	for _, want := range []string{"# Rules\n\n", "\n\nKeep this too.\n", "project-memory/.memento/manifest.json"} {
		if !strings.Contains(got, want) {
			t.Fatalf("AGENTS.md = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "old block") {
		t.Fatalf("AGENTS.md = %q, want old bootloader removed", got)
	}
	if count := strings.Count(got, "<!-- memento:start -->"); count != 1 {
		t.Fatalf("AGENTS.md start sentinel count = %d, want 1; contents = %q", count, got)
	}
}

func TestInitBootloaderUsesCustomMemoryDirectoryPath(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "docs/project-memory"); err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "AGENTS.md")
	for _, want := range []string{
		"Durable project knowledge lives in `docs/project-memory`.",
		"The manifest is at `docs/project-memory/.memento/manifest.json`.",
		"Write back according to `docs/project-memory/writing_guide.md` once it exists.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("AGENTS.md = %q, want it to contain %q", got, want)
		}
	}
}

func TestInitBootloaderIsIdempotent(t *testing.T) {
	repo := t.TempDir()

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("first Init() error = %v, want nil", err)
	}
	first := readSetupFile(t, repo, "AGENTS.md")

	if _, err := Init(repo, "memory"); err != nil {
		t.Fatalf("second Init() error = %v, want nil", err)
	}
	second := readSetupFile(t, repo, "AGENTS.md")

	if second != first {
		t.Fatalf("AGENTS.md changed on rerun:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if count := strings.Count(second, "<!-- memento:start -->"); count != 1 {
		t.Fatalf("AGENTS.md start sentinel count = %d, want 1; contents = %q", count, second)
	}
}

func TestInitCanUseConfiguredAgentInstructionFile(t *testing.T) {
	repo := t.TempDir()

	if _, err := InitWithOptions(repo, "memory", InitOptions{AgentInstructionsPath: "CLAUDE.md"}); err != nil {
		t.Fatalf("InitWithOptions() error = %v, want nil", err)
	}

	got := readSetupFile(t, repo, "CLAUDE.md")
	if !strings.Contains(got, "The manifest is at `memory/.memento/manifest.json`.") {
		t.Fatalf("CLAUDE.md = %q, want memory manifest path", got)
	}
	if _, err := os.Stat(filepath.Join(repo, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md stat err = %v, want file not to exist", err)
	}
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
