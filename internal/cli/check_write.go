package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/markdown"
	"github.com/tpisel/memento/internal/vault"
)

// Resolution-stage reason codes, completing ADR-0031's denial-UX taxonomy
// alongside the content-invariant codes in internal/enforce. unwritable_path
// fires when an in-vault target is not a writable note; vault_discovery_ambiguous
// is the one verdict that asks rather than denies.
const (
	reasonUnwritablePath          = "unwritable_path"
	reasonVaultDiscoveryAmbiguous = "vault_discovery_ambiguous"
)

// errUnsupportedDerivation marks a write tool whose new-bytes deriveNewBytes does
// not handle — a guard against a future dispatch wiring a tool here without a
// derivation. For an in-vault target it surfaces as an internal error so the
// dumb-pipe wrapper fails closed (ADR-0031) rather than allowing an ungated write.
var errUnsupportedDerivation = errors.New("unsupported write derivation")

// preToolUse is the slice of the raw PreToolUse payload check-write consumes.
// tool_input is kept raw so each tool can decode the shape it needs: codex's
// apply_patch carries the envelope under an untyped key (and may even be a bare
// string), which a fixed struct could not unmarshal without erroring the whole
// payload (codex hooks contract / ADR-0031).
type preToolUse struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// fileToolInput is the file-write slice of tool_input: the inputs the
// Write/Edit/MultiEdit derivations need, plus Bash's command. Unknown fields are
// ignored.
type fileToolInput struct {
	FilePath string `json:"file_path"`
	// Write derivation.
	Content string `json:"content"`
	// Edit derivation (a single-edit MultiEdit).
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
	// MultiEdit derivation: edits applied in order.
	Edits []editInput `json:"edits"`
	// Bash classifier: the shell command, not a file_path.
	Command string `json:"command"`
}

// editInput is one element of a MultiEdit payload's edits array.
type editInput struct {
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

type hookOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
	ReasonCode         string             `json:"reason_code,omitempty"`
}

type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

// runCheckWrite is the hook-facing verdict engine (ADR-0031, hidden from help).
// It reads the raw PreToolUse payload on stdin and dispatches by tool: Write /
// Edit / MultiEdit derive concrete new-bytes against a file_path; Bash is
// classified against the deny-unless-provably-append rule. Tools outside that set
// are inert (exit 0, no output) so normal permission flow governs the rest of the
// repo. Internal failures exit non-zero for the wrapper to fail closed.
func runCheckWrite(stdin io.Reader, stdout, stderr io.Writer) int {
	payload, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: read payload: %v\n", err)
		return 1
	}
	var p preToolUse
	if err := json.Unmarshal(payload, &p); err != nil {
		fmt.Fprintf(stderr, "memento check-write: parse PreToolUse payload: %v\n", err)
		return 1
	}

	switch p.ToolName {
	case "Write", "Edit", "MultiEdit":
		var ti fileToolInput
		if err := json.Unmarshal(p.ToolInput, &ti); err != nil || ti.FilePath == "" {
			return 0
		}
		return checkWriteFile(p.ToolName, ti, stdout, stderr)
	case "Bash":
		var ti fileToolInput
		if err := json.Unmarshal(p.ToolInput, &ti); err != nil || ti.Command == "" {
			return 0
		}
		return checkWriteBash(ti.Command, stdout, stderr)
	case "apply_patch":
		patchText, ok := findPatchEnvelope(p.ToolInput)
		if !ok {
			// An apply_patch call whose payload carries no recognisable envelope is
			// anomalous: we cannot derive its targets, so fail closed (ADR-0031)
			// rather than wave through a write we cannot see.
			fmt.Fprintf(stderr, "memento check-write: apply_patch payload carried no recognisable patch envelope\n")
			return 1
		}
		return checkWriteApplyPatch(patchText, stdout, stderr)
	default:
		return 0
	}
}

// resolveVaultForCheck resolves the active vault for a verdict, mapping discovery
// outcomes onto the shared posture: no vault ⇒ inert (the rest of the repo is not
// ours to guard); multiple vaults ⇒ an `ask` verdict naming MEMENTO_VAULT_ROOT;
// any other error ⇒ fail closed (non-zero). ok is false when the caller should
// stop with the returned exit code. target names the write subject for the
// ambiguity message.
func resolveVaultForCheck(stdout, stderr io.Writer, target string) (vault.Vault, bool, int) {
	v, err := resolveVault()
	if err != nil {
		if errors.Is(err, vault.ErrVaultNotFound) {
			return vault.Vault{}, false, 0 // no vault to guard
		}
		if errors.Is(err, vault.ErrMultipleVaults) {
			emitVerdict(stdout, "ask", reasonVaultDiscoveryAmbiguous, fmt.Sprintf(
				"memento found more than one .memento vault, so it cannot tell which one %s belongs to. "+
					"Set MEMENTO_VAULT_ROOT to the intended vault root, then retry.", target))
			return vault.Vault{}, false, 0
		}
		fmt.Fprintf(stderr, "memento check-write: resolve vault: %v\n", err)
		return vault.Vault{}, false, 1
	}
	return v, true, 0
}

// checkWriteFile gives the verdict for a file-targeted write tool (Write / Edit /
// MultiEdit): it resolves the target to a vault-relative key, reads old-bytes
// from disk + ratification from git + active unlock grants itself, derives
// new-bytes, evaluates the prefix invariant, and emits the harness verdict.
// Writes outside the vault are inert.
func checkWriteFile(toolName string, ti fileToolInput, stdout, stderr io.Writer) int {
	v, ok, code := resolveVaultForCheck(stdout, stderr, fmt.Sprintf("%q", ti.FilePath))
	if !ok {
		return code
	}

	key, inVault, err := vaultRelativeKey(v, ti.FilePath)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: resolve target: %v\n", err)
		return 1
	}
	if !inVault {
		return 0
	}

	return gateVaultWrite(v, key, toolName, brokenPrefixReason(toolName), true, stdout, stderr,
		func(old []byte, exists bool) ([]byte, error) { return deriveNewBytes(toolName, ti, old, exists) })
}

// gateVaultWrite is the shared in-vault mode gate for a write already resolved to
// a vault-relative key. It rejects non-writable keys (unwritable_path), reads
// old-bytes + ratification + active grants, derives new-bytes via derive (whose
// error fails the verdict closed), and emits the prefix-invariant verdict.
// deriveLabel names the operation in the fail-closed message; brokenReason selects
// the append-only denial flavour. recordPending records the expected post-write
// bytes-hash into the handshake ledger on an allow — true only when newBytes is
// the exact bytes that will land (Write/Edit/MultiEdit); a Bash append models a
// synthetic suffix, so it passes false and leaves no drift expectation.
func gateVaultWrite(v vault.Vault, key, deriveLabel, brokenReason string, recordPending bool, stdout, stderr io.Writer, derive func(old []byte, exists bool) ([]byte, error)) int {
	verdict, err := computeVaultWriteVerdict(v, key, deriveLabel, brokenReason, derive, stderr)
	if err != nil {
		return 1 // fail-closed; computeVaultWriteVerdict already wrote stderr
	}
	if verdict.decision == "allow" && recordPending {
		// Record the bytes we expect to land so the PostToolUse compile can detect
		// a replay/derivation divergence (ADR-0031: the detective backstop under the
		// predictive gate). A ledger-write failure must not flip an allow into a
		// deny — the gate's verdict stands; the handshake is best-effort detection,
		// so it degrades to a missed drift check, surfaced on stderr.
		if err := enforce.RecordPending(v, verdict.normKey, enforce.HashBytes(verdict.newBytes)); err != nil {
			fmt.Fprintf(stderr, "memento check-write: record pending write for %s: %v\n", verdict.normKey, err)
		}
	}
	emitVerdict(stdout, verdict.decision, verdict.reasonCode, verdict.message)
	return 0
}

// vaultWriteVerdict is the outcome of evaluating one resolved in-vault write:
// the harness decision and, on an allow, the normalized key and derived bytes the
// caller may record into the drift-detection ledger.
type vaultWriteVerdict struct {
	decision   string
	reasonCode string
	message    string
	normKey    string
	newBytes   []byte
}

// computeVaultWriteVerdict evaluates a write already resolved to a vault-relative
// key without emitting or recording anything, so callers that gate several writes
// in one tool call (codex apply_patch) can stop on the first denial. It rejects
// non-writable keys (unwritable_path), reads old-bytes + ratification + active
// grants, derives new-bytes via derive (whose error fails closed via a returned
// error), and applies the drive-by then prefix-invariant verdict. deriveLabel
// names the operation in the fail-closed stderr message; brokenReason selects the
// append-only denial flavour.
func computeVaultWriteVerdict(v vault.Vault, key, deriveLabel, brokenReason string, derive func(old []byte, exists bool) ([]byte, error), stderr io.Writer) (vaultWriteVerdict, error) {
	normKey, err := enforce.NormalizeWritableKey(v, key)
	if err != nil {
		return vaultWriteVerdict{decision: "deny", reasonCode: reasonUnwritablePath, message: fmt.Sprintf(
			"%s is not a writable memento note — it is git-ignored, operational, or not a .md file — so this write is denied and the identical write will be denied again. "+
				"Pick a different .md key under the vault.", key)}, nil
	}
	path, err := enforce.ResolveWritablePath(v, normKey)
	if err != nil {
		return vaultWriteVerdict{decision: "deny", reasonCode: reasonUnwritablePath, message: fmt.Sprintf(
			"%s does not name a writable memento note (it resolves to a directory, a symlink, or outside the vault), so this write is denied and the identical write will be denied again. "+
				"Pick a different .md key under the vault.", normKey)}, nil
	}

	old, exists, err := readOldBytes(path)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: read %s: %v\n", normKey, err)
		return vaultWriteVerdict{}, err
	}

	newBytes, err := derive(old, exists)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: cannot derive new bytes for %s: %v\n", deriveLabel, err)
		return vaultWriteVerdict{}, err
	}

	ratified, err := enforce.IsRatified(v, normKey)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: ratification for %s: %v\n", normKey, err)
		return vaultWriteVerdict{}, err
	}
	_, granted, err := enforce.LookupGrant(v, normKey)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: unlock grants for %s: %v\n", normKey, err)
		return vaultWriteVerdict{}, err
	}

	// The drive-by mode-change defense runs first and ignores the grant: a
	// temporary unlock re-opens the body, never the mode: field (ADR-0031). Only
	// then is the body held to the prefix invariant the grant can waive.
	if d := enforce.EvaluateDriveByModeChange(normKey, old, newBytes, exists, ratified); !d.Allow {
		return vaultWriteVerdict{decision: "deny", reasonCode: d.ReasonCode, message: d.Message}, nil
	}

	decision := enforce.EvaluateVaultWrite(normKey, effectiveMode(normKey, old), old, newBytes, exists, ratified, granted, brokenReason)
	if decision.Allow {
		return vaultWriteVerdict{decision: "allow", normKey: normKey, newBytes: newBytes}, nil
	}
	return vaultWriteVerdict{decision: "deny", reasonCode: decision.ReasonCode, message: decision.Message}, nil
}

// deriveNewBytes computes the bytes a tool would land on disk. Per ADR-0031 the
// payload-alone derivation is exact only for Write (content verbatim); Edit and
// MultiEdit replay Claude's apply algorithm against disk-old via
// enforce.ReplayEdits — a replay abort (no match, ambiguous match, create via
// edit) returns an error so the in-vault write fails closed.
func deriveNewBytes(toolName string, ti fileToolInput, old []byte, exists bool) ([]byte, error) {
	switch toolName {
	case "Write":
		return []byte(ti.Content), nil
	case "Edit":
		return enforce.ReplayEdits(old, exists, []enforce.Edit{{
			OldString:  ti.OldString,
			NewString:  ti.NewString,
			ReplaceAll: ti.ReplaceAll,
		}})
	case "MultiEdit":
		edits := make([]enforce.Edit, len(ti.Edits))
		for i, e := range ti.Edits {
			edits[i] = enforce.Edit{OldString: e.OldString, NewString: e.NewString, ReplaceAll: e.ReplaceAll}
		}
		return enforce.ReplayEdits(old, exists, edits)
	}
	return nil, fmt.Errorf("%w: %s", errUnsupportedDerivation, toolName)
}

// brokenPrefixReason selects the append-only denial code by mutation shape: a
// whole-file Write/truncate carries ReasonAppendOnlyOverwrite, an in-place
// Edit/MultiEdit carries ReasonAppendOnlyInterior. They fire on the same prefix
// invariant and differ only in the recovery the message offers (ADR-0031).
func brokenPrefixReason(toolName string) string {
	switch toolName {
	case "Edit", "MultiEdit":
		return enforce.ReasonAppendOnlyInterior
	default:
		return enforce.ReasonAppendOnlyOverwrite
	}
}

// effectiveMode reads the note's current declared mode from its on-disk bytes,
// defaulting append-only when absent or unparseable. The verdict enforces the
// mode the note already carries, not whatever the write proposes (ADR-0031:
// mode is read from disk; a body-write may not change it).
func effectiveMode(key string, old []byte) markdown.WriteMode {
	meta, _, _ := markdown.ExtractMetadataLenient(key, old)
	return meta.Mode
}

func readOldBytes(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func emitVerdict(stdout io.Writer, decision, reasonCode, message string) {
	out := hookOutput{
		HookSpecificOutput: hookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision,
			PermissionDecisionReason: message,
		},
		ReasonCode: reasonCode,
	}
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	fmt.Fprintf(stdout, "%s\n", data)
}

// vaultRelativeKey maps an absolute tool file_path to a forward-slash
// vault-relative key, reporting whether it lands inside the vault. It resolves
// symlinks on both the vault root and the deepest existing ancestor of the
// target so a vault reached through a symlinked path (e.g. a macOS temp dir)
// still maps correctly before the target or its parents exist.
func vaultRelativeKey(v vault.Vault, filePath string) (string, bool, error) {
	realRoot, err := realPath(v.Root)
	if err != nil {
		return "", false, fmt.Errorf("resolve vault root: %w", err)
	}
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return "", false, err
	}
	realFile, err := realPath(abs)
	if err != nil {
		return "", false, err
	}
	rel, err := filepath.Rel(realRoot, realFile)
	if err != nil {
		return "", false, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false, nil
	}
	return filepath.ToSlash(rel), true, nil
}

// realPath resolves symlinks on the deepest existing prefix of path and
// re-appends the not-yet-existing suffix verbatim. EvalSymlinks errors on a
// missing path, but a non-existent component cannot be a symlink, so resolving
// the existing ancestor is sufficient to compare against the vault root.
func realPath(path string) (string, error) {
	dir := filepath.Clean(path)
	suffix := ""
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			if suffix == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, suffix), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		suffix = filepath.Join(filepath.Base(dir), suffix)
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", err
		}
		dir = parent
	}
}
