package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/setup"
	"github.com/tpisel/memento/internal/vault"
)

var version = "dev"

const helpText = `memento

Usage:
  memento help
  memento version
  memento compile [--dir <vault>] [--print]
  memento init [--dir <vault>]
  memento read [--dir <vault>] <key>
  memento write [--dir <vault>] <key>
  memento serve

Commands:
  help      Show this help text.
  version   Print the memento version.
  compile   Compile a memory vault manifest.
  init      Adopt or create a memory vault.
  read      Read a memory note.
  write     Create or append to a memory note from stdin.
  serve     Run the MCP server. Not implemented in this scaffold.
`

// Run dispatches the CLI and returns a process-style exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithInput(args, os.Stdin, stdout, stderr)
}

// RunWithInput dispatches the CLI using stdin for commands that consume a body.
func RunWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, helpText)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, helpText)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "memento %s\n", version)
		return 0
	case "compile":
		return runCompile(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "read":
		return runRead(args[1:], stdout, stderr)
	case "write":
		return runWrite(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "memento: unknown command %q\n\n", args[0])
		fmt.Fprint(stderr, "Run 'memento help' for usage.\n")
		return 2
	}
}

func runCompile(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("compile", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", "", "memory vault directory")
	printOnly := flags.Bool("print", false, "print manifest JSON to stdout")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "memento compile: unexpected argument %q\n", flags.Arg(0))
		return 2
	}

	v, err := compileVault(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "memento compile: %v\n", err)
		return 1
	}

	if *printOnly {
		m, err := manifest.Compile(v)
		if err != nil {
			fmt.Fprintf(stderr, "memento compile: %v\n", err)
			return 1
		}
		data, err := manifest.Marshal(m)
		if err != nil {
			fmt.Fprintf(stderr, "memento compile: %v\n", err)
			return 1
		}
		if _, err := stdout.Write(data); err != nil {
			fmt.Fprintf(stderr, "memento compile: write stdout: %v\n", err)
			return 1
		}
		return 0
	}

	if err := manifest.Write(v); err != nil {
		fmt.Fprintf(stderr, "memento compile: %v\n", err)
		return 1
	}
	return 0
}

func runInit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", "", "memory vault directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "memento init: unexpected argument %q\n", flags.Arg(0))
		return 2
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "memento init: get current directory: %v\n", err)
		return 1
	}

	v, err := setup.Init(wd, *dir)
	if err != nil {
		fmt.Fprintf(stderr, "memento init: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Initialized memento vault: %s\n", v.Root)
	return 0
}

func runRead(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("read", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", "", "memory vault directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprint(stderr, "memento read: expected exactly one key\n")
		return 2
	}

	v, err := resolveVault(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "memento read: %v\n", err)
		return 1
	}

	data, err := note.Read(v, flags.Arg(0))
	if err != nil {
		switch {
		case errors.Is(err, note.ErrInvalidKey):
			fmt.Fprintf(stderr, "memento read: %v\n", err)
		case errors.Is(err, note.ErrSectionNotFound):
			fmt.Fprintf(stderr, "memento read: section not found: %s\n", flags.Arg(0))
		case errors.Is(err, note.ErrNotFound):
			fmt.Fprintf(stderr, "memento read: key not found: %s\n", flags.Arg(0))
		default:
			fmt.Fprintf(stderr, "memento read: %v\n", err)
		}
		return 1
	}

	if _, err := stdout.Write(data); err != nil {
		fmt.Fprintf(stderr, "memento read: write stdout: %v\n", err)
		return 1
	}
	return 0
}

func runWrite(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("write", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", "", "memory vault directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprint(stderr, "memento write: expected exactly one key\n")
		return 2
	}

	v, err := resolveVault(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "memento write: %v\n", err)
		return 1
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "memento write: read stdin: %v\n", err)
		return 1
	}

	if err := note.Write(v, flags.Arg(0), data, note.WriteOptions{}); err != nil {
		switch {
		case errors.Is(err, note.ErrInvalidKey):
			fmt.Fprintf(stderr, "memento write: %v\n", err)
		case errors.Is(err, note.ErrUnsupportedWriteOperation):
			fmt.Fprintf(stderr, "memento write: %v\n", err)
		case errors.Is(err, note.ErrReadOnly):
			fmt.Fprintf(stderr, "memento write: %v\n", err)
		default:
			fmt.Fprintf(stderr, "memento write: %v\n", err)
		}
		return 1
	}
	return 0
}

func compileVault(dir string) (vault.Vault, error) {
	return resolveVault(dir)
}

func resolveVault(dir string) (vault.Vault, error) {
	if dir != "" {
		return vault.Open(dir)
	}

	wd, err := os.Getwd()
	if err != nil {
		return vault.Vault{}, fmt.Errorf("get current directory: %w", err)
	}
	return vault.Discover(wd)
}
