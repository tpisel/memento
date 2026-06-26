package enforce

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tpisel/memento/internal/vault"
)

// DecisionLogFileName is the check-write decision log: the structured, gitignored
// audit the retired write verb got for free via its commit trail (ADR-0031). It
// lives under the marker dir beside the unlock-grant and pending-write sidecars,
// and like them is gitignored; the manifest and config beside it stay tracked.
// It is JSONL — one verdict per line — so it is append-only, greppable, and never
// needs a read-modify-rewrite that could race with a concurrent verdict.
const DecisionLogFileName = "decision-log.jsonl"

// The enforcement-visible events the log records (ADR-0031 enumerates exactly
// these three): outright denials, the drive-by mode-change blocks broken out so
// they are distinguishable from ordinary content denials, and the writes a
// temporary unlock grant permitted that would otherwise have been denied. Plain
// allows are deliberately not recorded: the log is the enforcement audit, not a
// write journal, so it stays scoped to what enforcement actually did.
const (
	EventDeny             = "deny"
	EventDriveByBlock     = "drive_by_block"
	EventGrantConsumption = "grant_consumption"
)

// DecisionLogPath returns the absolute path of the decision log for v.
func DecisionLogPath(v vault.Vault) string {
	return filepath.Join(v.MarkerDir, DecisionLogFileName)
}

// DecisionLogEntry is one structured check-write verdict record. Time is the
// wall-clock instant the verdict was reached (UTC); Tool names the originating
// write tool (Write/Edit/MultiEdit/Bash/apply_patch); Key is the vault-relative
// note the write targeted (empty when the denial fires before a key resolves,
// e.g. an opaque Bash write); Decision is the harness verdict; ReasonCode is the
// denial-UX code that drove it.
type DecisionLogEntry struct {
	Time       time.Time `json:"time"`
	Event      string    `json:"event"`
	Tool       string    `json:"tool"`
	Key        string    `json:"key,omitempty"`
	Decision   string    `json:"decision"`
	ReasonCode string    `json:"reason_code,omitempty"`
}

// AppendDecisionLog appends one verdict record to the decision log, creating the
// marker dir and the log file on first write. It marshals to a single line and
// appends, so concurrent verdicts never clobber each other's records. The caller
// treats a returned error as best-effort: a log-write failure must never flip a
// verdict — the audit degrades to a missed line, surfaced on stderr — so this is
// the only mutation check-write performs that is allowed to fail silently.
func AppendDecisionLog(v vault.Vault, e DecisionLogEntry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode decision log entry: %w", err)
	}
	line = append(line, '\n')
	if err := os.MkdirAll(v.MarkerDir, 0o755); err != nil {
		return fmt.Errorf("create marker directory: %w", err)
	}
	f, err := os.OpenFile(DecisionLogPath(v), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open decision log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("append decision log: %w", err)
	}
	return nil
}
