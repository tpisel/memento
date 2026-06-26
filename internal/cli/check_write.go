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

// errUnsupportedDerivation marks a tool whose new-bytes derivation this build
// cannot compute yet (Edit/MultiEdit/Bash/codex land in later beads). For an
// in-vault target it surfaces as an internal error so the dumb-pipe wrapper
// fails closed (ADR-0031) rather than silently allowing an ungated write.
var errUnsupportedDerivation = errors.New("unsupported write derivation")

// preToolUse is the slice of the raw PreToolUse payload check-write consumes.
// Unknown fields are ignored; only the tool name and the file-write inputs the
// Write derivation needs are read.
type preToolUse struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	} `json:"tool_input"`
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
// It reads the raw PreToolUse payload on stdin, resolves the target to a
// vault-relative key, reads old-bytes from disk + ratification from git + active
// unlock grants itself, derives new-bytes (Write only in this build), evaluates
// the prefix invariant, and emits the harness verdict JSON on stdout. Writes
// outside the vault and non-file tools are inert (exit 0, no output) so normal
// permission flow governs the rest of the repo. Internal failures exit non-zero
// for the wrapper to convert to a fail-closed deny.
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

	// Only file-targeted write tools are in scope here; Bash (command, not
	// file_path) and non-write tools fall through to normal permission flow.
	if p.ToolName == "" || p.ToolInput.FilePath == "" {
		return 0
	}
	switch p.ToolName {
	case "Write", "Edit", "MultiEdit":
	default:
		return 0
	}

	v, err := resolveVault()
	if err != nil {
		if errors.Is(err, vault.ErrVaultNotFound) {
			return 0 // no vault to guard
		}
		if errors.Is(err, vault.ErrMultipleVaults) {
			emitVerdict(stdout, "ask", reasonVaultDiscoveryAmbiguous, fmt.Sprintf(
				"memento found more than one .memento vault, so it cannot tell which one %q belongs to. "+
					"Set MEMENTO_VAULT_ROOT to the intended vault root, then retry the write.", p.ToolInput.FilePath))
			return 0
		}
		fmt.Fprintf(stderr, "memento check-write: resolve vault: %v\n", err)
		return 1
	}

	key, inVault, err := vaultRelativeKey(v, p.ToolInput.FilePath)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: resolve target: %v\n", err)
		return 1
	}
	if !inVault {
		return 0
	}

	normKey, err := enforce.NormalizeWritableKey(v, key)
	if err != nil {
		emitVerdict(stdout, "deny", reasonUnwritablePath, fmt.Sprintf(
			"%s is not a writable memento note — it is git-ignored, operational, or not a .md file — so this write is denied and the identical write will be denied again. "+
				"Pick a different .md key under the vault.", key))
		return 0
	}
	path, err := enforce.ResolveWritablePath(v, normKey)
	if err != nil {
		emitVerdict(stdout, "deny", reasonUnwritablePath, fmt.Sprintf(
			"%s does not name a writable memento note (it resolves to a directory, a symlink, or outside the vault), so this write is denied and the identical write will be denied again. "+
				"Pick a different .md key under the vault.", normKey))
		return 0
	}

	old, exists, err := readOldBytes(path)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: read %s: %v\n", normKey, err)
		return 1
	}

	newBytes, err := deriveNewBytes(p, old)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: %s derivation is not implemented in this build: %v\n", p.ToolName, err)
		return 1
	}

	ratified, err := enforce.IsRatified(v, normKey)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: ratification for %s: %v\n", normKey, err)
		return 1
	}
	_, granted, err := enforce.LookupGrant(v, normKey)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: unlock grants for %s: %v\n", normKey, err)
		return 1
	}

	decision := enforce.EvaluateVaultWrite(normKey, effectiveMode(normKey, old), old, newBytes, exists, ratified, granted, enforce.ReasonAppendOnlyOverwrite)
	if decision.Allow {
		emitVerdict(stdout, "allow", "", "")
		return 0
	}
	emitVerdict(stdout, "deny", decision.ReasonCode, decision.Message)
	return 0
}

// deriveNewBytes computes the bytes a tool would land on disk. Per ADR-0031 the
// payload-alone derivation is exact only for Write (content verbatim); Edit /
// MultiEdit must replay the tool's mutation algorithm against disk-old (a
// separate bead), so they report errUnsupportedDerivation here.
func deriveNewBytes(p preToolUse, _ []byte) ([]byte, error) {
	if p.ToolName == "Write" {
		return []byte(p.ToolInput.Content), nil
	}
	return nil, fmt.Errorf("%w: %s", errUnsupportedDerivation, p.ToolName)
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
