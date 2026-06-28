package enforce

import (
	"testing"

	"github.com/tpisel/memento/internal/markdown"
)

// TestAuditRatifiedChange exercises the pure ratification-boundary verdict: old
// is sourced from HEAD bytes, and a non-Allow Decision marks an ungated mode
// violation. Each case mirrors an acceptance bullet from the backstop note.
func TestAuditRatifiedChange(t *testing.T) {
	const key = "n.md"

	cases := []struct {
		name      string
		head      string
		disk      string
		granted   bool
		violation bool
	}{
		{
			name:      "read-only edited on disk is a violation",
			head:      "---\nmode: read-only\n---\n# N\n\nOriginal.\n",
			disk:      "---\nmode: read-only\n---\n# N\n\nRewritten.\n",
			violation: true,
		},
		{
			name:      "living note edited is not a violation",
			head:      "---\nmode: living\n---\n# N\n\nOriginal.\n",
			disk:      "---\nmode: living\n---\n# N\n\nRewritten and more.\n",
			violation: false,
		},
		{
			name:      "append-only pure append is not a violation",
			head:      "---\nmode: append-only\n---\n# N\n\nEntry one.\n",
			disk:      "---\nmode: append-only\n---\n# N\n\nEntry one.\nEntry two.\n",
			violation: false,
		},
		{
			name:      "append-only interior rewrite is a violation",
			head:      "---\nmode: append-only\n---\n# N\n\nEntry one.\n",
			disk:      "---\nmode: append-only\n---\n# N\n\nEntry CHANGED.\n",
			violation: true,
		},
		{
			name:      "grant-covered read-only edit is not flagged",
			head:      "---\nmode: read-only\n---\n# N\n\nOriginal.\n",
			disk:      "---\nmode: read-only\n---\n# N\n\nRewritten under grant.\n",
			granted:   true,
			violation: false,
		},
		{
			name:      "pure write-mode change (read-only to living) is not flagged",
			head:      "---\nmode: read-only\n---\n# N\n\nBody.\n",
			disk:      "---\nmode: living\n---\n# N\n\nBody.\n",
			violation: false,
		},
		{
			name:      "pure write-mode change survives frontmatter reformatting",
			head:      "---\ntitle: N\nmode: read-only\n---\n# N\n\nBody.\n",
			disk:      "---\ntitle: N\nmode: append-only\n---\n# N\n\nBody.\n",
			violation: false,
		},
		{
			name:      "mode change smuggling a body edit is still a violation",
			head:      "---\nmode: read-only\n---\n# N\n\nBody.\n",
			disk:      "---\nmode: living\n---\n# N\n\nBody tampered.\n",
			violation: true,
		},
		{
			name:      "deletion of a read-only note is a violation",
			head:      "---\nmode: read-only\n---\n# N\n\nBody.\n",
			disk:      "",
			violation: true,
		},
		{
			name:      "no-mode note defaults to append-only: interior rewrite is a violation",
			head:      "# N\n\nEntry one.\n",
			disk:      "# N\n\nEntry CHANGED.\n",
			violation: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := AuditRatifiedChange(key, []byte(tc.head), []byte(tc.disk), tc.granted)
			if tc.violation && d.Allow {
				t.Fatalf("AuditRatifiedChange allowed a change that should be a MODE VIOLATION")
			}
			if !tc.violation && !d.Allow {
				t.Fatalf("AuditRatifiedChange flagged a change that should be allowed (reason %q)", d.ReasonCode)
			}
		})
	}
}

// TestAuditRatifiedChangeReadOnlyReasonCode pins the reason code a read-only
// violation carries, since the compile alarm renders its human cause from it.
func TestAuditRatifiedChangeReadOnlyReasonCode(t *testing.T) {
	d := AuditRatifiedChange("n.md",
		[]byte("---\nmode: read-only\n---\n# N\n\nA.\n"),
		[]byte("---\nmode: read-only\n---\n# N\n\nB.\n"), false)
	if d.Allow {
		t.Fatalf("read-only edit should be a violation")
	}
	if d.ReasonCode != ReasonReadOnly {
		t.Fatalf("ReasonCode = %q, want %q", d.ReasonCode, ReasonReadOnly)
	}
}

// TestAuditRatifiedChangeBadModeFailsClosed: an unrecognised mode token must fall
// to the append-only default, never living, so a bad-mode interior rewrite is
// still caught (ADR-0031: bad mode DATA must not unlock a note).
func TestAuditRatifiedChangeBadModeFailsClosed(t *testing.T) {
	_ = markdown.DefaultWriteMode // append-only is the documented fail-closed default
	d := AuditRatifiedChange("n.md",
		[]byte("---\nmode: bogus\n---\n# N\n\nEntry one.\n"),
		[]byte("---\nmode: bogus\n---\n# N\n\nEntry CHANGED.\n"), false)
	if d.Allow {
		t.Fatalf("bad-mode interior rewrite should fail closed to append-only and be a violation")
	}
}
