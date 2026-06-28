package enforce

import (
	"bytes"

	"github.com/tpisel/memento/internal/markdown"
)

// AuditRatifiedChange runs ADR-0031's ratification-boundary diff audit for a
// single note that exists at HEAD (ratified) and whose on-disk bytes differ from
// HEAD. It is the path-agnostic backstop to the PreToolUse check-write gate:
// because it inspects END-STATE (HEAD vs disk) it catches ungated mode violations
// regardless of the write path that produced them — codex exec shell writes,
// untrusted codex hooks, and Claude opaque shell all fail open at the gate but
// land here. See [[enforcement backstop — ratification-boundary diff audit]].
//
// It reuses the pure prefix invariant (EvaluatePrefixInvariant) with `old`
// sourced from HEAD instead of the pending-writes ledger, after the two
// authorisation carve-outs check-write composes:
//
//   - granted — an active unlock grant re-opens the edit window, so any change is
//     allowed (the grant is the user's recorded authorisation). In the pre-commit
//     tier the caller reads the grant sidecar BEFORE `memento clear-grants` (which
//     runs later in the same hook) clears it.
//   - a pure write-mode change — a mode: edit is legitimate only through the
//     write-mode verb, which touches the mode line alone. Re-normalising both HEAD
//     and disk to one mode and comparing isolates that: byte-equal after
//     normalisation ⟹ the only difference was the mode line ⟹ not a violation.
//
// Brand-new notes (absent at HEAD) and compile's own operational rewrites are
// excluded by the caller, which only audits ratified writable note keys. A
// non-Allow Decision is an ungated mode violation; Allow means no violation. The
// mode enforced is read from HEAD bytes (the ratified mode in force), matching
// check-write's read-mode-from-the-baseline rule.
func AuditRatifiedChange(key string, head, disk []byte, granted bool) Decision {
	if granted {
		return Decision{Allow: true}
	}
	// A write-mode change rewrites only the mode: line; normalising both sides to
	// the same mode collapses that single edit, so byte equality means the body and
	// the rest of the frontmatter are untouched. This cannot mask a body edit: any
	// differing body byte survives the normalisation and breaks equality.
	if bytes.Equal(markdown.SetMode(head, markdown.ModeLiving), markdown.SetMode(disk, markdown.ModeLiving)) {
		return Decision{Allow: true}
	}
	return EvaluatePrefixInvariant(key, effectiveModeOf(key, head), head, disk, ReasonAppendOnlyInterior)
}
