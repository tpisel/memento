package enforce

import (
	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/vault"
)

// IsRatified reports whether key is committed in the vault's git tree. It reuses
// the predicate in internal/note rather than forking the git mechanics; a
// non-git tree is treated as ratified. read-only/append-only modes bite only
// after ratification (ADR-0017), so check-write composes this with EvaluateMode.
func IsRatified(v vault.Vault, key string) (bool, error) {
	return note.IsRatified(v, key)
}
