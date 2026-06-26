package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tpisel/memento/internal/enforce"
)

// reasonApplyPatchUnsupported denies a codex apply_patch operation whose effect on
// a vault note check-write cannot yet verify: a Delete File, or an Update File that
// renames the note (Move to). Both change a note's existence/identity rather than
// append to its body, so they are held back fail-closed (ADR-0031) until a verb
// owns that operation, rather than gated on bytes we cannot model.
const reasonApplyPatchUnsupported = "apply_patch_unsupported_op"

// findPatchEnvelope recovers the apply_patch envelope text from the raw tool_input.
// codex's PreToolUse tool_input is untyped (codex hooks contract), so rather than
// bet on one key name, it scans for the first string value — at any depth, or a
// bare string tool_input — that contains the `*** Begin Patch` marker. Returning
// false means the payload carried no recognisable envelope, which the caller fails
// closed on.
func findPatchEnvelope(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.Contains(s, patchBeginMarker) {
			return s, true
		}
		return "", false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, v := range obj {
			if found, ok := findPatchEnvelope(v); ok {
				return found, true
			}
		}
	}
	return "", false
}

// patchBeginMarker is the envelope frame opener used to recognise a patch payload.
// It mirrors enforce's internal marker; duplicated here to keep the recogniser in
// the CLI layer without exporting parser internals.
const patchBeginMarker = "*** Begin Patch"

// checkWriteApplyPatch gives the verdict for a codex apply_patch call. It parses
// the envelope into file sections, gates every section that resolves into the
// vault against the same invariant/drive-by/verdict path as a Claude write, and
// denies the whole call on the first violation (the tool call is atomic, so one
// blocked section blocks all). With no vault target it is inert; an unparseable
// envelope fails closed (ADR-0031).
func checkWriteApplyPatch(patchText string, stdout, stderr io.Writer) int {
	ops, err := enforce.ParseApplyPatch(patchText)
	if err != nil {
		fmt.Fprintf(stderr, "memento check-write: parse apply_patch envelope: %v\n", err)
		return 1
	}

	v, ok, code := resolveVaultForCheck(stdout, stderr, "this apply_patch target")
	if !ok {
		return code
	}

	type pending struct {
		key  string
		hash string
	}
	var pendings []pending
	sawInVault := false

	for _, op := range ops {
		// Each section is resolved to its vault key first; an out-of-vault section is
		// left to normal permission flow even when a sibling section is in-vault.
		paths := []string{op.Path}
		if op.MoveTo != "" {
			paths = append(paths, op.MoveTo)
		}
		inVault := false
		for _, p := range paths {
			_, in, err := vaultRelativeKey(v, p)
			if err != nil {
				fmt.Fprintf(stderr, "memento check-write: resolve apply_patch target %s: %v\n", p, err)
				return 1
			}
			if in {
				inVault = true
			}
		}
		if !inVault {
			continue
		}
		sawInVault = true

		// Delete and rename touch a vault note's existence/identity, not just its
		// body; deny them fail-closed rather than model bytes we cannot derive.
		if op.Kind == enforce.PatchDelete || op.MoveTo != "" {
			emitVerdict(stdout, "deny", reasonApplyPatchUnsupported,
				"This apply_patch deletes or renames a memento note, which memento cannot verify as a safe write, "+
					"so it is denied and the identical patch will be denied again. "+
					"Edit the note in place, or change its lifecycle with the memento verbs.")
			return 0
		}

		key, _, err := vaultRelativeKey(v, op.Path)
		if err != nil {
			fmt.Fprintf(stderr, "memento check-write: resolve apply_patch target %s: %v\n", op.Path, err)
			return 1
		}

		verdict, ferr := computeVaultWriteVerdict(v, key, "apply_patch", brokenReasonForOp(op), deriveForOp(op), stderr)
		if ferr != nil {
			return 1 // fail-closed; stderr already written
		}
		if verdict.decision != "allow" {
			emitVerdict(stdout, verdict.decision, verdict.reasonCode, verdict.message)
			return 0
		}
		// Only an Update derives the exact bytes that will land (hunks applied to
		// known disk bytes), so only it seeds the drift ledger; an Add's creation is
		// allowed regardless of its exact bytes, so a possibly-imperfect rendering of
		// the added lines must not arm a false drift alarm.
		if op.Kind == enforce.PatchUpdate {
			pendings = append(pendings, pending{key: verdict.normKey, hash: enforce.HashBytes(verdict.newBytes)})
		}
	}

	if !sawInVault {
		return 0 // no vault target: leave the patch to normal permission flow
	}
	for _, p := range pendings {
		if err := enforce.RecordPending(v, p.key, p.hash); err != nil {
			fmt.Fprintf(stderr, "memento check-write: record pending write for %s: %v\n", p.key, err)
		}
	}
	emitVerdict(stdout, "allow", "", "")
	return 0
}

// deriveForOp returns the new-bytes derivation for an Add or Update section. Add
// yields the added lines verbatim (creation is allowed regardless of mode, so its
// exact bytes do not gate the verdict); Update replays the hunks against disk-old.
func deriveForOp(op enforce.PatchOp) func(old []byte, exists bool) ([]byte, error) {
	switch op.Kind {
	case enforce.PatchAdd:
		content := strings.Join(op.Added, "\n")
		return func(_ []byte, _ bool) ([]byte, error) { return []byte(content), nil }
	default: // PatchUpdate
		return func(old []byte, exists bool) ([]byte, error) {
			return enforce.ApplyHunks(old, exists, op.Hunks)
		}
	}
}

// brokenReasonForOp picks the append-only denial flavour by section shape: an Add
// is a whole-file write (overwrite), an Update is an in-place edit (interior).
func brokenReasonForOp(op enforce.PatchOp) string {
	if op.Kind == enforce.PatchAdd {
		return enforce.ReasonAppendOnlyOverwrite
	}
	return enforce.ReasonAppendOnlyInterior
}
