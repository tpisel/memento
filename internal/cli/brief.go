package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

func runBrief(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("brief", flag.ContinueOnError)
	if ok, code := parseSubcommandFlags(flags, args, stdout, stderr, "brief", briefHelpText); !ok {
		return code
	}
	if flags.NArg() != 0 {
		printCLIError(stderr, "brief", fmt.Errorf("%w: unexpected argument %q", ErrInvalidArguments, flags.Arg(0)))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "brief", err)
		return 1
	}

	data, err := readOrRenderBrief(v)
	if err != nil {
		printCLIError(stderr, "brief", err)
		return 1
	}
	if _, err := stdout.Write(data); err != nil {
		printCLIError(stderr, "brief", fmt.Errorf("%w: write stdout: %v", ErrIO, err))
		return 1
	}
	return 0
}

func readOrRenderBrief(v vault.Vault) ([]byte, error) {
	path := vault.BriefPath(v)
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read brief: %w", err)
	}

	m, err := manifest.Load(v)
	if err != nil {
		return nil, err
	}
	if err := writeBriefArtifact(v, m); err != nil {
		return nil, err
	}

	data, err = os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rendered brief: %w", err)
	}
	return data, nil
}
