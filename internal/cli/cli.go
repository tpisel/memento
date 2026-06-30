package cli

import (
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
  memento init [--dir <vault>] [--agents detect|none|claude,codex]
  memento doctor [--session]
  memento compile
  memento brief
  memento orient
  memento read <key|@N>
  memento convention <name>
  memento write-mode <key> <append-only|living|read-only> [--justification <reason>]
  memento unlock <key> --justification <reason>

Commands:
  help        Show this help text.
  version     Print the memento version.
  init        Adopt or create a memory vault.
  doctor      Report whether vault write enforcement is live.
  compile     Compile a memory vault manifest.
  brief       Print the agent-facing manifest projection.
  orient      Print tool-usage orientation and project overlays.
  read        Read a memory note by key or @N entry reference.
  convention  Read an operational convention by name from _memento/conventions.
  write-mode  Durably change a note's frontmatter mode, then compile.
  unlock      Temporarily re-open a read-only note's edit window until the next commit.
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
	case "brief":
		return runBrief(args[1:], stdout, stderr)
	case "compile":
		return runCompile(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "orient":
		return runOrient(args[1:], stdout, stderr)
	case "read":
		return runRead(args[1:], stdout, stderr)
	case "convention":
		return runConvention(args[1:], stdout, stderr)
	case "write-mode":
		return runWriteMode(args[1:], stdout, stderr)
	case "unlock":
		return runUnlock(args[1:], stdout, stderr)
	case "check-write":
		// Hook plumbing (ADR-0031), deliberately absent from help: the
		// PreToolUse verdict engine, fed the raw payload on stdin.
		return runCheckWrite(stdin, stdout, stderr)
	case "schema":
		// Doctor plumbing (ADR-0032), deliberately absent from help: prints the
		// manifest schema version THIS binary supports, so a doctor running elsewhere
		// can query the schema of the binary the gate shells to (binary-schema-compatible)
		// rather than assuming it equals doctor's own compiled-in version.
		fmt.Fprintln(stdout, manifest.CurrentSchemaVersion)
		return 0
	case "clear-grants":
		// Hook plumbing (ADR-0031), deliberately absent from help: the pre-commit
		// step that drops all unlock grants (the "any commit re-locks" semantics),
		// run after `memento compile` so its audit still sees the grants first.
		return runClearGrants(args[1:], stdout, stderr)
	default:
		printRootError(stderr, fmt.Errorf("%w %q", ErrUnknownCommand, args[0]))
		return 2
	}
}

func readManifest(v vault.Vault) (manifest.Manifest, error) {
	return manifest.Load(v)
}

func resolveVault() (vault.Vault, error) {
	wd, err := os.Getwd()
	if err != nil {
		return vault.Vault{}, fmt.Errorf("get current directory: %w", err)
	}
	return vault.Discover(wd)
}
