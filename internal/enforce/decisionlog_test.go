package enforce

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tpisel/memento/internal/vault"
)

func TestAppendDecisionLogCreatesAndAppends(t *testing.T) {
	v := vaultFromRoot(makeVault(t))

	first := DecisionLogEntry{
		Time:       time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC),
		Event:      EventDeny,
		Tool:       "Write",
		Key:        "frozen.md",
		Decision:   "deny",
		ReasonCode: ReasonReadOnly,
	}
	second := DecisionLogEntry{
		Time:     time.Date(2026, 6, 27, 9, 1, 0, 0, time.UTC),
		Event:    EventGrantConsumption,
		Tool:     "Edit",
		Key:      "spec.md",
		Decision: "allow",
	}

	if err := AppendDecisionLog(v, first); err != nil {
		t.Fatalf("AppendDecisionLog(first) error = %v, want nil", err)
	}
	if err := AppendDecisionLog(v, second); err != nil {
		t.Fatalf("AppendDecisionLog(second) error = %v, want nil", err)
	}

	entries := readDecisionLog(t, v)
	if len(entries) != 2 {
		t.Fatalf("decision log = %d entries, want 2: %+v", len(entries), entries)
	}
	if entries[0] != first {
		t.Fatalf("entry[0] = %+v, want %+v", entries[0], first)
	}
	if entries[1] != second {
		t.Fatalf("entry[1] = %+v, want %+v", entries[1], second)
	}
}

func TestAppendDecisionLogOmitsEmptyOptionalFields(t *testing.T) {
	v := vaultFromRoot(makeVault(t))

	// An opaque Bash denial carries no key and no reason mapping is recorded as a
	// content reason: the omitempty tags must keep those fields out of the line.
	if err := AppendDecisionLog(v, DecisionLogEntry{
		Time:     time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC),
		Event:    EventDeny,
		Tool:     "Bash",
		Decision: "deny",
	}); err != nil {
		t.Fatalf("AppendDecisionLog() error = %v, want nil", err)
	}

	data, err := os.ReadFile(DecisionLogPath(v))
	if err != nil {
		t.Fatalf("read decision log: %v", err)
	}
	line := strings.TrimSpace(string(data))
	if strings.Contains(line, "\"key\"") || strings.Contains(line, "\"reason_code\"") {
		t.Fatalf("decision log line = %q, want no empty key/reason_code fields", line)
	}
}

func readDecisionLog(t *testing.T, v vault.Vault) []DecisionLogEntry {
	t.Helper()
	f, err := os.Open(DecisionLogPath(v))
	if err != nil {
		t.Fatalf("open decision log: %v", err)
	}
	defer f.Close()

	var entries []DecisionLogEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var e DecisionLogEntry
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("decision log line %q is not valid JSON: %v", line, err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan decision log: %v", err)
	}
	return entries
}
