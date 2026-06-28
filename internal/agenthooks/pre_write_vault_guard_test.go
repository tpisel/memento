package agenthooks_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests pin the PreToolUse wrapper's contract under ADR-0031: it is a dumb
// pipe to `memento check-write` that fails CLOSED. They do not re-test the mode
// lattice (that lives in internal/cli and internal/enforce); they assert only
// that check-write's verdict reaches the harness unchanged, that out-of-vault
// targets stay silent, and that an unrunnable check-write becomes a deny.

// mementoBin is the path to the check-write-capable memento binary built once for
// the whole package in TestMain.
var mementoBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "memento-agenthooks")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mktemp: %v\n", err)
		os.Exit(1)
	}
	bin := filepath.Join(dir, "memento")
	build := exec.Command("go", "build", "-o", bin, "./cmd/memento")
	build.Dir = repoRoot()
	if out, buildErr := build.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(os.Stderr, "build memento: %v\n%s", buildErr, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	mementoBin = bin
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// TestGuardPassesThroughDeny: a write check-write denies reaches the harness as a
// deny on stdout with a clean exit. Uses a Bash truncating redirect into the
// vault (bash_opaque_write) — a deny independent of git ratification.
func TestGuardPassesThroughDeny(t *testing.T) {
	repo := newVault(t)
	target := filepath.Join(repo, "spec.md")

	payload := bashPayload(t, "printf x > "+shellQuote(target))
	stdout, stderr, code := runGuard(t, repo, payload)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr = %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	assertDecision(t, stdout, "deny")
}

// TestGuardPassesThroughAllow: an allowed in-vault write (creating a new note)
// reaches the harness as an allow verdict on stdout.
func TestGuardPassesThroughAllow(t *testing.T) {
	repo := newVault(t)
	target := filepath.Join(repo, "fresh.md")
	content := "---\nmode: append-only\n---\n# Fresh\n\nBody.\n"

	payload := writePayload(t, target, content)
	stdout, stderr, code := runGuard(t, repo, payload)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr = %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	assertDecision(t, stdout, "allow")
}

// TestGuardSilentOutsideVault: a write to a target outside the vault produces no
// verdict and no output — normal permission flow governs the rest of the repo.
func TestGuardSilentOutsideVault(t *testing.T) {
	repo := newVault(t)
	outside := filepath.Join(t.TempDir(), "README.md")

	payload := writePayload(t, outside, "hello\n")
	stdout, stderr, code := runGuard(t, repo, payload)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr = %s", code, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q; want both empty for silent allow", stdout, stderr)
	}
}

// TestGuardFailsClosedOnMissingBinary is the US8 fail-closed self-test: with the
// check-write binary unreachable, an in-vault write is BLOCKED (deny, exit 2),
// not allowed to fall through. This is the "rename check-write => write blocked"
// guarantee that buys back the old verb's loud-failure property.
func TestGuardFailsClosedOnMissingBinary(t *testing.T) {
	repo := newVault(t)
	target := filepath.Join(repo, "spec.md")

	cmd := guardCommand(t, repo, writePayload(t, target, "anything\n"))
	cmd.Env = append(os.Environ(), "MEMENTO_BIN="+filepath.Join(t.TempDir(), "absent-memento"))

	stdout, stderr, code := runCommand(cmd)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (fail-closed block); stdout = %q stderr = %q", code, stdout, stderr)
	}
	assertDecision(t, stdout, "deny")
	if stderr == "" {
		t.Fatalf("stderr empty; want a fail-closed diagnostic for the harness")
	}
}

// TestGuardFailsClosedOnUnparseablePayload: a payload check-write cannot parse
// makes check-write exit non-zero; the wrapper must deny, not fall through.
func TestGuardFailsClosedOnUnparseablePayload(t *testing.T) {
	repo := newVault(t)

	stdout, stderr, code := runGuard(t, repo, "this is not json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (fail-closed block); stdout = %q stderr = %q", code, stdout, stderr)
	}
	assertDecision(t, stdout, "deny")
}

// --- helpers ---

// newVault creates a tempdir vault (a .memento marker at its root) and returns
// the root. The directory doubles as the working directory passed to the guard,
// so check-write discovers this single vault from its cwd.
func newVault(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".memento"))
	return repo
}

func writePayload(t *testing.T, path, content string) string {
	t.Helper()
	return marshalPayload(t, "Write", map[string]any{"file_path": path, "content": content})
}

func bashPayload(t *testing.T, command string) string {
	t.Helper()
	return marshalPayload(t, "Bash", map[string]any{"command": command})
}

func marshalPayload(t *testing.T, tool string, input map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"tool_name": tool, "tool_input": input})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(b)
}

// runGuard runs the wrapper with the built memento binary, the vault as its
// working directory, and the payload on stdin.
func runGuard(t *testing.T, workdir, stdin string) (string, string, int) {
	t.Helper()
	cmd := guardCommand(t, workdir, stdin)
	cmd.Env = append(os.Environ(), "MEMENTO_BIN="+mementoBin)
	return runCommand(cmd)
}

// assertDecision checks stdout carries the harness verdict envelope with the
// expected permissionDecision.
func assertDecision(t *testing.T, stdout, decision string) {
	t.Helper()
	for _, want := range []string{
		`"hookEventName":"PreToolUse"`,
		`"permissionDecision":"` + decision + `"`,
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want it to contain %q", stdout, want)
		}
	}
}

func guardCommand(t *testing.T, dir, stdin string) *exec.Cmd {
	t.Helper()
	script := filepath.Join(repoRoot(), "scripts", "agent-hooks", "pre-write-vault-guard.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	return cmd
}

func runCommand(cmd *exec.Cmd) (string, string, int) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	return stdout.String(), stderr.String(), cmd.ProcessState.ExitCode()
}

func repoRoot() string {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func shellQuote(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `'\''`) + `'`
}
