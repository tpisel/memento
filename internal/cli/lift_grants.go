package cli

import (
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"

	"github.com/tpisel/memento/internal/enforce"
)

// runLiftGrants is the prepare-commit-msg hook plumbing (ADR-0031): on every
// commit it lifts each pending unlock grant's justification into a
// Memento-Unlock: commit trailer, then clears ALL grants. The sidecar is
// gitignored and commit-cleared, so the trailer is the only place the *why*
// survives; clearing every grant is the decided "any commit re-locks" semantics
// (grant deletion, not ratification, is what re-locks an already-ratified
// read-only note).
//
// It lives in prepare-commit-msg rather than pre-commit because pre-commit runs
// before the commit message exists and cannot write a trailer; prepare-commit-msg
// runs only after pre-commit succeeds and owns the message file ($1). See
// [[unlock-grant trailer lift runs in prepare-commit-msg]].
//
// Not listed in help: hook plumbing, not an agent verb.
func runLiftGrants(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "memento lift-grants: expected <commit-msg-file>\n")
		return 2
	}
	msgPath := args[0]

	v, err := resolveVault()
	if err != nil {
		// No vault means no grants to lift; never block the commit on this.
		return 0
	}

	grants, err := enforce.LoadGrants(v)
	if err != nil {
		// A corrupt sidecar must not be silently cleared as "no exceptions" —
		// that would drop the audit trail. Fail closed so the commit surfaces it.
		fmt.Fprintf(stderr, "memento lift-grants: %v\n", err)
		return 1
	}
	if len(grants) == 0 {
		// The steady state: leave the commit message untouched.
		return 0
	}

	keys := make([]string, 0, len(grants))
	for key := range grants {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	trailerArgs := []string{"interpret-trailers", "--in-place"}
	for _, key := range keys {
		reason := collapseTrailerValue(grants[key].Justification)
		trailerArgs = append(trailerArgs, "--trailer", fmt.Sprintf("Memento-Unlock=%s: %s", key, reason))
	}
	trailerArgs = append(trailerArgs, msgPath)

	cmd := exec.Command("git", trailerArgs...)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "memento lift-grants: write Memento-Unlock trailer: %v\n", err)
		return 1
	}

	if err := enforce.ClearGrants(v); err != nil {
		fmt.Fprintf(stderr, "memento lift-grants: clear grants: %v\n", err)
		return 1
	}
	return 0
}

// collapseTrailerValue flattens a justification to a single trailer-safe line.
// Git trailers are one logical line; embedded newlines would fold or corrupt the
// block, so they collapse to spaces.
func collapseTrailerValue(value string) string {
	replaced := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(value)
	return strings.TrimSpace(replaced)
}
