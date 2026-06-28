package enforce

import (
	"fmt"

	"github.com/tpisel/memento/internal/markdown"
)

// ReasonDriveByModeChange names a denial where a body-write tried to change a
// note's effective mode — smuggling a permanent mode: change under cover of a
// temporary unlock (ADR-0031). It also covers new-bytes whose frontmatter no
// longer parses, since mode safety cannot then be verified.
const ReasonDriveByModeChange = "drive_by_mode_change"

// EvaluateDriveByModeChange enforces ADR-0031's drive-by mode-change defense:
// a body-write must not change a note's effective mode under cover of a
// temporary unlock — mode changes route through write-mode exclusively. For an
// existing, ratified note any change to the effective parsed mode (default
// applied) between old and new bytes is denied, even under an active grant; and
// new-bytes whose frontmatter fails to parse are denied, because mode safety
// cannot be verified. A new note (old absent) and an unratified note are
// carve-outs — legitimate birth/authoring may set mode: freely — so they always
// allow. This defense is layered alongside EvaluateVaultWrite, not inside it,
// because it ignores the grant the prefix invariant honours.
func EvaluateDriveByModeChange(key string, old, new []byte, exists, ratified bool) Decision {
	if !exists || !ratified {
		return Decision{Allow: true}
	}

	newMeta, newErrs, _ := markdown.ExtractMetadataLenient(key, new)
	if len(newErrs) > 0 {
		return Decision{
			ReasonCode: ReasonDriveByModeChange,
			Message: fmt.Sprintf(
				"note %s: the new frontmatter does not parse, so memento cannot verify this write leaves the mode unchanged — it is denied and the identical write will be denied again. "+
					"Fix the frontmatter so it parses (and route any mode change through `memento write-mode %s <mode> --justification <reason>`).",
				key, key),
		}
	}

	oldMode := effectiveModeOf(key, old)
	if oldMode == newMeta.Mode {
		return Decision{Allow: true}
	}
	return Decision{
		ReasonCode: ReasonDriveByModeChange,
		Message: fmt.Sprintf(
			"note %s: this write changes the note's mode (%s → %s) through a body edit, which is denied and the identical write will be denied again. "+
				"Split it: make the body edit without touching the mode: line, then run `memento write-mode %s %s --justification <reason>`.",
			key, oldMode, newMeta.Mode, key, newMeta.Mode),
	}
}

// effectiveModeOf returns the note's effective parsed mode (default applied),
// matching the verdict's read-mode-from-disk rule. A parse failure falls back to
// the default, since the old bytes are the committed baseline, not the write
// under scrutiny.
func effectiveModeOf(key string, source []byte) markdown.WriteMode {
	meta, _, _ := markdown.ExtractMetadataLenient(key, source)
	return meta.Mode
}
