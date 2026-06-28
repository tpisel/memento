package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// checkBashPayload renders a raw PreToolUse JSON payload for a Bash command,
// mirroring the harness envelope check-write reads (command, not file_path).
func checkBashPayload(t *testing.T, command string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": command},
	})
	if err != nil {
		t.Fatalf("marshal Bash payload: %v", err)
	}
	return string(b)
}

// TestClassifyBash table-tests the pure command classifier against a committed
// append-only note in a vault whose root is the working directory (so bare
// operands like `log.md` resolve into it). The verdict's mode gate is exercised
// separately; here we assert only which bucket a command lands in.
func TestClassifyBash(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\n")
	writeCLIFile(t, root, "sub/nested.md", "---\nmode: append-only\n---\n# Nested\n")
	writeCLIFile(t, root, "notes.txt", "plain\n")
	commitCLIGit(t, root)

	v, err := resolveVault()
	if err != nil {
		t.Fatalf("resolveVault: %v", err)
	}

	cases := []struct {
		name string
		cmd  string
		want bashVerdict
		key  string // expected key when want == bashAppend
	}{
		// --- the one allowed shape ---
		{"plain append", "echo hi >> log.md", bashAppend, "log.md"},
		{"append no space", "echo hi>>log.md", bashAppend, "log.md"},
		{"append nested", "echo hi >> sub/nested.md", bashAppend, "sub/nested.md"},
		{"append quoted target", `echo hi >> "log.md"`, bashAppend, "log.md"},
		{"append with stderr to devnull", "echo hi >> log.md 2>/dev/null", bashAppend, "log.md"},
		{"append non-vault data redirect-free", "cat other.md >> log.md", bashAppend, "log.md"},

		// --- truncation / non-append redirections deny ---
		{"truncate redirect", "echo hi > log.md", bashOpaque, ""},
		{"append then truncate same file", "echo a >> log.md > log.md", bashOpaque, ""},
		{"append to two vault files", "echo a >> log.md >> sub/nested.md", bashOpaque, ""},

		// --- compounds: provably-append needs a single segment ---
		{"append in a pipeline", "echo hi | tee -a log.md", bashOpaque, ""},
		{"append after &&", "cd /tmp && echo hi >> log.md", bashOpaque, ""},
		{"append in subshell", "(echo hi) >> log.md", bashOpaque, ""},
		{"two commands semicolon", "echo a >> log.md ; echo b >> log.md", bashOpaque, ""},

		// --- known mutators touching the vault deny ---
		{"tee to vault", "echo hi | tee log.md", bashOpaque, ""},
		{"cp into vault", "cp /etc/hosts log.md", bashOpaque, ""},
		{"mv into vault", "mv /tmp/x log.md", bashOpaque, ""},
		{"sed in place", "sed -i s/a/b/ log.md", bashOpaque, ""},

		// --- non-vault writes are inert ---
		{"redirect outside vault", "echo hi >> /tmp/elsewhere.log", bashInert, ""},
		{"no redirection at all", "echo hi", bashInert, ""},
		{"read only redirect", "cat < log.md", bashInert, ""},

		// --- documented fail-open: targets the parser cannot see ---
		{"variable target", "echo hi >> $LOG", bashInert, ""},
		{"command-substitution target", "echo hi >> $(echo log.md)", bashInert, ""},
		{"interpreter indirection", `bash -c "echo hi >> log.md"`, bashInert, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, key := classifyBash(v, tc.cmd)
			if got != tc.want {
				t.Fatalf("classifyBash(%q) verdict = %v, want %v", tc.cmd, got, tc.want)
			}
			if tc.want == bashAppend && key != tc.key {
				t.Fatalf("classifyBash(%q) key = %q, want %q", tc.cmd, key, tc.key)
			}
		})
	}
}

// TestCheckWriteBashAppendModeGated exercises the end-to-end verdict for the
// allowed `>>` shape against each mode: append-only/living allow, read-only deny.
func TestCheckWriteBashAppendModeGated(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\n")
	writeCLIFile(t, root, "live.md", "---\nmode: living\n---\n# Live\n")
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)

	t.Run("append-only allows append", func(t *testing.T) {
		decision, _, _, _, stderr, code := invokeCheckWrite(t, checkBashPayload(t, "echo more >> log.md"))
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
		}
		if decision != "allow" {
			t.Fatalf("decision = %q, want allow", decision)
		}
	})

	t.Run("living allows append", func(t *testing.T) {
		decision, _, _, _, stderr, code := invokeCheckWrite(t, checkBashPayload(t, "echo more >> live.md"))
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
		}
		if decision != "allow" {
			t.Fatalf("decision = %q, want allow", decision)
		}
	})

	t.Run("read-only denies append", func(t *testing.T) {
		decision, reasonCode, reason, _, stderr, code := invokeCheckWrite(t, checkBashPayload(t, "echo more >> frozen.md"))
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
		}
		if decision != "deny" || reasonCode != "read_only" {
			t.Fatalf("verdict = (%q,%q), want (deny, read_only)", decision, reasonCode)
		}
		if !strings.Contains(reason, "frozen.md") {
			t.Fatalf("reason = %q, want it to name the note", reason)
		}
	})
}

func TestCheckWriteBashOpaqueDenied(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\n")
	commitCLIGit(t, root)

	decision, reasonCode, reason, _, stderr, code := invokeCheckWrite(t, checkBashPayload(t, "echo bad > log.md"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "bash_opaque_write" {
		t.Fatalf("verdict = (%q,%q), want (deny, bash_opaque_write)", decision, reasonCode)
	}
	for _, want := range []string{"denied", "denied again"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("reason = %q, want it to contain %q", reason, want)
		}
	}
}

func TestCheckWriteBashAppendToNewNoteAllowed(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	// fresh.md does not exist: `>>` creates it, which is legitimate creation.

	decision, _, _, _, stderr, code := invokeCheckWrite(t, checkBashPayload(t, "echo hi >> fresh.md"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "allow" {
		t.Fatalf("decision = %q, want allow (append creates a new note)", decision)
	}
}

func TestCheckWriteBashAppendUnwritablePathDenied(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)

	// A `>>` to an operational, non-note path is recognised as an append but is
	// not a writable note: it is denied as unwritable_path, like a file tool.
	decision, reasonCode, _, _, stderr, code := invokeCheckWrite(t, checkBashPayload(t, "echo hi >> _memento/x.md"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "unwritable_path" {
		t.Fatalf("verdict = (%q,%q), want (deny, unwritable_path)", decision, reasonCode)
	}
}

func TestCheckWriteBashOutsideVaultIsInert(t *testing.T) {
	makeCLIVault(t)

	_, _, _, stdout, stderr, code := invokeCheckWrite(t, checkBashPayload(t, "echo hi >> /tmp/elsewhere.log"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, want empty (writes outside the vault are not gated)", stdout)
	}
}

func TestCheckWriteBashEmptyCommandInert(t *testing.T) {
	makeCLIVault(t)

	_, _, _, stdout, stderr, code := invokeCheckWrite(t, checkBashPayload(t, ""))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, want empty for an empty command", stdout)
	}
}

func TestCheckWriteBashGrantReopensReadOnly(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)

	var ustdout, ustderr strings.Builder
	if c := Run([]string{"unlock", "frozen.md", "--justification", "fix typo"}, &ustdout, &ustderr); c != 0 {
		t.Fatalf("unlock exit code = %d, want 0; stderr = %q", c, ustderr.String())
	}

	decision, _, _, _, stderr, code := invokeCheckWrite(t, checkBashPayload(t, "echo more >> frozen.md"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "allow" {
		t.Fatalf("decision = %q, want allow under an active unlock grant", decision)
	}
}

// TestCheckWriteBashTildeTarget confirms ~ is expanded before resolution, so an
// append addressed through the home directory is still recognised. It is skipped
// unless the vault happens to live under $HOME.
func TestCheckWriteBashTildeTarget(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n")
	commitCLIGit(t, root)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	rel, err := filepath.Rel(home, filepath.Join(root, "log.md"))
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Skip("vault is not under $HOME; tilde expansion not exercisable here")
	}
	v, _ := resolveVault()
	got, key := classifyBash(v, "echo hi >> ~/"+filepath.ToSlash(rel))
	if got != bashAppend || key != "log.md" {
		t.Fatalf("classifyBash(tilde) = (%v,%q), want (bashAppend, log.md)", got, key)
	}
}
