package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/tpisel/memento/internal/convention"
)

func runConvention(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("convention", flag.ContinueOnError)
	if ok, code := parseSubcommandFlags(flags, args, stdout, stderr, "convention", conventionHelpText); !ok {
		return code
	}
	if flags.NArg() != 1 {
		printCLIError(stderr, "convention", fmt.Errorf("%w: expected exactly one convention name", ErrInvalidArguments))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "convention", err)
		return 1
	}

	c, err := convention.Read(v, flags.Arg(0))
	if err != nil {
		printCLIError(stderr, "convention", err)
		return 1
	}

	if _, err := stdout.Write(c.Body); err != nil {
		printCLIError(stderr, "convention", fmt.Errorf("%w: write stdout: %v", ErrIO, err))
		return 1
	}
	return 0
}
