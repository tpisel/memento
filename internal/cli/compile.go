package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tpisel/memento/internal/brief"
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
	fmt.Fprintf(stderr, "compiled: %d entries\n", count)
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
