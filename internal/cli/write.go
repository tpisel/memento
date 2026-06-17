package cli

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/vault"
)

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
	priorManifest := preWriteManifestSnapshot(v)

	data, err := io.ReadAll(stdin)
	if err != nil {
		printCLIError(stderr, "write", fmt.Errorf("%w: read stdin: %v", ErrIO, err))
		return 1
	}

	operation := note.OperationAppend
	if *overwrite {
		operation = note.OperationOverwrite
	}
	writtenPath, err := note.Write(v, flags.Arg(0), data, note.WriteOptions{Operation: operation})
	if err != nil {
		printCLIError(stderr, "write", err)
		return 1
	}
	fmt.Fprintf(stderr, "wrote: %s (%d, %s)\n", writtenPath, len(data), operation)
	if segment, ok := newTopLevelVaultDirSegment(v, writtenPath, priorManifest); ok {
		fmt.Fprintf(stderr, "warn: created new top-level vault directory '%s' — confirm this is intentional\n", segment)
	}
	warnings, count, err := writeCompileArtifactsAfterWrite(v)
	if err != nil {
		fmt.Fprintf(stderr, "memento write: warning: write succeeded but recompile failed; run 'memento compile' to refresh the manifest: %v\n", err)
		return 3
	}
	printCompileWarnings(stderr, warnings)
	fmt.Fprintf(stderr, "compiled: %d entries\n", count)
	return 0
}

func preWriteManifestSnapshot(v vault.Vault) manifest.Manifest {
	m, err := manifest.Load(v)
	if err != nil {
		return manifest.Manifest{}
	}
	return m
}

func newTopLevelVaultDirSegment(v vault.Vault, writtenPath string, prior manifest.Manifest) (string, bool) {
	rel, ok := resolvedVaultRelativePath(v, writtenPath)
	if !ok {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	segment, _, ok := strings.Cut(rel, "/")
	if !ok || segment == "" {
		return "", false
	}

	prefix := segment + "/"
	for _, entry := range prior.Entries {
		if strings.HasPrefix(entry.Key, prefix) {
			return "", false
		}
	}
	return segment, true
}

func resolvedVaultRelativePath(v vault.Vault, target string) (string, bool) {
	root, err := filepath.EvalSymlinks(v.Root)
	if err != nil {
		return "", false
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", false
	}
	root = filepath.Clean(root)

	rel, err := filepath.Rel(root, filepath.Clean(target))
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}
