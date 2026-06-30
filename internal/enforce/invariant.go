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

// ReasonUnparsedMode names a denial where the note's frontmatter does not parse,
// so memento cannot read the mode the author declared. The note is held to the
// most-restrictive read-only treatment until the frontmatter is fixed — a parse
// error must never quietly make a note MORE writable than the author intended,
// and the recovery is to fix the frontmatter, not to loosen the mode (memento-o0a).
const ReasonUnparsedMode = "unparsed_mode"

// unparsedFrontmatterMessage is the shared denial copy for a note whose
// frontmatter fails to parse, used by both the prefix invariant and the
// operation lattice so the two surfaces cannot drift. The recovery is to repair
// the frontmatter — not `unlock`/`write-mode`, which presume a known mode.
func unparsedFrontmatterMessage(key string) string {
	return fmt.Sprintf(
		"note %s: its frontmatter does not parse, so memento cannot tell the mode its author declared and holds it read-only until that is fixed — this write is denied and the identical write will be denied again. "+
			"Fix the frontmatter so it parses (then the declared mode applies); do not self-authorise around this.",
		key)
}

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
// passes ReasonAppendOnlyInterior. The markdown.ModeUnparsed sentinel (frontmatter
// that does not parse) fails closed to read-only; any other unrecognised token
// fails closed to the append-only default (markdown.DefaultWriteMode), never
// living: it must not be possible to bypass the prefix invariant with bad mode
// DATA (ADR-0031, memento-o0a).
func EvaluatePrefixInvariant(key string, mode markdown.WriteMode, old, new []byte, brokenReason string) Decision {
	switch mode {
	case markdown.ModeUnparsed:
		// Frontmatter that does not parse has no readable mode; fail closed to
		// read-only (deny any change) so a parser failure cannot leave a note more
		// writable than its author declared (memento-o0a).
		if bytes.Equal(old, new) {
			return Decision{Allow: true}
		}
		return Decision{ReasonCode: ReasonUnparsedMode, Message: unparsedFrontmatterMessage(key)}
	case markdown.ModeReadOnly:
		if bytes.Equal(old, new) {
			return Decision{Allow: true}
		}
		return Decision{
			ReasonCode: ReasonReadOnly,
			Message: fmt.Sprintf(
				"read-only note %s: this write is denied and the identical write will be denied again. "+
					"Its mode blocks edits, and loosening that mode is a deliberate act that needs the user's explicit say-so — being told to do the task is not authorisation to loosen the note. "+
					"Stop and confirm with the user; only once they authorise the loosening, run `memento unlock %s --justification <reason>` for a one-off edit or `memento write-mode %s living --justification <reason>` for a permanent change.",
				key, key, key),
		}
	case markdown.ModeLiving:
		return Decision{Allow: true}
	default:
		// append-only — the explicit ModeAppendOnly case and the safe default for any
		// unrecognised/retired mode token, which must fail closed rather than fall
		// through to living (ADR-0031).
		if bytes.HasPrefix(new, old) {
			return Decision{Allow: true}
		}
		return Decision{
			ReasonCode: brokenReason,
			Message: fmt.Sprintf(
				"append-only note %s: this write drops or rewrites existing content, so it is denied and the identical write will be denied again. "+
					"Re-do it as an append that keeps the current content as a prefix. "+
					"Overwriting instead means loosening the mode — a deliberate act that needs the user's explicit say-so, not something to self-authorise to get unblocked; being told to do the task is not authorisation to loosen the note. "+
					"Stop and confirm with the user; only once they authorise it, run `memento unlock %s --justification <reason>` for a one-off edit or `memento write-mode %s living --justification <reason>` for a permanent change.",
				key, key, key),
		}
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
