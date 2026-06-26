package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteModeHelp(t *testing.T) {
	for _, helpFlag := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"write-mode", helpFlag}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("Run(write-mode %s) exit code = %d, want 0; stderr = %q", helpFlag, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(write-mode %s) stderr = %q, want empty", helpFlag, stderr.String())
		}
		for _, want := range []string{
			"memento write-mode <key> <append-only|living|read-only> [--justification <reason>]",
			"only path that changes an existing note's mode",
			"Loosening toward living requires --justification",
			"rejected, not defaulted",
		} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("Run(write-mode %s) stdout =\n%s\nwant %q", helpFlag, stdout.String(), want)
			}
		}
	}
}

func TestTopLevelHelpListsWriteMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(help) exit code = %d, want 0", code)
	}
	for _, want := range []string{
		"memento write-mode <key> <append-only|living|read-only> [--justification <reason>]",
		"write-mode  Durably change a note's frontmatter mode",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("Run(help) stdout =\n%s\nwant %q", stdout.String(), want)
		}
	}
}

func TestWriteModeSetsModeAndRecompiles(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "---\ntitle: Note\nmode: append-only\n---\n# Note\n\nBody.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"write-mode", "note.md", "read-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(write-mode read-only) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write-mode) stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{"mode: note.md append-only -> read-only", "compiled: 1 entries"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(write-mode) stderr = %q, want %q", stderr.String(), want)
		}
	}

	got := readCLIFile(t, root, "note.md")
	if want := "---\ntitle: Note\nmode: read-only\n---\n# Note\n\nBody.\n"; got != want {
		t.Fatalf("note after write-mode =\n%q\nwant\n%q", got, want)
	}
	if m := readCLIFile(t, root, ".memento/manifest.json"); !strings.Contains(m, `"mode": "read-only"`) {
		t.Fatalf("manifest = %q, want mode read-only for note.md", m)
	}
}

func TestWriteModeRejectsUnknownMode(t *testing.T) {
	root := makeCLIVault(t)
	source := "---\nmode: append-only\n---\n# Note\n\nBody.\n"
	writeCLIFile(t, root, "note.md", source)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"write-mode", "note.md", "frozen"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("Run(write-mode frozen) exit code = %d, want 2; stderr = %q", code, stderr.String())
	}
	assertCLIErrorToken(t, stderr.String(), "write-mode", "invalid-arguments")
	if got := readCLIFile(t, root, "note.md"); got != source {
		t.Fatalf("note mutated on rejected mode:\ngot %q\nwant %q", got, source)
	}
}

func TestWriteModeLooseningRequiresJustification(t *testing.T) {
	root := makeCLIVault(t)
	source := "---\nmode: read-only\n---\n# Note\n\nBody.\n"
	writeCLIFile(t, root, "note.md", source)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"write-mode", "note.md", "living"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("Run(write-mode living) without justification exit code = %d, want 2; stderr = %q", code, stderr.String())
	}
	assertCLIErrorToken(t, stderr.String(), "write-mode", "invalid-arguments")
	if !strings.Contains(stderr.String(), "requires --justification") {
		t.Fatalf("Run(write-mode living) stderr = %q, want justification hint", stderr.String())
	}
	if got := readCLIFile(t, root, "note.md"); got != source {
		t.Fatalf("note mutated on rejected loosening:\ngot %q\nwant %q", got, source)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"write-mode", "note.md", "living", "--justification", "promoted to working scratch"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(write-mode living --justification) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"mode: note.md read-only -> living", "justification: promoted to working scratch"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(write-mode living --justification) stderr = %q, want %q", stderr.String(), want)
		}
	}
	if got := readCLIFile(t, root, "note.md"); !strings.Contains(got, "mode: living") {
		t.Fatalf("note after loosening = %q, want mode living", got)
	}
}

func TestWriteModeTighteningAcceptsOptionalJustification(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "---\nmode: living\n---\n# Note\n\nBody.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"write-mode", "note.md", "read-only", "--justification", "freezing the decision"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(write-mode read-only --justification) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got := readCLIFile(t, root, "note.md"); !strings.Contains(got, "mode: read-only") {
		t.Fatalf("note after tightening = %q, want mode read-only", got)
	}

	// Tightening without --justification is allowed too.
	writeCLIFile(t, root, "other.md", "---\nmode: append-only\n---\n# Other\n\nBody.\n")
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"write-mode", "other.md", "read-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(write-mode other read-only) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got := readCLIFile(t, root, "other.md"); !strings.Contains(got, "mode: read-only") {
		t.Fatalf("other after tightening = %q, want mode read-only", got)
	}
}

func TestWriteModeRejectsMissingNote(t *testing.T) {
	makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"write-mode", "absent.md", "read-only"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(write-mode absent) exit code = %d, want 1; stderr = %q", code, stderr.String())
	}
	assertCLIErrorToken(t, stderr.String(), "write-mode", "key-not-found")
}

func TestWriteModeRequiresKeyAndMode(t *testing.T) {
	makeCLIVault(t)

	for _, args := range [][]string{
		{"write-mode"},
		{"write-mode", "note.md"},
		{"write-mode", "note.md", "read-only", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		code := Run(args, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("Run(%v) exit code = %d, want 2; stderr = %q", args, code, stderr.String())
		}
		assertCLIErrorToken(t, stderr.String(), "write-mode", "invalid-arguments")
	}
}
