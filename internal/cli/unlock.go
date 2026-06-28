package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/note"
)

// runUnlock records a temporary single-key unlock grant (ADR-0031): it re-opens
// the edit window on a read-only note until the next commit, when the pre-commit
// hook clears the grant sidecar (grant deletion is the re-lock). --justification
// is required as deliberate friction — the agent must state a reason to thaw a
// frozen note — and is surfaced on stderr, but is intentionally NOT persisted past
// the grant's lifetime (ADR-0031 2026-06-28 addendum: no commit trailer; only
// durable write-mode loosenings are logged). The grant is the only artifact; no
// recompile is needed because the gitignored sidecar is not vault corpus.
func runUnlock(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("unlock", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	justification := flags.String("justification", "", "reason the read-only note is being temporarily unlocked (required)")

	positional, ok, code := parseInterspersedFlags(flags, args, stdout, stderr, "unlock", unlockHelpText)
	if !ok {
		return code
	}
	if len(positional) != 1 {
		printCLIError(stderr, "unlock", fmt.Errorf("%w: expected <key>", ErrInvalidArguments))
		return 2
	}
	reason := strings.TrimSpace(*justification)
	if reason == "" {
		printCLIError(stderr, "unlock", fmt.Errorf("%w: --justification <reason> is required", ErrInvalidArguments))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "unlock", err)
		return 1
	}
	key, err := enforce.NormalizeWritableKey(v, positional[0])
	if err != nil {
		printCLIError(stderr, "unlock", err)
		return 1
	}
	path, err := enforce.ResolveWritablePath(v, key)
	if err != nil {
		printCLIError(stderr, "unlock", err)
		return 1
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		printCLIError(stderr, "unlock", fmt.Errorf("%w: %s", note.ErrNotFound, key))
		return 1
	} else if err != nil {
		printCLIError(stderr, "unlock", fmt.Errorf("%w: stat %s: %v", ErrIO, key, err))
		return 1
	}

	if err := enforce.AddGrant(v, key, reason, time.Now().UTC()); err != nil {
		printCLIError(stderr, "unlock", fmt.Errorf("%w: %v", ErrIO, err))
		return 1
	}

	fmt.Fprintf(stderr, "unlocked: %s until next commit\n", key)
	fmt.Fprintf(stderr, "justification: %s\n", reason)
	return 0
}
