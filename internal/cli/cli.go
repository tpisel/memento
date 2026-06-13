package cli

import (
	"encoding/json"
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
	"github.com/tpisel/memento/internal/setup"
	"github.com/tpisel/memento/internal/vault"
)

var version = "dev"

const helpText = `memento

Usage:
  memento help
  memento version
  memento brief [--dir <vault>]
  memento compile [--dir <vault>] [--print]
  memento init [--dir <vault>]
  memento read [--dir <vault>] <key|N>
  memento write [--dir <vault>] <key>
  memento serve

Commands:
  help      Show this help text.
  version   Print the memento version.
  brief     Print the agent-facing manifest projection.
  compile   Compile a memory vault manifest.
  init      Adopt or create a memory vault.
  read      Read a memory note.
  write     Create or append to a memory note from stdin.
  serve     MCP server (not implemented; see spec §13).
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
	case "read":
		return runRead(args[1:], stdout, stderr)
	case "write":
		return runWrite(args[1:], stdin, stdout, stderr)
	case "serve":
		fmt.Fprint(stderr, "memento serve: not implemented (v3; see spec §13)\n")
		return 1
	default:
		fmt.Fprintf(stderr, "memento: unknown command %q\n\n", args[0])
		fmt.Fprint(stderr, "Run 'memento help' for usage.\n")
		return 2
	}
}

func runBrief(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("brief", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", "", "memory vault directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "memento brief: unexpected argument %q\n", flags.Arg(0))
		return 2
	}

	v, err := resolveVault(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "memento brief: %v\n", err)
		return 1
	}

	data, err := readOrRenderBrief(v)
	if err != nil {
		fmt.Fprintf(stderr, "memento brief: %v\n", err)
		return 1
	}
	if _, err := stdout.Write(data); err != nil {
		fmt.Fprintf(stderr, "memento brief: write stdout: %v\n", err)
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

	data, err = os.ReadFile(v.ManifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("manifest missing at %s; run memento compile", v.ManifestPath)
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
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
		m, warnings, err := manifest.CompileWithWarnings(v)
		if err != nil {
			fmt.Fprintf(stderr, "memento compile: %v\n", err)
			return 1
		}
		printCompileWarnings(stderr, warnings)
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

	warnings, err := writeCompileArtifacts(v)
	if err != nil {
		fmt.Fprintf(stderr, "memento compile: %v\n", err)
		return 1
	}
	printCompileWarnings(stderr, warnings)
	return 0
}

func writeCompileArtifacts(v vault.Vault) ([]manifest.Warning, error) {
	m, warnings, err := manifest.CompileWithWarnings(v)
	if err != nil {
		return nil, err
	}

	data, err := manifest.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.MkdirAll(v.MarkerDir, 0o755); err != nil {
		return nil, fmt.Errorf("create marker directory: %w", err)
	}
	if err := os.WriteFile(v.ManifestPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	if err := writeBriefArtifact(v, m); err != nil {
		warnings = append(warnings, manifest.Warning{Path: filepath.ToSlash(filepath.Join(vault.ToolDirName, vault.BriefFileName)), Err: err})
	}
	return warnings, nil
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
		fmt.Fprint(stderr, "memento read: expected exactly one key or entry number\n")
		return 2
	}

	v, err := resolveVault(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "memento read: %v\n", err)
		return 1
	}

	target := flags.Arg(0)
	var data []byte
	if isAllDigits(target) {
		data, err = readNumberedEntry(v, target, stderr)
	} else {
		data, err = note.Read(v, target)
	}
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

func readNumberedEntry(v vault.Vault, target string, stderr io.Writer) ([]byte, error) {
	number, err := strconv.Atoi(target)
	if err != nil || number < 1 {
		return nil, fmt.Errorf("entry number must be 1 or greater: %s", target)
	}

	m, err := readManifest(v)
	if err != nil {
		return nil, err
	}

	numbered := brief.NumberedEntries(m)
	if number > len(numbered) {
		return nil, fmt.Errorf("entry %d does not exist in manifest; manifest has %d entries", number, len(numbered))
	}

	key := numbered[number-1].Entry.Key
	path, err := manifestEntryPath(v, key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("entry %d's file `%s` no longer exists.\nmanifest is stale; run: memento compile && memento brief\nnote: entry numbers will likely shift after compile.", number, key)
		}
		return nil, fmt.Errorf("read %s: %w", key, err)
	}

	warnIfBriefHashDrift(v, m, stderr)
	return data, nil
}

func readManifest(v vault.Vault) (manifest.Manifest, error) {
	data, err := os.ReadFile(v.ManifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manifest.Manifest{}, fmt.Errorf("manifest missing at %s; run memento compile", v.ManifestPath)
		}
		return manifest.Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest.Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return m, nil
}

func manifestEntryPath(v vault.Vault, key string) (string, error) {
	if key == "" || strings.Contains(key, "\\") || filepath.IsAbs(key) || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("manifest entry has invalid key: %s", key)
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("manifest entry has invalid key: %s", key)
		}
	}
	return filepath.Join(v.Root, filepath.FromSlash(key)), nil
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
	line := string(data)
	if before, _, ok := strings.Cut(line, "\n"); ok {
		line = before
	}

	const prefix = "<!-- manifest: "
	const suffix = " -->"
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix), true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
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
