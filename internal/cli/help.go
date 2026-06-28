package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

const briefHelpText = `memento brief

Usage:
  memento brief

Print the agent-facing manifest projection: titles, summaries, tags, headings, modes, and @N read references.

No flags.

For the deeper picture, run: memento orient
`

const compileHelpText = `memento compile

Usage:
  memento compile

Rebuild .memento/manifest.json and _memento/brief.md from the discovered memory vault.

No flags.

For the deeper picture, run: memento orient
`

const initHelpText = `memento init

Usage:
  memento init [--dir <vault>] [--agents detect|none|claude,codex]

Adopt or create a memory vault and install project bootstrapping artifacts.

Flags:
  --dir <vault>     Vault root to adopt or create. Defaults to <project>-memory/.
  --agents <value>  Agent hook integrations to install. Default: detect.
                    Values: detect, none, claude, codex, claude,codex.

Init reports the vault it created or adopted, the agent families it detected or was asked to install, and any post-install trust steps.

For the deeper picture, run: memento orient
`

const orientHelpText = `memento orient

Usage:
  memento orient

Print tool-usage orientation: verb semantics, write modes, triggered preconditions, and opt-in project overlays.

No flags.

For the deeper picture, run: memento orient
`

const readHelpText = `memento read

Usage:
  memento read <key|@N>
  memento read <key|@N>#<heading>

Read a vault note or one heading section.

Key contract:
  key is a vault-relative .md path, for example spec.md or Architecture decision record/adr-0001.md.

References:
  @N reads the numbered entry shown by memento brief.
  key#heading and @N#heading read a single section by heading text or slug.

Stderr metadata:
  stdout is the raw markdown body.
  stderr begins with binding: ratified|unratified and summary: current|stale|missing.
  stderr may also include role-flattened link lines: inlinks:, outlinks:, transcludes:, transcluded-by:.

For the deeper picture, run: memento orient
`

const conventionHelpText = `memento convention

Usage:
  memento convention <name>

Read an operational convention from _memento/conventions/<name>.md and print its body without frontmatter.

Name contract:
  name is a bare lowercase filename stem: no slash, extension, spaces, or path traversal. For example writing, not writing.md.

Conventions:
  Conventions are operational guidance declared by a non-empty when_to_read: in frontmatter; they are not part of the normal brief corpus.
  memento orient lists the conventions worth reading and when to read each.

Errors:
  convention-not-found    no _memento/conventions/<name>.md.
  invalid-convention      the file exists but has no non-empty when_to_read.

For the deeper picture, run: memento orient
`

const writeModeHelpText = `memento write-mode

Usage:
  memento write-mode <key> <append-only|living|read-only> [--justification <reason>]

Durably rewrite a note's frontmatter mode: line, then recompile the vault. This is the only path that changes an existing note's mode.

Key contract:
  key is a vault-relative .md path. Repo-relative paths and paths prefixed with the vault directory are invalid.

Modes:
  append-only  accepts appends, rejects overwrites once ratified.
  living       accepts appends and overwrites.
  read-only    rejects writes once ratified.
  An unknown mode value is rejected, not defaulted.

Justification:
  Loosening toward living requires --justification <reason>.
  Tightening toward read-only accepts --justification as optional self-documentation.

Stderr:
  On success, stderr includes mode: <key> <old> -> <new> before the compile result.

For the deeper picture, run: memento orient
`

const unlockHelpText = `memento unlock

Usage:
  memento unlock <key> --justification <reason>

Record a temporary single-key exception that re-opens the edit window on a read-only note until the next commit.

Key contract:
  key is a vault-relative .md path naming an existing note. Repo-relative paths and paths prefixed with the vault directory are invalid.

Justification:
  --justification <reason> is required as deliberate friction — you must state why you are thawing a frozen note. It is held in a gitignored .memento/unlock-grants.json sidecar for the grant's lifetime and surfaced on stderr, but is not persisted past the grant (only durable write-mode loosenings are recorded, in the gitignored .memento/ decision log).

Lifetime:
  The grant covers all writes to the key until the next commit, when it is cleared. There is no TTL and no durable mode change; use write-mode to change a note's mode permanently.

Stderr:
  On success, stderr includes unlocked: <key> until next commit and justification: <reason>.

For the deeper picture, run: memento orient
`

func parseSubcommandFlags(flags *flag.FlagSet, args []string, stdout, stderr io.Writer, verb, helpText string) (bool, int) {
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, helpText)
			return false, 0
		}
		printCLIError(stderr, verb, fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return false, 2
	}
	return true, 0
}
