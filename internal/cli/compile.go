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
	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/vault"
)

// strictCommitEnv, when set to a truthy value, flips the ratification-boundary
// mode audit from DETECTION (loud alarm, exit 0) to MITIGATION (exit non-zero so
// the git pre-commit hook blocks the commit). Default off (ADR-0031): nothing is
// ratified pre-commit, so an unexpected hard failure would be more surprising than
// the loud alarm, and the alarm alone is the right signal for an honest agent.
const strictCommitEnv = "MEMENTO_STRICT_COMMIT"

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

	// The ratification-boundary mode audit (ADR-0031): the path-agnostic backstop
	// under the PreToolUse check-write gate. It compares the on-disk diff against
	// ratified (git HEAD) state, so it catches ungated mode violations whatever
	// write path produced them. Like the drift alarm it is best-effort detection —
	// an audit error never fails the compile, whose coherence work has succeeded.
	violations, err := reportModeViolations(v, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "memento compile: warning: mode audit: %v\n", err)
	}

	fmt.Fprintf(stderr, "compiled: %d entries\n", count)
	if violations > 0 && strictCommit() {
		// MITIGATION mode: a non-zero exit makes the pre-commit hook abort the commit,
		// holding the unauthorised change out of ratified state. The composed hook block
		// self-propagates this exit (`memento compile || exit $?`), so the mitigation no
		// longer depends on the host hook's `set -e` — it survives composition into a
		// foreign hook that lacks it (memento-5dn).
		return 1
	}
	return 0
}

// strictCommit reports whether the optional commit-blocking mitigation is enabled
// via the strictCommitEnv environment variable. Any value other than empty, "0",
// "false", "no", or "off" (case-insensitive) is treated as on. It shares the parser
// with MEMENTO_DOCTOR_STRICT (envFlagEnabled) so the two strict surfaces cannot drift.
func strictCommit() bool {
	return envFlagEnabled(strictCommitEnv)
}

// reportModeViolations runs the ratification-boundary diff audit: for each
// ratified note changed on disk vs HEAD it recomputes the pure prefix invariant
// with old=HEAD bytes (enforce.AuditRatifiedChange), honouring active unlock
// grants and legitimate write-mode changes, and raises a loud MODE VIOLATION on
// stderr for any ungated violation. Brand-new notes (absent at HEAD), non-note
// paths, and compile's own operational rewrites (manifest/brief) are excluded.
// It returns the number of violations so the caller can decide whether to block
// the commit (MEMENTO_STRICT_COMMIT). The MODE VIOLATION token is a NEW alarm
// class, separate from the gated-handshake DRIFT ALARM above.
func reportModeViolations(v vault.Vault, stderr io.Writer) (int, error) {
	changed, err := note.ChangedNotesVsHead(v)
	if err != nil {
		return 0, err
	}
	if len(changed) == 0 {
		return 0, nil
	}
	// Read the grant sidecar once, BEFORE `memento clear-grants` clears it (it runs
	// later in the same pre-commit hook, after this compile), so grant-covered
	// changes are correctly waived.
	grants, err := enforce.LoadGrants(v)
	if err != nil {
		return 0, err
	}

	violations := 0
	for _, key := range changed {
		normKey, err := enforce.NormalizeWritableKey(v, key)
		if err != nil {
			// Not a writable note — gitignored, operational, non-.md, or compile's own
			// manifest/brief output. Excluded from the audit by construction.
			continue
		}
		head, atHead, err := note.HeadBytes(v, normKey)
		if err != nil {
			return violations, err
		}
		if !atHead {
			continue // brand-new note: birth on disk, not a ratified-mode violation
		}
		disk, err := readDiskBytesForAudit(v, normKey)
		if err != nil {
			return violations, err
		}
		_, granted := grants[normKey]
		if d := enforce.AuditRatifiedChange(normKey, head, disk, granted); !d.Allow {
			violations++
			fmt.Fprintf(stderr,
				"memento compile: MODE VIOLATION: %s — %s (ungated; not covered by a grant). "+
					"The on-disk change breaks this note's ratified mode without passing the gate; loosening a mode needs the user's explicit say-so (memento write-mode / memento unlock). Re-inspect this note.\n",
				normKey, modeViolationReason(d.ReasonCode))
		}
	}
	return violations, nil
}

// readDiskBytesForAudit reads the current on-disk bytes for an audited note. A
// note deleted on disk (the file is gone but it changed vs HEAD) reads as empty
// bytes, so the prefix invariant treats deletion of a read-only/append-only note
// as the violation it is, while a living note's deletion stays allowed.
func readDiskBytesForAudit(v vault.Vault, key string) ([]byte, error) {
	path := filepath.Join(v.Root, filepath.FromSlash(key))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read disk bytes for %s: %w", key, err)
	}
	return data, nil
}

// modeViolationReason renders a short human cause for a MODE VIOLATION from the
// prefix-invariant reason code, so the alarm names what broke without dumping the
// gate's full multi-line recovery message.
func modeViolationReason(reasonCode string) string {
	switch reasonCode {
	case enforce.ReasonReadOnly:
		return "read-only note's ratified content was changed on disk"
	default:
		// append_only_interior / append_only_overwrite — the prefix was broken.
		return "append-only note's ratified content was dropped or rewritten (not a pure append)"
	}
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
