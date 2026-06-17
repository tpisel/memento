package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tpisel/memento/internal/setup"
)

func runInit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dir := flags.String("dir", "", "memory vault directory")
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "init", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 0 {
		printCLIError(stderr, "init", fmt.Errorf("%w: unexpected argument %q", ErrInvalidArguments, flags.Arg(0)))
		return 2
	}

	wd, err := os.Getwd()
	if err != nil {
		printCLIError(stderr, "init", fmt.Errorf("%w: get current directory: %v", ErrIO, err))
		return 1
	}

	v, err := setup.Init(wd, *dir)
	if err != nil {
		printCLIError(stderr, "init", err)
		return 1
	}
	fmt.Fprintf(stdout, "Initialized memento vault: %s\n", v.Root)
	return 0
}
