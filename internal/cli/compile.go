package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tpisel/memento/internal/brief"
	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/vault"
)

func runCompile(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("compile", flag.ContinueOnError)
	if ok, code := parseSubcommandFlags(flags, args, stdout, stderr, "compile", compileHelpText); !ok {
		return code
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

	warnings, count, err := writeCompileArtifacts(v)
	if err != nil {
		printCLIError(stderr, "compile", err)
		return 1
	}
	printCompileWarnings(stderr, warnings)

	// The compile half of the check-write↔compile handshake (ADR-0031): compare
	// what landed against the bytes-hash check-write gated, shout on mismatch,
	// then clear the ledger. This is the detective backstop under the predictive
	// gate and the only integrity signal in the absence of doctor — a ledger
	// failure is therefore surfaced loudly but never fails the compile itself,
	// whose coherence work (manifest/brief) has already succeeded.
	if err := reportPendingDrift(v, stderr); err != nil {
		fmt.Fprintf(stderr, "memento compile: warning: drift check: %v\n", err)
	}

	fmt.Fprintf(stderr, "compiled: %d entries\n", count)
	return 0
}

// reportPendingDrift runs the drift pass over the pending-write ledger: for each
// key check-write recorded, it hashes the bytes now on disk and compares them to
// the gated expectation. A mismatch (replay divergence, or the write never
// landing) raises a loud DRIFT ALARM on stderr naming the key. After the pass it
// clears the whole ledger so each expectation is verified exactly once and the
// alarm does not re-fire on the next compile.
func reportPendingDrift(v vault.Vault, stderr io.Writer) error {
	pending, err := enforce.LoadPending(v)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	for key, expected := range pending {
		path := filepath.Join(v.Root, filepath.FromSlash(key))
		data, err := os.ReadFile(path)
		var landed string
		switch {
		case err == nil:
			landed = enforce.HashBytes(data)
		case errors.Is(err, os.ErrNotExist):
			fmt.Fprintf(stderr, "memento compile: DRIFT ALARM: %s — the gated write was approved but no file landed on disk; the mode gate ran on bytes that are not there. Re-inspect this note.\n", key)
			continue
		default:
			return fmt.Errorf("read landed bytes for %s: %w", key, err)
		}
		if landed != expected {
			fmt.Fprintf(stderr, "memento compile: DRIFT ALARM: %s — the bytes on disk do not match the gated write check-write approved (expected %s, landed %s). The mode gate ran on bytes that differ from what landed; re-inspect this note.\n", key, expected, landed)
		}
	}
	return enforce.ClearPending(v)
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
	path := vault.BriefPath(v)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create brief directory: %w", err)
	}
	if err := os.WriteFile(path, brief.Render(m), 0o644); err != nil {
		return fmt.Errorf("write brief: %w", err)
	}
	return nil
}

func printCompileWarnings(stderr io.Writer, warnings []manifest.Warning) {
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "memento compile: warning: %v\n", warning)
	}
}
