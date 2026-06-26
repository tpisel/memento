package enforce

import (
	"bytes"
	"fmt"

	"github.com/tpisel/memento/internal/markdown"
)

// ReasonAppendOnlyInterior names an append-only denial where the prefix was
// broken in place (an Edit that rewrites existing bytes) rather than by a
// whole-file truncate/Write (ReasonAppendOnlyOverwrite). Both fire on the same
// invariant; they differ only in the recovery the denial message offers.
const ReasonAppendOnlyInterior = "append_only_interior"

// EvaluatePrefixInvariant applies ADR-0031's prefix invariant to a write already
// derived to concrete new-bytes against a note's declared mode:
//
//	read-only   ≡ new == old
//	append-only ≡ new has old as a prefix
//	living      ≡ always allow
//
// It is the pure content rule for a ratified, ungranted note; ratification and
// the unlock grant (the mutability predicate) are composed by EvaluateVaultWrite.
// brokenReason is the reason code to attach when append-only's prefix is broken:
// a whole-file Write/truncate passes ReasonAppendOnlyOverwrite, an in-place Edit
// passes ReasonAppendOnlyInterior. An unrecognised mode is treated as living.
func EvaluatePrefixInvariant(key string, mode markdown.WriteMode, old, new []byte, brokenReason string) Decision {
	switch mode {
	case markdown.ModeReadOnly:
		if bytes.Equal(old, new) {
			return Decision{Allow: true}
		}
		return Decision{
			ReasonCode: ReasonReadOnly,
			Message: fmt.Sprintf(
				"read-only note %s: edits are denied and the identical write will be denied again. "+
					"Ask the user before changing it; if they agree, run `memento unlock %s --justification <reason>` for a one-off edit.",
				key, key),
		}
	case markdown.ModeAppendOnly:
		if bytes.HasPrefix(new, old) {
			return Decision{Allow: true}
		}
		return Decision{
			ReasonCode: brokenReason,
			Message: fmt.Sprintf(
				"append-only note %s: this write drops or rewrites existing content, so it is denied and the identical write will be denied again. "+
					"Re-do it as an append that keeps the current content as a prefix, or run `memento write-mode %s living --justification <reason>` to allow overwrites.",
				key, key),
		}
	default:
		return Decision{Allow: true}
	}
}

// EvaluateVaultWrite composes the full check-write content verdict for a derived
// write: a note still in its edit window — never committed (unratified) or
// covered by an active unlock grant — accepts any write (ADR-0031's mutability
// predicate); a brand-new note (Write, old absent) is legitimate creation. Only a
// ratified, ungranted, existing note is held to the prefix invariant. The
// drive-by mode-change defense (ADR-0031) is layered on separately and is not
// applied here.
func EvaluateVaultWrite(key string, mode markdown.WriteMode, old, new []byte, exists, ratified, granted bool, brokenReason string) Decision {
	if !exists || !ratified || granted {
		return Decision{Allow: true}
	}
	return EvaluatePrefixInvariant(key, mode, old, new, brokenReason)
}
