package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestUnlockHelp(t *testing.T) {
	for _, helpFlag := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"unlock", helpFlag}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("Run(unlock %s) exit code = %d, want 0; stderr = %q", helpFlag, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(unlock %s) stderr = %q, want empty", helpFlag, stderr.String())
		}
		for _, want := range []string{
			"memento unlock <key> --justification <reason>",
			"until the next commit",
			"not persisted past the grant",
		} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("Run(unlock %s) stdout =\n%s\nwant %q", helpFlag, stdout.String(), want)
			}
		}
	}
}

func TestTopLevelHelpListsUnlock(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(help) exit code = %d, want 0", code)
	}
	for _, want := range []string{
		"memento unlock <key> --justification <reason>",
		"unlock      Temporarily re-open",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("Run(help) stdout =\n%s\nwant %q", stdout.String(), want)
		}
	}
}

func TestUnlockWritesGrantSidecar(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "---\nmode: read-only\n---\n# Note\n\nBody.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"unlock", "note.md", "--justification", "fixing a broken link"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(unlock) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(unlock) stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{"unlocked: note.md until next commit", "justification: fixing a broken link"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(unlock) stderr = %q, want %q", stderr.String(), want)
		}
	}

	raw := readCLIFile(t, root, ".memento/unlock-grants.json")
	var grants map[string]struct {
		Justification string `json:"justification"`
		GrantedAt     string `json:"granted_at"`
	}
	if err := json.Unmarshal([]byte(raw), &grants); err != nil {
		t.Fatalf("unlock-grants.json = %q, not valid JSON: %v", raw, err)
	}
	g, ok := grants["note.md"]
	if !ok {
		t.Fatalf("unlock-grants.json = %q, want a grant for note.md", raw)
	}
	if g.Justification != "fixing a broken link" {
		t.Fatalf("grant justification = %q, want the supplied reason", g.Justification)
	}
	if g.GrantedAt == "" {
		t.Fatalf("grant granted_at = %q, want a timestamp", g.GrantedAt)
	}
}

func TestUnlockRequiresJustification(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "---\nmode: read-only\n---\n# Note\n\nBody.\n")

	for _, args := range [][]string{
		{"unlock", "note.md"},
		{"unlock", "note.md", "--justification", "   "},
	} {
		var stdout, stderr bytes.Buffer
		code := Run(args, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("Run(%v) exit code = %d, want 2; stderr = %q", args, code, stderr.String())
		}
		assertCLIErrorToken(t, stderr.String(), "unlock", "invalid-arguments")
		if hasCLIFile(t, root, ".memento/unlock-grants.json") {
			t.Fatalf("Run(%v) wrote a grant sidecar despite invalid arguments", args)
		}
	}
}

func TestUnlockRejectsMissingNote(t *testing.T) {
	root := makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"unlock", "absent.md", "--justification", "reason"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(unlock absent) exit code = %d, want 1; stderr = %q", code, stderr.String())
	}
	assertCLIErrorToken(t, stderr.String(), "unlock", "key-not-found")
	if hasCLIFile(t, root, ".memento/unlock-grants.json") {
		t.Fatalf("Run(unlock absent) wrote a grant sidecar for a missing note")
	}
}

func TestUnlockRequiresKey(t *testing.T) {
	makeCLIVault(t)

	for _, args := range [][]string{
		{"unlock"},
		{"unlock", "--justification", "reason"},
		{"unlock", "a.md", "b.md", "--justification", "reason"},
	} {
		var stdout, stderr bytes.Buffer
		code := Run(args, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("Run(%v) exit code = %d, want 2; stderr = %q", args, code, stderr.String())
		}
		assertCLIErrorToken(t, stderr.String(), "unlock", "invalid-arguments")
	}
}
