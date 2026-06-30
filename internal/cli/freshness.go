package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tpisel/memento/internal/vault"
)

// errStaleNote stops the freshness stat-walk at the first note that out-paces the
// manifest — once one is found the answer is decided, so there is no reason to
// stat the rest of the corpus.
var errStaleNote = errors.New("stale note found")

// ensureBriefFresh lazily recompiles the manifest and brief when the vault has
// out-paced them. The PostToolUse check-write/compile hook only fires on the
// agent's own tool writes, so the most common drift — a human editing a note
// while the agent works — produces no recompile, and the manifest/brief would
// otherwise silently lag until the next explicit compile, SessionStart, or
// pre-commit. This gate closes that window on brief, the verb an agent hits at
// task start.
//
// It runs a cheap stat-only pass first and recompiles only when stale, so the
// common (already-fresh) case costs O(n) os.Stat calls and no body reads. When it
// does recompile it calls writeCompileArtifacts — the same coherence work compile
// does, but WITHOUT the DRIFT ALARM / MODE VIOLATION audits. Those integrity
// signals stay on the explicit compile, PostToolUse, and pre-commit paths so they
// are never silently absorbed by a read-side verb. Compile warnings are discarded
// here: brief keeps a clean stdout-only contract, and malformed-note warnings
// still surface on explicit compile and the hooks.
func ensureBriefFresh(v vault.Vault) error {
	info, err := os.Stat(v.ManifestPath)
	if errors.Is(err, os.ErrNotExist) {
		// No compiled manifest yet — a bootstrap concern, not freshness. Leave it to
		// readOrRenderBrief, which renders from an existing manifest or returns the
		// manifest-not-found compile hint. This gate only catches an EXISTING
		// manifest the vault has out-paced.
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat manifest: %w", err)
	}

	stale, err := vaultOutpacedManifest(v, info.ModTime())
	if err != nil {
		return err
	}
	if !stale {
		return nil
	}
	_, _, err = writeCompileArtifacts(v)
	return err
}

// vaultOutpacedManifest reports whether any markdown note's mtime is strictly
// newer than manifestModTime. It stat-walks the note corpus (no body reads) via
// WalkMarkdown and short-circuits at the first newer note. Equal mtimes count as
// fresh: the manifest is written after the notes it indexes, so coarse-resolution
// ties should not force a needless recompile. The check is best-effort — it
// inherits the filesystem-mtime caveats spec §9 flags for the body-hash trigger,
// and the authoritative refresh remains explicit compile plus the hooks.
func vaultOutpacedManifest(v vault.Vault, manifestModTime time.Time) (bool, error) {
	walkErr := vault.WalkMarkdown(v, func(relPath, absPath string) error {
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("stat note %s: %w", relPath, err)
		}
		if info.ModTime().After(manifestModTime) {
			return errStaleNote
		}
		return nil
	})
	if errors.Is(walkErr, errStaleNote) {
		return true, nil
	}
	if walkErr != nil {
		return false, walkErr
	}
	return false, nil
}
