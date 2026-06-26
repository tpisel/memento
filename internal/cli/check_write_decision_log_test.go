package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/vault"
)

// decisionLogEntries reads and decodes the gitignored check-write decision log
// for the vault rooted at root, or returns nil when the log does not yet exist
// (the steady state when no enforcement-visible verdict has been reached).
func decisionLogEntries(t *testing.T, root string) []enforce.DecisionLogEntry {
	t.Helper()
	path := filepath.Join(root, vault.MarkerDirName, enforce.DecisionLogFileName)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("open decision log: %v", err)
	}
	defer f.Close()

	var entries []enforce.DecisionLogEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var e enforce.DecisionLogEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("decision log line %q is not valid JSON: %v", sc.Text(), err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan decision log: %v", err)
	}
	return entries
}

func lastDecisionLogEntry(t *testing.T, root string) enforce.DecisionLogEntry {
	t.Helper()
	entries := decisionLogEntries(t, root)
	if len(entries) == 0 {
		t.Fatalf("decision log empty, want at least one entry")
	}
	return entries[len(entries)-1]
}

func TestCheckWriteDecisionLogRecordsDeny(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)

	target := filepath.Join(root, "frozen.md")
	decision, _, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: read-only\n---\n# Frozen\n\nRewritten.\n"))
	if code != 0 || decision != "deny" {
		t.Fatalf("verdict = (%q, exit %d), want (deny, 0); stderr = %q", decision, code, stderr)
	}

	got := lastDecisionLogEntry(t, root)
	if got.Event != enforce.EventDeny {
		t.Fatalf("logged event = %q, want %q", got.Event, enforce.EventDeny)
	}
	if got.Decision != "deny" || got.ReasonCode != enforce.ReasonReadOnly {
		t.Fatalf("logged verdict = (%q,%q), want (deny, read_only)", got.Decision, got.ReasonCode)
	}
	if got.Tool != "Write" || got.Key != "frozen.md" {
		t.Fatalf("logged tool/key = (%q,%q), want (Write, frozen.md)", got.Tool, got.Key)
	}
	if got.Time.IsZero() {
		t.Fatalf("logged time is zero, want a wall-clock instant")
	}
}

func TestCheckWriteDecisionLogRecordsDriveByBlock(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)

	var ustdout, ustderr bytes.Buffer
	if c := Run([]string{"unlock", "frozen.md", "--justification", "fix typo"}, &ustdout, &ustderr); c != 0 {
		t.Fatalf("unlock exit code = %d, want 0; stderr = %q", c, ustderr.String())
	}

	target := filepath.Join(root, "frozen.md")
	decision, reasonCode, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: living\n---\n# Frozen\n\nFixed.\n"))
	if code != 0 || decision != "deny" || reasonCode != enforce.ReasonDriveByModeChange {
		t.Fatalf("verdict = (%q,%q, exit %d), want (deny, drive_by_mode_change, 0); stderr = %q", decision, reasonCode, code, stderr)
	}

	got := lastDecisionLogEntry(t, root)
	if got.Event != enforce.EventDriveByBlock {
		t.Fatalf("logged event = %q, want %q", got.Event, enforce.EventDriveByBlock)
	}
	if got.ReasonCode != enforce.ReasonDriveByModeChange {
		t.Fatalf("logged reason_code = %q, want drive_by_mode_change", got.ReasonCode)
	}
}

func TestCheckWriteDecisionLogRecordsGrantConsumption(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)

	var ustdout, ustderr bytes.Buffer
	if c := Run([]string{"unlock", "frozen.md", "--justification", "fix typo"}, &ustdout, &ustderr); c != 0 {
		t.Fatalf("unlock exit code = %d, want 0; stderr = %q", c, ustderr.String())
	}

	target := filepath.Join(root, "frozen.md")
	decision, _, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: read-only\n---\n# Frozen\n\nFixed.\n"))
	if code != 0 || decision != "allow" {
		t.Fatalf("verdict = (%q, exit %d), want (allow, 0); stderr = %q", decision, code, stderr)
	}

	got := lastDecisionLogEntry(t, root)
	if got.Event != enforce.EventGrantConsumption {
		t.Fatalf("logged event = %q, want %q", got.Event, enforce.EventGrantConsumption)
	}
	if got.Decision != "allow" || got.Key != "frozen.md" {
		t.Fatalf("logged verdict = (%q, key %q), want (allow, frozen.md)", got.Decision, got.Key)
	}
}

func TestCheckWriteDecisionLogSkipsPlainAllow(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	// A brand-new note: allowed without any grant in play. The log is the
	// enforcement audit, not a write journal, so an ordinary allow records nothing.
	target := filepath.Join(root, "fresh.md")
	decision, _, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: append-only\n---\n# Fresh\n\nNew.\n"))
	if code != 0 || decision != "allow" {
		t.Fatalf("verdict = (%q, exit %d), want (allow, 0); stderr = %q", decision, code, stderr)
	}

	if entries := decisionLogEntries(t, root); len(entries) != 0 {
		t.Fatalf("decision log = %+v, want empty after a plain allow", entries)
	}
}
