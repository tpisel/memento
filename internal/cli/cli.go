package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tpisel/memento/internal/brief"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/orient"
	"github.com/tpisel/memento/internal/setup"
	"github.com/tpisel/memento/internal/vault"
)

var version = "dev"

var writeCompileArtifactsAfterWrite = writeCompileArtifacts

const helpText = `memento

Usage:
  memento help
  memento version
  memento brief
  memento compile
  memento init [--dir <vault>]
  memento orient
  memento read <key|@N>
  memento write [--overwrite] <key>

Commands:
  help      Show this help text.
  version   Print the memento version.
  brief     Print the agent-facing manifest projection.
  compile   Compile a memory vault manifest.
  init      Adopt or create a memory vault.
  orient    Print tool-usage orientation and project overlays.
  read      Read a memory note by key or @N entry reference.
  write     Create, append to, or overwrite a memory note from stdin, then compile.
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
	case "orient":
		return runOrient(args[1:], stdout, stderr)
	case "read":
		return runRead(args[1:], stdout, stderr)
	case "write":
		return runWrite(args[1:], stdin, stdout, stderr)
	default:
		printRootError(stderr, fmt.Errorf("%w %q", ErrUnknownCommand, args[0]))
		return 2
	}
}

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

func runBrief(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("brief", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "brief", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
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

func runCompile(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("compile", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "compile", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 0 {
		printCLIError(stderr, "compile", fmt.Errorf("%w: unexpected argument %q", ErrInvalidArguments, flags.Arg(0)))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "compile", err)
		return 1
	}

	warnings, _, err := writeCompileArtifacts(v)
	if err != nil {
		printCLIError(stderr, "compile", err)
		return 1
	}
	printCompileWarnings(stderr, warnings)
	return 0
}

func writeCompileArtifacts(v vault.Vault) ([]manifest.Warning, int, error) {
	m, warnings, err := manifest.CompileWithWarnings(v)
	if err != nil {
		return nil, 0, err
	}

	data, err := manifest.Marshal(m)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.MkdirAll(v.MarkerDir, 0o755); err != nil {
		return nil, 0, fmt.Errorf("create marker directory: %w", err)
	}
	if err := os.WriteFile(v.ManifestPath, data, 0o644); err != nil {
		return nil, 0, fmt.Errorf("write manifest: %w", err)
	}

	if err := writeBriefArtifact(v, m); err != nil {
		warnings = append(warnings, manifest.Warning{Path: filepath.ToSlash(filepath.Join(vault.ToolDirName, vault.BriefFileName)), Err: err})
	}
	return warnings, len(m.Entries), nil
}

func writeBriefArtifact(v vault.Vault, m manifest.Manifest) error {
	toolFiles, err := brief.DetectToolFiles(v)
	if err != nil {
		return err
	}
	path := vault.BriefPath(v)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create brief directory: %w", err)
	}
	if err := os.WriteFile(path, brief.RenderWithToolFiles(m, toolFiles), 0o644); err != nil {
		return fmt.Errorf("write brief: %w", err)
	}
	return nil
}

func printCompileWarnings(stderr io.Writer, warnings []manifest.Warning) {
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "memento compile: warning: %v\n", warning)
	}
}

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

func runRead(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("read", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "read", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 1 {
		printCLIError(stderr, "read", fmt.Errorf("%w: expected exactly one key or @N entry reference", ErrInvalidArguments))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "read", err)
		return 1
	}

	target := flags.Arg(0)
	var data []byte
	var key string
	var numberedManifest manifest.Manifest
	if strings.HasPrefix(target, "@") {
		data, key, numberedManifest, err = readNumberedEntry(v, strings.TrimPrefix(target, "@"))
	} else {
		data, err = note.Read(v, target)
		key = target
	}
	if err != nil {
		printCLIError(stderr, "read", err)
		return 1
	}

	binding, err := note.BindingForReadTarget(v, key)
	if err != nil {
		printCLIError(stderr, "read", err)
		return 1
	}
	fmt.Fprintf(stderr, "binding: %s\n", binding)
	if strings.HasPrefix(target, "@") {
		warnIfBriefHashDrift(v, numberedManifest, stderr)
	}

	if _, err := stdout.Write(data); err != nil {
		printCLIError(stderr, "read", fmt.Errorf("%w: write stdout: %v", ErrIO, err))
		return 1
	}
	return 0
}

func readNumberedEntry(v vault.Vault, target string) ([]byte, string, manifest.Manifest, error) {
	number, err := strconv.Atoi(target)
	if err != nil {
		return nil, "", manifest.Manifest{}, fmt.Errorf("%w: entry reference must be @ followed by a number: @%s", ErrInvalidEntryReference, target)
	}
	if number < 1 {
		return nil, "", manifest.Manifest{}, fmt.Errorf("%w: entry number must be 1 or greater: @%s", ErrNumericOutOfRange, target)
	}

	m, err := readManifest(v)
	if err != nil {
		return nil, "", manifest.Manifest{}, err
	}

	numbered := brief.NumberedEntries(m)
	if number > len(numbered) {
		return nil, "", manifest.Manifest{}, fmt.Errorf("%w: entry %d does not exist in manifest; manifest has %d entries", ErrNumericOutOfRange, number, len(numbered))
	}

	key := numbered[number-1].Entry.Key
	data, err := note.Read(v, key)
	if err != nil {
		if errors.Is(err, note.ErrNotFound) {
			return nil, "", manifest.Manifest{}, fmt.Errorf("%w: entry %d's file `%s` no longer exists", manifest.ErrStale, number, key)
		}
		return nil, "", manifest.Manifest{}, err
	}

	return data, key, m, nil
}

func readManifest(v vault.Vault) (manifest.Manifest, error) {
	return manifest.Load(v)
}

func warnIfBriefHashDrift(v vault.Vault, m manifest.Manifest, stderr io.Writer) {
	data, err := os.ReadFile(vault.BriefPath(v))
	if err != nil {
		return
	}

	briefHash, ok := briefManifestHash(data)
	if !ok || briefHash == brief.ManifestHash(m) {
		return
	}
	fmt.Fprintln(stderr, "warn: manifest changed since last brief, numbers may not match your view — re-run memento brief.")
}

func briefManifestHash(data []byte) (string, bool) {
	text := string(data)
	line, rest, ok := strings.Cut(text, "\n")
	if !ok || strings.TrimSuffix(line, "\r") != "---" {
		return "", false
	}

	for {
		line, next, hasNext := strings.Cut(rest, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "---" {
			return "", false
		}
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "manifest" {
			return strings.TrimSpace(value), true
		}
		if !hasNext {
			return "", false
		}
		rest = next
	}
}

func runWrite(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("write", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	overwrite := flags.Bool("overwrite", false, "replace the target note with stdin instead of appending")
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "write", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 1 {
		printCLIError(stderr, "write", fmt.Errorf("%w: expected exactly one key", ErrInvalidArguments))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "write", err)
		return 1
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		printCLIError(stderr, "write", fmt.Errorf("%w: read stdin: %v", ErrIO, err))
		return 1
	}

	opts := note.WriteOptions{}
	if *overwrite {
		opts.Operation = note.OperationOverwrite
	}
	if err := note.Write(v, flags.Arg(0), data, opts); err != nil {
		printCLIError(stderr, "write", err)
		return 1
	}
	warnings, count, err := writeCompileArtifactsAfterWrite(v)
	if err != nil {
		fmt.Fprintf(stderr, "memento write: warning: recompile failed after successful write: %v\n", err)
		return 1
	}
	printCompileWarnings(stderr, warnings)
	fmt.Fprintf(stderr, "compiled: %d entries\n", count)
	return 0
}

func resolveVault() (vault.Vault, error) {
	wd, err := os.Getwd()
	if err != nil {
		return vault.Vault{}, fmt.Errorf("get current directory: %w", err)
	}
	return vault.Discover(wd)
}
