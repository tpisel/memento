package agenthooks_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/vault"
)

// These tests pin the PostToolUse wrapper's contract under ADR-0031: it runs
// `memento compile` (re-homing ADR-0022's auto-compile) and surfaces the drift
// alarm. They do not re-test compile's manifest output (that lives in
// internal/cli / internal/manifest); they assert only that the hook recompiles
// on a vault write and turns a DRIFT ALARM into the exit-2 the harness feeds
// back to the agent.

// TestPostHookRecompilesVaultWrite: the hook runs compile, which writes the
// manifest, and exits 0 cleanly when there is nothing to alarm about.
func TestPostHookRecompilesVaultWrite(t *testing.T) {
	repo := newVault(t)
	if err := os.WriteFile(filepath.Join(repo, "note.md"),
		[]byte("---\nmode: append-only\n---\n# Note\n\nBody.\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	stdout, stderr, code := runPostHook(t, repo)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stdout = %q stderr = %q", code, stdout, stderr)
	}
	if strings.Contains(stderr, "DRIFT ALARM") {
		t.Fatalf("stderr = %q, want no drift alarm for a coherent write", stderr)
	}
	if _, err := os.Stat(filepath.Join(repo, ".memento", "manifest.json")); err != nil {
		t.Fatalf("manifest not written by post-write compile: %v", err)
	}
}

// TestPostHookSurfacesDriftAsExit2: a recorded expectation that disagrees with
// disk makes compile shout DRIFT ALARM; the wrapper turns that into exit 2 so
// the alarm reaches the agent (PostToolUse exit 2 feeds stderr back).
func TestPostHookSurfacesDriftAsExit2(t *testing.T) {
	repo := newVault(t)
	if err := os.WriteFile(filepath.Join(repo, "note.md"),
		[]byte("---\nmode: append-only\n---\n# Note\n\nLanded.\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	v := vault.Vault{Root: repo, MarkerDir: filepath.Join(repo, vault.MarkerDirName)}
	if err := enforce.RecordPending(v, "note.md", enforce.HashBytes([]byte("not-what-landed"))); err != nil {
		t.Fatalf("record pending: %v", err)
	}

	stdout, stderr, code := runPostHook(t, repo)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (drift surfaced); stdout = %q stderr = %q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "DRIFT ALARM") || !strings.Contains(stderr, "note.md") {
		t.Fatalf("stderr = %q, want a DRIFT ALARM naming note.md", stderr)
	}
}

// runPostHook runs the post-write compile wrapper with the built memento binary,
// the vault as its working directory, and an empty stdin (the hook ignores the
// PostToolUse payload — it always recompiles).
func runPostHook(t *testing.T, workdir string) (string, string, int) {
	t.Helper()
	script := filepath.Join(repoRoot(), "scripts", "agent-hooks", "post-write-compile.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = workdir
	cmd.Stdin = strings.NewReader("")
	cmd.Env = append(os.Environ(), "MEMENTO_BIN="+mementoBin)
	return runCommand(cmd)
}
