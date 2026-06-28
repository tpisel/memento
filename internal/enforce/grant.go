package enforce

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tpisel/memento/internal/vault"
)

// GrantsFileName is the unlock-grant sidecar, stored under the vault marker
// directory and gitignored (ADR-0031). The manifest and config beside it stay
// tracked; only this file is ignored.
const GrantsFileName = "unlock-grants.json"

// Grant is a single temporary read-only exception (ADR-0031): it re-opens the
// edit window on one key until the next commit, when the pre-commit hook clears
// the sidecar (grant deletion is the re-lock). There is no TTL — the grant's
// lifetime is identical to ratification's edit window. The justification is held
// here for the grant's lifetime only; it is not persisted past the clear
// (ADR-0031 2026-06-28 addendum: the Memento-Unlock trailer is retired).
type Grant struct {
	Justification string    `json:"justification"`
	GrantedAt     time.Time `json:"granted_at"`
}

// GrantsPath returns the absolute path of the unlock-grant sidecar for v.
func GrantsPath(v vault.Vault) string {
	return filepath.Join(v.MarkerDir, GrantsFileName)
}

// LoadGrants reads every active unlock grant keyed by vault-relative key. A
// missing sidecar is not an error — it returns an empty, non-nil map (no grants
// is the normal steady state). Malformed JSON is an error: a corrupt sidecar
// must not be silently read as "no exceptions". This is the list helper the
// ratification-boundary audit (compile) consumes to waive grant-covered changes
// before the pre-commit hook clears the grants.
func LoadGrants(v vault.Vault) (map[string]Grant, error) {
	data, err := os.ReadFile(GrantsPath(v))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Grant{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read unlock grants: %w", err)
	}
	grants := map[string]Grant{}
	if err := json.Unmarshal(data, &grants); err != nil {
		return nil, fmt.Errorf("parse unlock grants at %s: %w", GrantsPath(v), err)
	}
	return grants, nil
}

// LookupGrant reports whether key has an active unlock grant and returns it.
// check-write composes this into its mutability predicate
// (mutable = unratified ∪ active grant).
func LookupGrant(v vault.Vault, key string) (Grant, bool, error) {
	grants, err := LoadGrants(v)
	if err != nil {
		return Grant{}, false, err
	}
	g, ok := grants[key]
	return g, ok, nil
}

// AddGrant records (or refreshes) the unlock grant for key, merging into any
// existing sidecar rather than replacing it so concurrent grants on other keys
// survive. grantedAt is supplied by the caller so the verb stays deterministic
// under test.
func AddGrant(v vault.Vault, key, justification string, grantedAt time.Time) error {
	grants, err := LoadGrants(v)
	if err != nil {
		return err
	}
	grants[key] = Grant{Justification: justification, GrantedAt: grantedAt}
	data, err := json.MarshalIndent(grants, "", "  ")
	if err != nil {
		return fmt.Errorf("encode unlock grants: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(GrantsPath(v), data, 0o644); err != nil {
		return fmt.Errorf("write unlock grants: %w", err)
	}
	return nil
}

// ClearGrants removes the entire sidecar, dropping every active grant at once.
// It is the re-lock the pre-commit hook performs (via `memento clear-grants`,
// after `memento compile`). A missing sidecar is a no-op: clearing is idempotent.
func ClearGrants(v vault.Vault) error {
	if err := os.Remove(GrantsPath(v)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear unlock grants: %w", err)
	}
	return nil
}
