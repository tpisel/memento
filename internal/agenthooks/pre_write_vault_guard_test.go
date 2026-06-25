package agenthooks_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPreWriteVaultGuardDeniesVaultWrite(t *testing.T) {
	repo := t.TempDir()
	vaultRoot := filepath.Join(repo, "memento-memory")
	mkdir(t, filepath.Join(vaultRoot, ".memento"))
	target := filepath.Join(vaultRoot, "spec.md")

	cmd := guardCommand(t, repo, `{"tool_name":"Write","tool_input":{"file_path":`+quoteJSON(target)+`}}`)
	cmd.Env = append(os.Environ(), "MEMENTO_VAULT_ROOT="+vaultRoot)

	stdout, stderr, err := runCommand(cmd)
	if err != nil {
		t.Fatalf("guard command error = %v; stderr = %s", err, stderr)
	}
	for _, want := range []string{
		`"hookEventName":"PreToolUse"`,
		`"permissionDecision":"deny"`,
		"memento write",
		"mode check",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want it to contain %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestPreWriteVaultGuardAllowsNonVaultWrite(t *testing.T) {
	repo := t.TempDir()
	vaultRoot := filepath.Join(repo, "memento-memory")
	mkdir(t, filepath.Join(vaultRoot, ".memento"))
	target := filepath.Join(repo, "README.md")

	cmd := guardCommand(t, repo, `{"tool_name":"Edit","tool_input":{"file_path":`+quoteJSON(target)+`}}`)
	cmd.Env = append(os.Environ(), "MEMENTO_VAULT_ROOT="+vaultRoot)

	stdout, stderr, err := runCommand(cmd)
	if err != nil {
		t.Fatalf("guard command error = %v; stderr = %s", err, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q; want both empty for allow", stdout, stderr)
	}
}

func TestPreWriteVaultGuardDiscoversVaultRoot(t *testing.T) {
	repo := t.TempDir()
	vaultRoot := filepath.Join(repo, "notes")
	mkdir(t, filepath.Join(vaultRoot, ".memento"))
	target := filepath.Join(vaultRoot, "Architecture decision record", "adr.md")

	cmd := guardCommand(t, repo, `{"tool_name":"MultiEdit","tool_input":{"file_path":`+quoteJSON(target)+`}}`)
	cmd.Env = append(os.Environ(), "MEMENTO_REPO_ROOT="+repo)

	stdout, stderr, err := runCommand(cmd)
	if err != nil {
		t.Fatalf("guard command error = %v; stderr = %s", err, stderr)
	}
	if !strings.Contains(stdout, `"permissionDecision":"deny"`) {
		t.Fatalf("stdout = %q, want deny decision", stdout)
	}
}

func guardCommand(t *testing.T, dir, stdin string) *exec.Cmd {
	t.Helper()

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	script := filepath.Join(repoRoot, "scripts", "agent-hooks", "pre-write-vault-guard.sh")

	cmd := exec.Command("bash", script)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	return cmd
}

func runCommand(cmd *exec.Cmd) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func quoteJSON(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
