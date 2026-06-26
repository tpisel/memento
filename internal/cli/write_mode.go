package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/markdown"
	"github.com/tpisel/memento/internal/note"
)

// runWriteMode durably rewrites a note's frontmatter mode: line (ADR-0031). It
// is the only path that may change an existing note's mode. Loosening toward
// living requires --justification; tightening toward read-only accepts it as
// optional self-documentation. Unknown mode values are rejected, not defaulted.
func runWriteMode(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("write-mode", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	justification := flags.String("justification", "", "reason for the mode change; required when loosening, optional when tightening")

	positional, ok, code := parseInterspersedFlags(flags, args, stdout, stderr, "write-mode", writeModeHelpText)
	if !ok {
		return code
	}
	if len(positional) != 2 {
		printCLIError(stderr, "write-mode", fmt.Errorf("%w: expected <key> <append-only|living|read-only>", ErrInvalidArguments))
		return 2
	}

	newMode, err := markdown.ParseWriteMode(positional[1])
	if err != nil {
		printCLIError(stderr, "write-mode", fmt.Errorf("%w: unknown mode %q; want append-only, living, or read-only", ErrInvalidArguments, positional[1]))
		return 2
	}
	reason := strings.TrimSpace(*justification)

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "write-mode", err)
		return 1
	}
	key, err := enforce.NormalizeWritableKey(v, positional[0])
	if err != nil {
		printCLIError(stderr, "write-mode", err)
		return 1
	}
	path, err := enforce.ResolveWritablePath(v, key)
	if err != nil {
		printCLIError(stderr, "write-mode", err)
		return 1
	}

	source, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		printCLIError(stderr, "write-mode", fmt.Errorf("%w: %s", note.ErrNotFound, key))
		return 1
	}
	if err != nil {
		printCLIError(stderr, "write-mode", fmt.Errorf("%w: read %s: %v", ErrIO, key, err))
		return 1
	}

	meta, err := markdown.ExtractMetadata(key, source)
	if err != nil {
		printCLIError(stderr, "write-mode", err)
		return 1
	}
	oldMode := meta.Mode

	if modeRank(newMode) > modeRank(oldMode) && reason == "" {
		printCLIError(stderr, "write-mode", fmt.Errorf("%w: loosening %s from %s to %s requires --justification", ErrInvalidArguments, key, oldMode, newMode))
		return 2
	}

	if err := os.WriteFile(path, markdown.SetMode(source, newMode), 0o644); err != nil {
		printCLIError(stderr, "write-mode", fmt.Errorf("%w: write %s: %v", ErrIO, key, err))
		return 1
	}

	fmt.Fprintf(stderr, "mode: %s %s -> %s\n", key, oldMode, newMode)
	if reason != "" {
		fmt.Fprintf(stderr, "justification: %s\n", reason)
	}

	warnings, count, err := writeCompileArtifacts(v)
	if err != nil {
		fmt.Fprintf(stderr, "memento write-mode: warning: mode change succeeded but recompile failed; run 'memento compile' to refresh the manifest: %v\n", err)
		return 3
	}
	printCompileWarnings(stderr, warnings)
	fmt.Fprintf(stderr, "compiled: %d entries\n", count)
	return 0
}

// modeRank orders the lattice from most to least restrictive so a higher rank
// means looser (read-only < append-only < living). The default arm matches
// ExtractMetadata's append-only default for a note with no mode: line.
func modeRank(m markdown.WriteMode) int {
	switch m {
	case markdown.ModeReadOnly:
		return 0
	case markdown.ModeLiving:
		return 2
	default:
		return 1
	}
}

// parseInterspersedFlags parses args allowing flags to appear before or after
// positionals (the std flag package stops at the first non-flag), returning the
// collected positionals. It mirrors parseSubcommandFlags' help and error
// handling so write-mode's --justification can trail its <key> <mode> as the
// documented usage shows.
func parseInterspersedFlags(flags *flag.FlagSet, args []string, stdout, stderr io.Writer, verb, helpText string) ([]string, bool, int) {
	var positional []string
	rest := args
	for {
		if err := flags.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				fmt.Fprint(stdout, helpText)
				return nil, false, 0
			}
			printCLIError(stderr, verb, fmt.Errorf("%w: %v", ErrInvalidArguments, err))
			return nil, false, 2
		}
		if flags.NArg() == 0 {
			return positional, true, 0
		}
		positional = append(positional, flags.Arg(0))
		rest = flags.Args()[1:]
	}
}
