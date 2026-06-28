package acceptance

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

// mementoBin is the path to the real memento binary, built once for the whole
// suite in TestMain. Its directory is named so the binary is reachable as
// `memento` on PATH, which the installed git hooks (`command -v memento`) need.
var mementoBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "memento-acceptance")
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

// ---------------------------------------------------------------------------
// US1: a ratified read-only note is denied a native edit.
// ADR-0031 §237 ("read-only ratified note: native edit denied").
// ---------------------------------------------------------------------------

func TestUS1_ReadOnlyRatifiedDenied(t *testing.T) {
	f := newFixture(t)
	f.writeNote("frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	f.commit("ratify frozen")

	v := f.preToolUse(f.writePayload("frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nRewritten.\n"))

	v.requireDeny(t, "read_only")
	v.requireReasonContains(t, "frozen.md", "denied again", "memento unlock")
}

// ---------------------------------------------------------------------------
// US2: operator matrix — a truncating Write to an append-only note is denied,
// a prefix-preserving Write (the `>>` shape) is allowed.
// ADR-0031 §238.
// ---------------------------------------------------------------------------

func TestUS2_AppendOnlyOperatorMatrix(t *testing.T) {
	f := newFixture(t)
	const old = "---\nmode: append-only\n---\n# Log\n\nEntry one.\n"
	f.writeNote("log.md", old)
	f.commit("ratify log")

	t.Run("truncating write denied", func(t *testing.T) {
		v := f.preToolUse(f.writePayload("log.md", "---\nmode: append-only\n---\n# Log\n"))
		v.requireDeny(t, "append_only_overwrite")
	})

	t.Run("prefix-preserving write allowed", func(t *testing.T) {
		v := f.preToolUse(f.writePayload("log.md", old+"Entry two.\n"))
		v.requireAllow(t)
	})
}

// ---------------------------------------------------------------------------
// US3: an interior Edit of an append-only note is denied; a tail-append Edit
// that keeps the old bytes as a prefix is allowed.
// ADR-0031 §239.
// ---------------------------------------------------------------------------

func TestUS3_AppendOnlyInteriorVsTail(t *testing.T) {
	f := newFixture(t)
	f.writeNote("log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\nEntry two.\n")
	f.commit("ratify log")

	t.Run("interior edit denied", func(t *testing.T) {
		v := f.preToolUse(f.editPayload("log.md", "Entry one.", "Edited one.", false))
		v.requireDeny(t, "append_only_interior")
		v.requireReasonContains(t, "log.md")
	})

	t.Run("tail-append edit allowed", func(t *testing.T) {
		v := f.preToolUse(f.editPayload("log.md", "Entry two.\n", "Entry two.\nEntry three.\n", false))
		v.requireAllow(t)
	})
}

// ---------------------------------------------------------------------------
// US4: a body-write may not smuggle a permanent mode: change under an active
// unlock grant — the drive-by mode-change is denied even though the body is
// reopened. A body-only edit under the same grant is allowed.
// ADR-0031 §240.
// ---------------------------------------------------------------------------

func TestUS4_DriveByModeChangeDeniedUnderGrant(t *testing.T) {
	f := newFixture(t)
	f.writeNote("frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	f.commit("ratify frozen")
	f.mustMemento("unlock", "frozen.md", "--justification", "fix typo")

	t.Run("mode flip denied", func(t *testing.T) {
		v := f.preToolUse(f.writePayload("frozen.md", "---\nmode: living\n---\n# Frozen\n\nFixed.\n"))
		v.requireDeny(t, "drive_by_mode_change")
		v.requireReasonContains(t, "frozen.md", "write-mode")
	})

	t.Run("body-only edit under same grant allowed", func(t *testing.T) {
		v := f.preToolUse(f.writePayload("frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nFixed.\n"))
		v.requireAllow(t)
	})
}

// ---------------------------------------------------------------------------
// US5: a brand-new note (no old bytes) is created freely, including the mode it
// declares. ADR-0031 (carve-out: legitimate authoring is not drive-by).
// ---------------------------------------------------------------------------

func TestUS5_NewNoteAllowed(t *testing.T) {
	f := newFixture(t)
	// No commit: the target does not exist, so this is creation.
	v := f.preToolUse(f.writePayload("fresh.md", "---\nmode: read-only\n---\n# Fresh\n\nA brand-new frozen record.\n"))
	v.requireAllow(t)
}

// ---------------------------------------------------------------------------
// US6: an unratified (written but never committed) note accepts any write — the
// read-only/append-only bite begins only after ratification (ADR-0017/0031).
// ---------------------------------------------------------------------------

func TestUS6_UnratifiedNoteAllowed(t *testing.T) {
	f := newFixture(t)
	// Written but never committed: still inside its edit window.
	f.writeNote("draft.md", "---\nmode: read-only\n---\n# Draft\n\nFirst.\n")

	v := f.preToolUse(f.writePayload("draft.md", "---\nmode: read-only\n---\n# Draft\n\nReworked.\n"))
	v.requireAllow(t)
}

// ---------------------------------------------------------------------------
// US7: unlock + relock. A ratified read-only note is locked; `memento unlock`
// reopens its window; any commit clears the grant and the note re-locks. The
// justification is held only for the grant's lifetime — no commit trailer
// (ADR-0031, 2026-06-28 addendum retires the Memento-Unlock trailer).
// ---------------------------------------------------------------------------

func TestUS7_UnlockRelock(t *testing.T) {
	f := newFixture(t)
	f.writeNote("note.md", "---\nmode: read-only\n---\n# Note\n\nFrozen body.\n")
	f.commit("ratify note")

	// Locked: an edit to the ratified read-only note is denied before unlock.
	locked := f.preToolUse(f.writePayload("note.md", "---\nmode: read-only\n---\n# Note\n\nEdited body.\n"))
	locked.requireDeny(t, "read_only")

	// Unlock reopens the window and records the grant sidecar.
	f.mustMemento("unlock", "note.md", "--justification", "fix a typo")
	grantsPath := filepath.Join(f.vault, ".memento", "unlock-grants.json")
	if _, err := os.Stat(grantsPath); err != nil {
		t.Fatalf("unlock-grants sidecar missing after unlock: %v", err)
	}

	// Under the grant the same edit is now allowed.
	reopened := f.preToolUse(f.writePayload("note.md", "---\nmode: read-only\n---\n# Note\n\nEdited body.\n"))
	reopened.requireAllow(t)

	// Any commit clears every grant (the re-lock); the justification is NOT lifted
	// into a commit trailer — that mechanism is retired (ADR-0031 2026-06-28 addendum).
	f.commitAllowEmpty("ordinary work")
	msg := f.git("log", "-1", "--format=%B")
	if strings.Contains(msg, "Memento-Unlock") {
		t.Fatalf("commit message = %q, want NO Memento-Unlock trailer (retired)", msg)
	}
	if _, err := os.Stat(grantsPath); !os.IsNotExist(err) {
		t.Fatalf("unlock-grants sidecar still present after commit (want cleared); stat err = %v", err)
	}

	// Re-locked: with the grant gone the read-only note denies edits again.
	relocked := f.preToolUse(f.writePayload("note.md", "---\nmode: read-only\n---\n# Note\n\nEdited again.\n"))
	relocked.requireDeny(t, "read_only")
}

// ---------------------------------------------------------------------------
// US8: fail-closed self-test. With the check-write binary unreachable, an
// in-vault write is BLOCKED (deny, exit 2), not allowed to fall through — the
// loud-failure property the removed verb gave for free. ADR-0031 §242.
// ---------------------------------------------------------------------------

func TestUS8_FailClosedWhenBinaryMissing(t *testing.T) {
	f := newFixture(t)
	f.writeNote("frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	f.commit("ratify frozen")

	cmd := f.preToolUseCommand(f.writePayload("frozen.md", "anything\n"))
	// Point the wrapper at a non-existent binary: check-write cannot run.
	cmd.Env = append(f.envWithout("MEMENTO_BIN"), "MEMENTO_BIN="+filepath.Join(t.TempDir(), "absent-memento"))
	stdout, stderr, code := runCommand(cmd)

	if code != 2 {
		t.Fatalf("exit = %d, want 2 (fail-closed block); stdout = %q stderr = %q", code, stdout, stderr)
	}
	v := parseVerdict(t, stdout, stderr, code)
	// The fail-closed deny is emitted by the bash wrapper itself, not check-write, so
	// it carries no reason_code (check-write never ran to log one) — assert the deny
	// decision and the fail-closed message rather than a code.
	if v.decision != "deny" {
		t.Fatalf("decision = %q, want deny (fail-closed block); stdout = %q stderr = %q", v.decision, stdout, stderr)
	}
	v.requireReasonContains(t, "could not run", "fail-closed")
	if stderr == "" {
		t.Fatalf("stderr empty; want a fail-closed diagnostic for the harness")
	}
}

// ---------------------------------------------------------------------------
// US9: PostToolUse compile + drift-alarm handshake. The PreToolUse gate records
// the bytes it expects to land; the PostToolUse compile compares disk to that
// expectation, recompiles the manifest, and raises a DRIFT ALARM (exit 2) when
// what landed disagrees with the gated expectation. ADR-0031 §241.
// ---------------------------------------------------------------------------

func TestUS9_CompileFiresAndDriftAlarm(t *testing.T) {
	const old = "---\nmode: append-only\n---\n# Log\n\nEntry one.\n"

	t.Run("coherent write recompiles without alarm", func(t *testing.T) {
		f := newFixture(t)
		f.writeNote("log.md", old)
		f.commit("ratify log")

		landed := old + "Entry two.\n"
		// Gate records the expected post-write hash for these exact bytes.
		f.preToolUse(f.writePayload("log.md", landed)).requireAllow(t)
		// The bytes the gate approved actually land on disk.
		f.writeNote("log.md", landed)

		stdout, stderr, code := f.postToolUse()
		if code != 0 {
			t.Fatalf("post-hook exit = %d, want 0; stdout = %q stderr = %q", code, stdout, stderr)
		}
		if strings.Contains(stderr, "DRIFT ALARM") {
			t.Fatalf("stderr = %q, want no drift alarm for a coherent write", stderr)
		}
		// Compile actually ran: the manifest reflects the note.
		manifest := readFile(t, filepath.Join(f.vault, ".memento", "manifest.json"))
		if !strings.Contains(manifest, "log.md") {
			t.Fatalf("manifest does not mention log.md after post-write compile:\n%s", manifest)
		}
	})

	t.Run("divergent landed bytes raise the drift alarm", func(t *testing.T) {
		f := newFixture(t)
		f.writeNote("log.md", old)
		f.commit("ratify log")

		// Gate approves (and records the hash of) one set of bytes...
		f.preToolUse(f.writePayload("log.md", old+"Entry two.\n")).requireAllow(t)
		// ...but a different set lands on disk (a replay/tamper divergence).
		f.writeNote("log.md", old+"Entry SURPRISE.\n")

		stdout, stderr, code := f.postToolUse()
		if code != 2 {
			t.Fatalf("post-hook exit = %d, want 2 (drift surfaced); stdout = %q stderr = %q", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "DRIFT ALARM") || !strings.Contains(stderr, "log.md") {
			t.Fatalf("stderr = %q, want a DRIFT ALARM naming log.md", stderr)
		}
	})
}

// ---------------------------------------------------------------------------
// US10: the write verb is gone; the surviving porcelain (write-mode, unlock) is
// advertised; check-write stays dispatchable hook plumbing, hidden from help.
// ADR-0031 "Consequences — CLI surface".
// ---------------------------------------------------------------------------

func TestUS10_WriteVerbGoneSurfaceIntact(t *testing.T) {
	f := newFixture(t)

	t.Run("write is an unknown command", func(t *testing.T) {
		stdout, stderr, code := f.memento("write", "note.md")
		if code != 2 {
			t.Fatalf("`memento write` exit = %d, want 2; stderr = %q", code, stderr)
		}
		if stdout != "" {
			t.Fatalf("`memento write` stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "unknown-command") || !strings.Contains(stderr, `unknown command "write"`) {
			t.Fatalf("`memento write` stderr = %q, want unknown-command message", stderr)
		}
	})

	t.Run("help advertises write-mode and unlock but not write or check-write", func(t *testing.T) {
		stdout, stderr, code := f.memento("help")
		if code != 0 {
			t.Fatalf("`memento help` exit = %d, want 0; stderr = %q", code, stderr)
		}
		for _, want := range []string{"memento write-mode", "memento unlock"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("help output does not contain %q:\n%s", want, stdout)
			}
		}
		if strings.Contains(stdout, "memento write [") || strings.Contains(stdout, "memento write <") {
			t.Fatalf("help still advertises the removed write verb:\n%s", stdout)
		}
		if strings.Contains(stdout, "check-write") {
			t.Fatalf("help advertises check-write hook plumbing:\n%s", stdout)
		}
	})

	t.Run("check-write stays dispatchable and inert for non-write tools", func(t *testing.T) {
		// A non-write tool payload is inert (exit 0, no verdict), proving the verb
		// still dispatches rather than falling through to unknown-command.
		stdout, stderr, code := f.mementoStdin(`{"tool_name":"Read"}`, "check-write")
		if code != 0 {
			t.Fatalf("`memento check-write` (inert) exit = %d, want 0; stderr = %q", code, stderr)
		}
		if strings.TrimSpace(stdout) != "" {
			t.Fatalf("`memento check-write` (inert) stdout = %q, want empty", stdout)
		}
	})
}

// ---------------------------------------------------------------------------
// US11: the Bash classifier. A provably-append `>>` redirect is allowed on an
// append-only note and denied on a read-only note; a truncating `>` (or any
// other recognisable write) to a vault path is denied. ADR-0031 ("Bash: deny
// unless provably append").
// ---------------------------------------------------------------------------

func TestUS11_BashClassifier(t *testing.T) {
	f := newFixture(t)
	f.writeNote("log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\n")
	f.writeNote("frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nFrozen.\n")
	f.commit("ratify notes")

	logPath := filepath.Join(f.vault, "log.md")
	frozenPath := filepath.Join(f.vault, "frozen.md")

	t.Run("append redirect to append-only allowed", func(t *testing.T) {
		v := f.preToolUse(f.bashPayload("printf 'Entry two.\\n' >> " + shellQuote(logPath)))
		v.requireAllow(t)
	})

	t.Run("truncating redirect to vault path denied", func(t *testing.T) {
		v := f.preToolUse(f.bashPayload("printf 'wiped\\n' > " + shellQuote(logPath)))
		v.requireDeny(t, "bash_opaque_write")
	})

	t.Run("append redirect to read-only denied", func(t *testing.T) {
		v := f.preToolUse(f.bashPayload("printf 'sneak\\n' >> " + shellQuote(frozenPath)))
		v.requireDeny(t, "read_only")
	})
}

// ---------------------------------------------------------------------------
// US12: codex apply_patch deny. An apply_patch Update to a ratified read-only
// note is denied, with a deny envelope byte-identical to the Claude Write
// contract. ADR-0031 ("Multi-agent" + codex hooks contract).
// ---------------------------------------------------------------------------

func TestUS12_CodexApplyPatchReadOnlyDenied(t *testing.T) {
	f := newFixture(t)
	f.writeNote("frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	f.commit("ratify frozen")
	target := filepath.Join(f.vault, "frozen.md")

	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: " + target,
		"@@",
		" # Frozen",
		" ",
		"-Original.",
		"+Rewritten.",
		"*** End Patch",
	}, "\n")

	codex := f.preToolUse(f.applyPatchPayload(patch))
	codex.requireDeny(t, "read_only")
	codex.requireReasonContains(t, "frozen.md", "denied again", "memento unlock")

	// The deny envelope must be byte-identical to the Claude Write deny.
	claude := f.preToolUse(f.writePayload("frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nRewritten.\n"))
	if codex.stdout != claude.stdout {
		t.Fatalf("apply_patch deny stdout not byte-identical to Claude Write deny:\n codex  = %q\n claude = %q", codex.stdout, claude.stdout)
	}
}

// =========================================================================
// Harness
// =========================================================================

// fixture is one black-box vault: a git repo with a `memento init`-installed
// vault at <repo>/memory, the PreToolUse / PostToolUse wrapper scripts, and the
// pre-commit / prepare-commit-msg git hooks. env carries the built binary on
// PATH (for the git hooks' `command -v memento`) and as MEMENTO_BIN (for the
// wrapper scripts).
type fixture struct {
	t     *testing.T
	repo  string
	vault string
	env   []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	requirePOSIXTools(t)

	repo := t.TempDir()
	f := &fixture{
		t:     t,
		repo:  repo,
		vault: filepath.Join(repo, "memory"),
	}
	binDir := filepath.Dir(mementoBin)
	f.env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MEMENTO_BIN="+mementoBin,
		// Make commits deterministic and unsigned regardless of the host config.
		"GIT_AUTHOR_NAME=Memento Acceptance",
		"GIT_AUTHOR_EMAIL=acc@example.invalid",
		"GIT_COMMITTER_NAME=Memento Acceptance",
		"GIT_COMMITTER_EMAIL=acc@example.invalid",
	)

	// Agent detection is per-family on the presence of the config dir, so create
	// .claude/ before init to wire the Claude gate scripts this fixture exercises.
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	f.git("init")
	if _, stderr, code := f.memento("init", "--dir", "memory"); code != 0 {
		t.Fatalf("`memento init` exit = %d; stderr = %q", code, stderr)
	}
	return f
}

// writeNote writes content to a vault-relative key (forward-slash), creating
// parent directories as needed.
func (f *fixture) writeNote(key, content string) {
	f.t.Helper()
	path := filepath.Join(f.vault, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatalf("mkdir parent for %q: %v", key, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatalf("write %q: %v", key, err)
	}
}

// commit stages everything and commits, firing the installed pre-commit and
// prepare-commit-msg hooks (so committed notes become ratified).
func (f *fixture) commit(message string) {
	f.t.Helper()
	f.git("add", "-A")
	f.gitCommit(message)
}

// commitAllowEmpty commits even when nothing is staged, used to fire the commit
// hooks (trailer lift + grant clear) without a content change.
func (f *fixture) commitAllowEmpty(message string) {
	f.t.Helper()
	f.gitCommit(message, "--allow-empty")
}

func (f *fixture) gitCommit(message string, extra ...string) {
	f.t.Helper()
	args := append([]string{"commit", "--no-gpg-sign", "-m", message}, extra...)
	f.git(args...)
}

// git runs a git command in the repo with the fixture env (so the hooks resolve
// `memento` on PATH), failing the test on a non-zero exit. It returns combined
// output for callers that inspect it (e.g. the commit log).
func (f *fixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.repo
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// memento runs the built binary in the repo with the fixture env.
func (f *fixture) memento(args ...string) (stdout, stderr string, code int) {
	f.t.Helper()
	return f.mementoStdin("", args...)
}

func (f *fixture) mementoStdin(stdin string, args ...string) (stdout, stderr string, code int) {
	f.t.Helper()
	cmd := exec.Command(mementoBin, args...)
	cmd.Dir = f.repo
	cmd.Env = f.env
	cmd.Stdin = strings.NewReader(stdin)
	return runCommand(cmd)
}

// mustMemento runs the binary and fails the test on a non-zero exit.
func (f *fixture) mustMemento(args ...string) {
	f.t.Helper()
	if stdout, stderr, code := f.memento(args...); code != 0 {
		f.t.Fatalf("`memento %s` exit = %d; stdout = %q stderr = %q", strings.Join(args, " "), code, stdout, stderr)
	}
}

// preToolUse drives the init-installed PreToolUse wrapper script with the given
// payload on stdin and parses the harness verdict.
func (f *fixture) preToolUse(payload string) verdict {
	f.t.Helper()
	cmd := f.preToolUseCommand(payload)
	stdout, stderr, code := runCommand(cmd)
	v := parseVerdict(f.t, stdout, stderr, code)
	// reason_code is NOT on the PreToolUse wire — codex's strict schema rejects
	// unknown top-level keys, so check-write records the code to the gitignored
	// decision log instead (ryr.37). Attach the latest logged reason_code so
	// requireDeny can assert it: a denial always logs an entry, so right after a
	// deny the log's last entry is this call's verdict.
	if rc, ok := f.lastDecisionReasonCode(); ok {
		v.reasonCode = rc
	}
	return v
}

// lastDecisionReasonCode returns the reason_code of the most recent decision-log
// entry (`<vault>/.memento/decision-log.jsonl`, matching
// enforce.DecisionLogFileName). ok is false when the log is absent or empty — a
// verdict that reached no logged event: a plain allow, or the fail-closed wrapper
// path where check-write never ran to record one.
func (f *fixture) lastDecisionReasonCode() (string, bool) {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.vault, ".memento", "decision-log.jsonl"))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		f.t.Fatalf("read decision log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var e struct {
			ReasonCode string `json:"reason_code"`
		}
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			f.t.Fatalf("parse decision-log line %q: %v", line, err)
		}
		return e.ReasonCode, true
	}
	return "", false
}

func (f *fixture) preToolUseCommand(payload string) *exec.Cmd {
	f.t.Helper()
	script := filepath.Join(f.repo, ".claude", "memento-pre-write-vault-guard.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = f.repo
	cmd.Env = f.env
	cmd.Stdin = strings.NewReader(payload)
	return cmd
}

// postToolUse drives the init-installed PostToolUse wrapper script (which always
// recompiles and surfaces a drift alarm). The hook ignores its stdin payload.
func (f *fixture) postToolUse() (stdout, stderr string, code int) {
	f.t.Helper()
	script := filepath.Join(f.repo, ".claude", "memento-post-write-compile.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = f.repo
	cmd.Env = f.env
	cmd.Stdin = strings.NewReader("")
	return runCommand(cmd)
}

// envWithout returns the fixture env with all assignments of key removed, so a
// caller can append a replacement (exec uses the last assignment otherwise, but
// removing keeps the intent explicit).
func (f *fixture) envWithout(key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(f.env))
	for _, kv := range f.env {
		if strings.HasPrefix(kv, prefix) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// ---- payload builders ----

func (f *fixture) writePayload(key, content string) string {
	f.t.Helper()
	return f.marshalPayload("Write", map[string]any{
		"file_path": filepath.Join(f.vault, filepath.FromSlash(key)),
		"content":   content,
	})
}

func (f *fixture) editPayload(key, oldStr, newStr string, replaceAll bool) string {
	f.t.Helper()
	return f.marshalPayload("Edit", map[string]any{
		"file_path":   filepath.Join(f.vault, filepath.FromSlash(key)),
		"old_string":  oldStr,
		"new_string":  newStr,
		"replace_all": replaceAll,
	})
}

func (f *fixture) bashPayload(command string) string {
	f.t.Helper()
	return f.marshalPayload("Bash", map[string]any{"command": command})
}

// applyPatchPayload renders a codex apply_patch PreToolUse payload, carrying the
// envelope under the untyped `input` key (codex hooks contract).
func (f *fixture) applyPatchPayload(patch string) string {
	f.t.Helper()
	return f.marshalPayload("apply_patch", map[string]any{"input": patch})
}

func (f *fixture) marshalPayload(tool string, input map[string]any) string {
	f.t.Helper()
	b, err := json.Marshal(map[string]any{"tool_name": tool, "tool_input": input})
	if err != nil {
		f.t.Fatalf("marshal %s payload: %v", tool, err)
	}
	return string(b)
}

// ---- verdict ----

// verdict is the decoded harness PreToolUse response plus the raw streams.
type verdict struct {
	decision   string
	reasonCode string
	reason     string
	stdout     string
	stderr     string
	code       int
}

func parseVerdict(t *testing.T, stdout, stderr string, code int) verdict {
	t.Helper()
	v := verdict{stdout: stdout, stderr: stderr, code: code}
	if strings.TrimSpace(stdout) == "" {
		return v // inert: no verdict emitted
	}
	// reason_code is deliberately absent from the wire (ryr.37); only decision and
	// the human reason ride the verdict JSON. The caller attaches reason_code from
	// the decision log (see fixture.preToolUse / lastDecisionReasonCode).
	var parsed struct {
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("verdict stdout = %q, not valid JSON: %v", stdout, err)
	}
	v.decision = parsed.HookSpecificOutput.PermissionDecision
	v.reason = parsed.HookSpecificOutput.PermissionDecisionReason
	return v
}

func (v verdict) requireAllow(t *testing.T) {
	t.Helper()
	if v.code != 0 {
		t.Fatalf("exit = %d, want 0; stderr = %q", v.code, v.stderr)
	}
	if v.decision != "allow" {
		t.Fatalf("decision = %q, want allow; stdout = %q stderr = %q", v.decision, v.stdout, v.stderr)
	}
}

func (v verdict) requireDeny(t *testing.T, reasonCode string) {
	t.Helper()
	if v.decision != "deny" {
		t.Fatalf("decision = %q, want deny; stdout = %q stderr = %q", v.decision, v.stdout, v.stderr)
	}
	if v.reasonCode != reasonCode {
		t.Fatalf("reason_code = %q, want %q; reason = %q", v.reasonCode, reasonCode, v.reason)
	}
}

func (v verdict) requireReasonContains(t *testing.T, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(v.reason, want) {
			t.Fatalf("reason = %q, want it to contain %q", v.reason, want)
		}
	}
}

// =========================================================================
// Shared helpers
// =========================================================================

func runCommand(cmd *exec.Cmd) (stdout, stderr string, code int) {
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	_ = cmd.Run()
	return out.String(), errOut.String(), cmd.ProcessState.ExitCode()
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func repoRoot() string {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(testFile), ".."))
}

func requirePOSIXTools(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("acceptance suite drives POSIX shell wrapper scripts and git hooks")
	}
	for _, tool := range []string{"bash", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found: %v", tool, err)
		}
	}
}

func shellQuote(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `'\''`) + `'`
}
