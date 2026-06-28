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
	dir := flags.String("dir", "", "memory vault directory")
	agents := flags.String("agents", "detect", "agent hook integration selection")
	if ok, code := parseSubcommandFlags(flags, args, stdout, stderr, "init", initHelpText); !ok {
		return code
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

	agentSelection, err := setup.ParseAgentSelection(*agents)
	if err != nil {
		printCLIError(stderr, "init", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}

	v, err := setup.InitWithOptions(wd, *dir, setup.InitOptions{AgentSelection: agentSelection, NoticeWriter: stdout})
	if err != nil {
		printCLIError(stderr, "init", err)
		return 1
	}
	fmt.Fprintf(stdout, "memento: init complete: %s\n", v.Root)
	return 0
}
