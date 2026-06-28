package enforce

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tpisel/memento/internal/vault"
)

// PendingFileName is the pending-write ledger, the gitignored sidecar that
// carries the check-write↔compile handshake (ADR-0031): check-write (PreToolUse)
// records the bytes-hash it expects each gated write to land; compile
// (PostToolUse) compares disk against it, shouts on mismatch, then clears it.
// Like the unlock-grant sidecar it lives under the marker dir and is gitignored;
// the manifest and config beside it stay tracked.
const PendingFileName = "pending-writes.json"

// PendingPath returns the absolute path of the pending-write ledger for v.
func PendingPath(v vault.Vault) string {
	return filepath.Join(v.MarkerDir, PendingFileName)
}

// HashBytes is the canonical ledger hash of a note's full on-disk bytes. The
// handshake hashes whole-file content (what a Write lands / an Edit replay
// produces), not the post-frontmatter body the manifest's body_sha covers, so
// the two are deliberately distinct.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", sum)
}

// LoadPending reads the expected post-write hash per vault-relative key. A
// missing sidecar is the steady state and returns an empty, non-nil map.
// Malformed JSON is an error: a corrupt ledger must not be read as "nothing to
// verify", which would silently disarm the drift alarm.
func LoadPending(v vault.Vault) (map[string]string, error) {
	data, err := os.ReadFile(PendingPath(v))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending writes: %w", err)
	}
	pending := map[string]string{}
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, fmt.Errorf("parse pending writes at %s: %w", PendingPath(v), err)
	}
	return pending, nil
}

// RecordPending records (or overwrites) the expected post-write hash for key,
// merging into any existing ledger so concurrent expectations on other keys
// survive. A second gated write to the same key overwrites the older
// expectation — the latest gated bytes are what should land. check-write calls
// this only on an allow verdict for a tool whose new-bytes it derived exactly
// (Write/Edit/MultiEdit); Bash appends carry no derivable hash and record none.
func RecordPending(v vault.Vault, key, hash string) error {
	pending, err := LoadPending(v)
	if err != nil {
		return err
	}
	pending[key] = hash
	data, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pending writes: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(v.MarkerDir, 0o755); err != nil {
		return fmt.Errorf("create marker directory: %w", err)
	}
	if err := os.WriteFile(PendingPath(v), data, 0o644); err != nil {
		return fmt.Errorf("write pending writes: %w", err)
	}
	return nil
}

// ClearPending removes the entire ledger, dropping every unverified expectation
// at once. compile calls it after a drift pass so each expectation is checked
// exactly once and the alarm does not re-fire on every later compile. A missing
// sidecar is a no-op: clearing is idempotent.
func ClearPending(v vault.Vault) error {
	if err := os.Remove(PendingPath(v)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear pending writes: %w", err)
	}
	return nil
}
