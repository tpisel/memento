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
// every write; append-only denies overwrite but allows append. An unrecognised
// mode is treated as living (allow), matching the prior write path.
func EvaluateMode(key string, mode markdown.WriteMode, op Operation) Decision {
	switch mode {
	case markdown.ModeReadOnly:
		return Decision{
			ReasonCode: ReasonReadOnly,
			Message:    fmt.Sprintf("%s is read-only", key),
		}
	case markdown.ModeAppendOnly:
		if op == OpOverwrite {
			return Decision{
				ReasonCode: ReasonAppendOnlyOverwrite,
				Message:    fmt.Sprintf("%s is append-only; overwrite is not permitted", key),
			}
		}
		return Decision{Allow: true}
	default:
		return Decision{Allow: true}
	}
}
