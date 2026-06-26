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

	tests := []struct {
		name     string
		toolName string
		path     string
	}{
		{
			name:     "write",
			toolName: "Write",
			path:     filepath.Join(vaultRoot, "spec.md"),
		},
		{
			name:     "edit",
			toolName: "Edit",
			path:     filepath.Join(vaultRoot, "Architecture decision record", "adr.md"),
		},
		{
			name:     "multiedit",
			toolName: "MultiEdit",
			path:     filepath.Join(vaultRoot, "_memento", "writing.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := `{"tool_name":` + quoteJSON(tt.toolName) + `,"tool_input":{"file_path":` + quoteJSON(tt.path) + `}}`
			stdout, stderr, err := runGuard(t, repo, vaultRoot, payload)
			if err != nil {
				t.Fatalf("guard command error = %v; stderr = %s", err, stderr)
			}
			assertDenied(t, stdout)
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestPreWriteVaultGuardAllowsNonVaultWrite(t *testing.T) {
	repo := t.TempDir()
	vaultRoot := filepath.Join(repo, "memento-memory")
	mkdir(t, filepath.Join(vaultRoot, ".memento"))
	target := filepath.Join(repo, "README.md")

	stdout, stderr, err := runGuard(t, repo, vaultRoot, `{"tool_name":"Edit","tool_input":{"file_path":`+quoteJSON(target)+`}}`)
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

func TestPreWriteVaultGuardDeniesObviousBashWritesIntoVault(t *testing.T) {
	repo := t.TempDir()
	vaultRoot := filepath.Join(repo, "notes")
	mkdir(t, filepath.Join(vaultRoot, ".memento"))

	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "redirect truncate",
			command: "printf hi > " + shellQuote(filepath.Join(vaultRoot, "direct.md")),
		},
		{
			name:    "redirect append relative",
			command: "printf hi >> notes/direct.md",
		},
		{
			name:    "tee",
			command: "printf hi | tee " + shellQuote(filepath.Join(vaultRoot, "tee.md")),
		},
		{
			name:    "cp",
			command: "cp README.md " + shellQuote(filepath.Join(vaultRoot, "copied.md")),
		},
		{
			name:    "mv",
			command: "mv scratch.md " + shellQuote(filepath.Join(vaultRoot, "moved.md")),
		},
		{
			name:    "sed in place",
			command: "sed -i '' 's/a/b/' " + shellQuote(filepath.Join(vaultRoot, "spec.md")),
		},
		{
			name:    "perl in place",
			command: "perl -pi -e 's/a/b/' " + shellQuote(filepath.Join(vaultRoot, "spec.md")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := `{"tool_name":"Bash","tool_input":{"command":` + quoteJSON(tt.command) + `}}`
			stdout, stderr, err := runGuard(t, repo, vaultRoot, payload)
			if err != nil {
				t.Fatalf("guard command error = %v; stderr = %s", err, stderr)
			}
			assertDenied(t, stdout)
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestPreWriteVaultGuardAllowsBashOutsideVaultAndMementoCommands(t *testing.T) {
	repo := t.TempDir()
	vaultRoot := filepath.Join(repo, "notes")
	mkdir(t, filepath.Join(vaultRoot, ".memento"))

	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "redirect outside vault",
			command: "printf hi > README.md",
		},
		{
			name:    "cp outside vault",
			command: "cp notes/spec.md README.md",
		},
		{
			name:    "memento read",
			command: "go run ./cmd/memento read spec.md",
		},
		{
			name:    "memento brief",
			command: "memento brief",
		},
		{
			name:    "memento orient",
			command: "memento orient",
		},
		{
			name:    "memento compile",
			command: "go run ./cmd/memento compile",
		},
		{
			name:    "memento write",
			command: "memento write discoveries/example.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := `{"tool_name":"Bash","tool_input":{"command":` + quoteJSON(tt.command) + `}}`
			stdout, stderr, err := runGuard(t, repo, vaultRoot, payload)
			if err != nil {
				t.Fatalf("guard command error = %v; stderr = %s", err, stderr)
			}
			if stdout != "" || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q; want both empty for allow", stdout, stderr)
			}
		})
	}
}

func runGuard(t *testing.T, dir, vaultRoot, stdin string) (string, string, error) {
	t.Helper()
	cmd := guardCommand(t, dir, stdin)
	cmd.Env = append(os.Environ(), "MEMENTO_VAULT_ROOT="+vaultRoot)
	return runCommand(cmd)
}

func assertDenied(t *testing.T, stdout string) {
	t.Helper()
	for _, want := range []string{
		`"hookEventName":"PreToolUse"`,
		`"permissionDecision":"deny"`,
		"memento write",
		"protected note",
		"--force-with-reason",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want it to contain %q", stdout, want)
		}
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

func shellQuote(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `'\''`) + `'`
}
