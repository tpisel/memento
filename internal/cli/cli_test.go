package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(help) wrote stderr = %q, want empty", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"memento",
		"Usage:",
		"compile",
		"read",
		"version",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("Run(help) output %q does not contain %q", out, want)
		}
	}
}

func TestDefaultCommandShowsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(nil) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("Run(nil) output %q does not contain Usage", stdout.String())
	}
}

func TestVersionCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(version) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "memento dev" {
		t.Fatalf("Run(version) output = %q, want %q", got, "memento dev")
	}
}

func TestUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"bogus"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("Run(bogus) exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(bogus) wrote stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), `unknown command "bogus"`) {
		t.Fatalf("Run(bogus) stderr = %q, want unknown command message", stderr.String())
	}
}

func TestCompilePrintsManifestForExplicitDir(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"compile", "--dir", root, "--print"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(compile --print) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(compile --print) stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"key": "note.md"`) {
		t.Fatalf("Run(compile --print) stdout = %q, want note entry", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".memento", "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("Run(compile --print) wrote manifest unexpectedly; stat err = %v", err)
	}
}

func TestCompileWritesDiscoveredManifest(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "project-memory")
	if err := os.MkdirAll(filepath.Join(root, ".memento"), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"compile"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(compile) stdout = %q, want empty", stdout.String())
	}

	manifestPath := filepath.Join(root, ".memento", "manifest.json")
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(contents), `"key": "note.md"`) {
		t.Fatalf("manifest contents = %q, want note entry", string(contents))
	}
}

func TestReadPrintsRequestedMarkdownForExplicitDir(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "notes/deep.md", "# Deep\n\nNested content.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "--dir", root, "notes/deep.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read --dir notes/deep.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(read) stderr = %q, want empty", stderr.String())
	}

	want := "# Deep\n\nNested content.\n"
	if stdout.String() != want {
		t.Fatalf("Run(read) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadFailsClearlyForMissingOrIgnoredKey(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, ".mementoignore", "ignored.md\n")
	writeCLIFile(t, root, "ignored.md", "# Ignored\n")

	for _, key := range []string{"missing.md", "ignored.md"} {
		t.Run(key, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run([]string{"read", "--dir", root, key}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(read %s) exit code = %d, want 1", key, code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("Run(read %s) stdout = %q, want empty", key, stdout.String())
			}
			if !strings.Contains(stderr.String(), "not found") {
				t.Fatalf("Run(read %s) stderr = %q, want not found message", key, stderr.String())
			}
		})
	}
}

func TestReadRejectsTraversalKey(t *testing.T) {
	root := makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "--dir", root, "../outside.md"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read traversal) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read traversal) stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid key") {
		t.Fatalf("Run(read traversal) stderr = %q, want invalid key message", stderr.String())
	}
}

func makeCLIVault(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".memento"), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	return root
}

func writeCLIFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %q: %v", relPath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", relPath, err)
	}
}
