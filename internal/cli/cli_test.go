package cli

import (
	"bytes"
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
