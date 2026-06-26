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
  memento init [--dir <vault>]

Adopt or create a memory vault and install project bootstrapping artifacts.

Flags:
  --dir <vault>   Vault root to adopt or create. Defaults to <project>-memory/.

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

const writeHelpText = `memento write

Usage:
  memento write [--overwrite] [--force-with-reason <reason>] <key>

Create, append to, or overwrite a vault note from stdin, then recompile the vault.

Key contract:
  key is a vault-relative .md path. Repo-relative paths and paths prefixed with the vault directory are invalid.

Flags:
  --overwrite                   Replace the full note body with stdin. Without this flag, write appends by default.
  --force-with-reason <reason>  Override a ratified mode rejection. Reason must be non-empty.

Mode interaction:
  append-only is the default when mode: is absent; ratified notes accept appends and reject overwrites.
  living accepts appends and overwrites.
  read-only rejects writes after ratification.
  Unratified notes are still in their edit window and accept appends and overwrites regardless of mode.

Stderr:
  On success, stderr includes wrote: <abs path> (<byte count>, <append|overwrite>) before the compile result.
  Forced writes also include forced: true and reason: <reason>.

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
