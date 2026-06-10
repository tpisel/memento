package cli

import (
	"fmt"
	"io"
)

var version = "dev"

const helpText = `memento

Usage:
  memento help
  memento version
  memento compile
  memento init
  memento read <key>
  memento write <key>
  memento serve

Commands:
  help      Show this help text.
  version   Print the memento version.
  compile   Compile a memory vault manifest. Not implemented in this scaffold.
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
	default:
		fmt.Fprintf(stderr, "memento: unknown command %q\n\n", args[0])
		fmt.Fprint(stderr, "Run 'memento help' for usage.\n")
		return 2
	}
}
