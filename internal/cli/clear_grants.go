package cli

import (
	"fmt"
	"io"

	"github.com/tpisel/memento/internal/enforce"
)

// runClearGrants is the pre-commit hook plumbing (ADR-0031, 2026-06-28 addendum):
// on every commit it drops ALL pending unlock grants. Clearing every grant is the
// decided "any commit re-locks" semantics — grant deletion, not ratification, is
// what re-locks an already-ratified read-only note. It runs in the pre-commit hook
// AFTER `memento compile` (whose ratification-boundary audit reads the grants to
// waive grant-covered changes), so the audit still sees a grant before this clears
// it.
//
// It supersedes the retired `lift-grants` / prepare-commit-msg trailer step: unlock
// justifications are no longer lifted into a Memento-Unlock commit trailer (the
// decision moved loosening audit into the gitignored decision log, and unlock
// justifications are intentionally not persisted past the grant's lifetime). See
// [[loosening justification persistence]].
//
// Not listed in help: hook plumbing, not an agent verb.
func runClearGrants(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintf(stderr, "memento clear-grants: takes no arguments\n")
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		// No vault means no grants to clear; never block the commit on this.
		return 0
	}

	if err := enforce.ClearGrants(v); err != nil {
		fmt.Fprintf(stderr, "memento clear-grants: %v\n", err)
		return 1
	}
	return 0
}
