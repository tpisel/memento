package enforce

import (
	"strings"
	"testing"
)

func TestEvaluateDriveByModeChange(t *testing.T) {
	const key = "notes/n.md"

	const roOld = "---\nmode: read-only\n---\n# N\n\nBody.\n"
	const livingNew = "---\nmode: living\n---\n# N\n\nBody.\n"
	const roEdited = "---\nmode: read-only\n---\n# N\n\nEdited body.\n"
	const noFrontmatter = "# N\n\nBody.\n" // effective mode defaults to append-only
	const aoOld = "---\nmode: append-only\n---\n# N\n\nBody.\n"
	const badFrontmatter = "---\nmode: : oops\n  - broken\n---\n# N\n\nBody.\n"

	tests := []struct {
		name       string
		old        string
		new        string
		exists     bool
		ratified   bool
		wantAllow  bool
		wantReason string
	}{
		{name: "ratified read-only flipped to living denied (US4)", old: roOld, new: livingNew, exists: true, ratified: true, wantAllow: false, wantReason: ReasonDriveByModeChange},
		{name: "ratified body edit keeping mode allowed", old: roOld, new: roEdited, exists: true, ratified: true, wantAllow: true},
		{name: "dropping frontmatter changes effective mode, denied", old: roOld, new: noFrontmatter, exists: true, ratified: true, wantAllow: false, wantReason: ReasonDriveByModeChange},
		{name: "default-mode note staying default allowed", old: aoOld, new: aoOld + "More.\n", exists: true, ratified: true, wantAllow: true},
		{name: "unparseable new frontmatter denied", old: roOld, new: badFrontmatter, exists: true, ratified: true, wantAllow: false, wantReason: ReasonDriveByModeChange},
		// Repairing a note whose committed frontmatter does not parse is not a
		// drive-by mode change: there is no known prior mode to protect, so the
		// defense defers to the prefix invariant (which holds it read-only) and to
		// any unlock grant, rather than locking the note out of repair (memento-o0a).
		{name: "repairing unparseable baseline is not a drive-by change", old: badFrontmatter, new: livingNew, exists: true, ratified: true, wantAllow: true},
		{name: "new note may set mode freely (carve-out)", old: "", new: livingNew, exists: false, ratified: false, wantAllow: true},
		{name: "unratified note may set mode freely (carve-out)", old: roOld, new: livingNew, exists: true, ratified: false, wantAllow: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateDriveByModeChange(key, []byte(tc.old), []byte(tc.new), tc.exists, tc.ratified)
			if got.Allow != tc.wantAllow {
				t.Fatalf("Allow = %v, want %v (%+v)", got.Allow, tc.wantAllow, got)
			}
			if tc.wantAllow {
				if got.ReasonCode != "" || got.Message != "" {
					t.Fatalf("allow verdict carried ReasonCode=%q Message=%q, want empty", got.ReasonCode, got.Message)
				}
				return
			}
			if got.ReasonCode != tc.wantReason {
				t.Fatalf("ReasonCode = %q, want %q", got.ReasonCode, tc.wantReason)
			}
			if !strings.Contains(got.Message, key) {
				t.Fatalf("Message = %q, want it to name key %q", got.Message, key)
			}
			if !strings.Contains(got.Message, "denied again") {
				t.Fatalf("Message = %q, want it to warn the identical write will be denied again", got.Message)
			}
		})
	}
}
