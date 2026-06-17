package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/tpisel/memento/internal/orient"
)

func runOrient(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("orient", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "orient", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 0 {
		printCLIError(stderr, "orient", fmt.Errorf("%w: unexpected argument %q", ErrInvalidArguments, flags.Arg(0)))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "orient", err)
		return 1
	}

	m, err := readManifest(v)
	if err != nil {
		printCLIError(stderr, "orient", err)
		return 1
	}
	data, err := orient.Render(v, m)
	if err != nil {
		printCLIError(stderr, "orient", err)
		return 1
	}
	if _, err := stdout.Write(data); err != nil {
		printCLIError(stderr, "orient", fmt.Errorf("%w: write stdout: %v", ErrIO, err))
		return 1
	}
	return 0
}
