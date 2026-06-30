package enforce

import (
	"fmt"

	"github.com/tpisel/memento/internal/markdown"
)

// Operation is the kind of write being evaluated against a note's mode.
type Operation string

const (
	OpAppend    Operation = "append"
	OpOverwrite Operation = "overwrite"
)

// Reason codes for a mode denial, aligned with ADR-0031's denial-UX taxonomy.
// The prefix-invariant codes (e.g. append_only_interior) are introduced by the
// check-write core; EvaluateMode covers only the operation-level lattice.
const (
	ReasonReadOnly            = "read_only"
	ReasonAppendOnlyOverwrite = "append_only_overwrite"
)

// Decision is the verdict of a mode-lattice evaluation. When Allow is true the
// operation is permitted and the other fields are empty; otherwise ReasonCode
// names why (for the denial-UX taxonomy) and Message is the human-facing
// explanation that becomes permissionDecisionReason.
type Decision struct {
	Allow      bool
	ReasonCode string
	Message    string
}

// EvaluateMode applies the three-mode lattice (ADR-0015) to op against a note's
// declared mode. It is pure: ratification, the unlock grant, and force/justify
// affordances are composed by the caller. living always allows; read-only denies
// every write; append-only denies overwrite but allows append. The
// markdown.ModeUnparsed sentinel (unparseable frontmatter) fails closed to
// read-only; any other unrecognised/retired token fails closed to the append-only
// default (markdown.DefaultWriteMode), never living: bad mode DATA must not unlock
// a note (ADR-0031, memento-o0a).
func EvaluateMode(key string, mode markdown.WriteMode, op Operation) Decision {
	switch mode {
	case markdown.ModeUnparsed:
		// Frontmatter that does not parse has no readable mode; fail closed to
		// read-only — deny every write — so a parser failure cannot leave a note
		// more writable than its author declared (memento-o0a).
		return Decision{ReasonCode: ReasonUnparsedMode, Message: unparsedFrontmatterMessage(key)}
	case markdown.ModeReadOnly:
		return Decision{
			ReasonCode: ReasonReadOnly,
			Message: fmt.Sprintf(
				"read-only note %s: writes are denied. Loosening the mode needs the user's explicit say-so — being told to do the task is not authorisation to loosen the note. "+
					"Stop and confirm with the user; only once they authorise it, run `memento unlock %s --justification <reason>` for a one-off edit or `memento write-mode %s living --justification <reason>` for a permanent change.",
				key, key, key),
		}
	case markdown.ModeLiving:
		return Decision{Allow: true}
	default:
		// append-only — the explicit ModeAppendOnly case and the safe default for any
		// unrecognised/retired mode token, which must fail closed rather than fall
		// through to living (ADR-0031).
		if op == OpOverwrite {
			return Decision{
				ReasonCode: ReasonAppendOnlyOverwrite,
				Message: fmt.Sprintf(
					"append-only note %s: overwrite is denied (append is allowed). Overwriting means loosening the mode, which needs the user's explicit say-so — being told to do the task is not authorisation to loosen the note. "+
						"Stop and confirm with the user; only once they authorise it, run `memento unlock %s --justification <reason>` for a one-off edit or `memento write-mode %s living --justification <reason>` for a permanent change.",
					key, key, key),
			}
		}
		return Decision{Allow: true}
	}
}
