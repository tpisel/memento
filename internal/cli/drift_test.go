package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/vault"
)

// vaultFor builds a Vault value addressing the test vault at root. Only the
// fields the handshake sidecar helpers touch (Root, MarkerDir) are populated.
func vaultFor(root string) vault.Vault {
	return vault.Vault{Root: root, MarkerDir: filepath.Join(root, vault.MarkerDirName)}
}

func runCLICompile(t *testing.T) (string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"compile"}, &stdout, &stderr)
	return stderr.String(), code
}

// TestCheckWriteAllowRecordsPendingExpectation: an allowed Write records the
// expected post-write bytes-hash under the key — the PreToolUse half of the
// check-write↔compile handshake (ADR-0031, US9).
func TestCheckWriteAllowRecordsPendingExpectation(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)

	target := filepath.Join(root, "fresh.md")
	content := "---\nmode: append-only\n---\n# Fresh\n\nBody.\n"
	decision, _, _, _, stderr, code := invokeCheckWrite(t, checkWritePayload(t, "Write", target, content))
	if code != 0 || decision != "allow" {
		t.Fatalf("verdict = (%q, exit %d); want (allow, 0); stderr = %q", decision, code, stderr)
	}

	pending, err := enforce.LoadPending(vaultFor(root))
	if err != nil {
		t.Fatalf("load pending: %v", err)
	}
	want := enforce.HashBytes([]byte(content))
	if got := pending["fresh.md"]; got != want {
		t.Fatalf("pending[fresh.md] = %q, want %q (full ledger = %v)", got, want, pending)
	}
}

// TestCheckWriteDenyRecordsNoExpectation: a denied write must not leave a stale
// expectation that would later fire a false drift alarm.
func TestCheckWriteDenyRecordsNoExpectation(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)

	target := filepath.Join(root, "frozen.md")
	decision, _, _, _, _, _ := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: read-only\n---\n# Frozen\n\nRewritten.\n"))
	if decision != "deny" {
		t.Fatalf("decision = %q, want deny", decision)
	}

	pending, err := enforce.LoadPending(vaultFor(root))
	if err != nil {
		t.Fatalf("load pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %v, want empty after a denied write", pending)
	}
}

// TestCheckWriteBashAppendRecordsNoExpectation: a Bash `>>` append is allowed but
// its landed bytes are not derivable, so it must leave no drift expectation
// (ADR-0031: Bash carries no path in the post payload; compile just recompiles).
func TestCheckWriteBashAppendRecordsNoExpectation(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\n")
	commitCLIGit(t, root)

	decision, _, _, _, _, code := invokeCheckWrite(t, checkBashPayload(t, "echo more >> log.md"))
	if code != 0 || decision != "allow" {
		t.Fatalf("bash append verdict = (%q, exit %d), want (allow, 0)", decision, code)
	}

	pending, err := enforce.LoadPending(vaultFor(root))
	if err != nil {
		t.Fatalf("load pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %v, want empty (Bash appends record no expectation)", pending)
	}
}

// TestCompileDriftAlarmFiresOnMismatch: with a gated expectation recorded but
// different bytes on disk, compile raises the loud DRIFT ALARM and then clears
// the ledger so the alarm fires once (ADR-0031, US9).
func TestCompileDriftAlarmFiresOnMismatch(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "---\nmode: append-only\n---\n# Note\n\nLanded body.\n")

	// check-write gated bytes that differ from what actually landed.
	if err := enforce.RecordPending(vaultFor(root), "note.md", enforce.HashBytes([]byte("gated-but-not-what-landed"))); err != nil {
		t.Fatalf("record pending: %v", err)
	}

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "DRIFT ALARM") || !strings.Contains(stderr, "note.md") {
		t.Fatalf("stderr = %q, want a DRIFT ALARM naming note.md", stderr)
	}

	pending, err := enforce.LoadPending(vaultFor(root))
	if err != nil {
		t.Fatalf("load pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %v, want cleared after the drift pass", pending)
	}
}

// TestCompileNoDriftWhenLandedMatches: when disk matches the gated expectation,
// compile is silent on drift and still clears the verified entry.
func TestCompileNoDriftWhenLandedMatches(t *testing.T) {
	root := makeCLIVault(t)
	body := "---\nmode: append-only\n---\n# Note\n\nLanded body.\n"
	writeCLIFile(t, root, "note.md", body)

	if err := enforce.RecordPending(vaultFor(root), "note.md", enforce.HashBytes([]byte(body))); err != nil {
		t.Fatalf("record pending: %v", err)
	}

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.Contains(stderr, "DRIFT ALARM") {
		t.Fatalf("stderr = %q, want no DRIFT ALARM when disk matches the gated bytes", stderr)
	}

	pending, err := enforce.LoadPending(vaultFor(root))
	if err != nil {
		t.Fatalf("load pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %v, want cleared after verification", pending)
	}
}

// TestCompileDriftAlarmFiresWhenFileAbsent: an expectation whose file never
// landed (an approved write the harness ultimately did not perform) is itself a
// drift signal — compile must not stay silent.
func TestCompileDriftAlarmFiresWhenFileAbsent(t *testing.T) {
	root := makeCLIVault(t)

	if err := enforce.RecordPending(vaultFor(root), "ghost.md", enforce.HashBytes([]byte("expected"))); err != nil {
		t.Fatalf("record pending: %v", err)
	}

	stderr, code := runCLICompile(t)
	if code != 0 {
		t.Fatalf("compile exit = %d, want 0; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "DRIFT ALARM") || !strings.Contains(stderr, "ghost.md") {
		t.Fatalf("stderr = %q, want a DRIFT ALARM naming ghost.md", stderr)
	}
}
