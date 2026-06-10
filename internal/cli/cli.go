package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

var version = "dev"

const helpText = `memento

Usage:
  memento help
  memento version
  memento compile [--dir <vault>] [--print]
  memento init
  memento read <key>
  memento write <key>
  memento serve

Commands:
  help      Show this help text.
  version   Print the memento version.
  compile   Compile a memory vault manifest.
  init      Adopt or create a memory vault. Not implemented in this scaffold.
  read      Read a memory note. Not implemented in this scaffold.
  write     Write to a memory note. Not implemented in this scaffold.
  serve     Run the MCP server. Not implemented in this scaffold.
`

// Run dispatches the CLI and returns a process-style exit code.
func Run(args []string, stdout, stderr io.Writer) int {
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

func compileVault(dir string) (vault.Vault, error) {
	if dir != "" {
		return vault.Open(dir)
	}

	wd, err := os.Getwd()
	if err != nil {
		return vault.Vault{}, fmt.Errorf("get current directory: %w", err)
	}
	return vault.Discover(wd)
}
