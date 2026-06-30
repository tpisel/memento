package enforce

import (
	"strings"
	"testing"

	"github.com/tpisel/memento/internal/markdown"
)

func TestEvaluatePrefixInvariant(t *testing.T) {
	const key = "notes/n.md"

	tests := []struct {
		name       string
		mode       markdown.WriteMode
		old        string
		new        string
		wantAllow  bool
		wantReason string
	}{
		{name: "living allows divergent overwrite", mode: markdown.ModeLiving, old: "a", new: "totally different", wantAllow: true},
		{name: "read-only allows identical bytes", mode: markdown.ModeReadOnly, old: "frozen", new: "frozen", wantAllow: true},
		{name: "read-only denies any edit", mode: markdown.ModeReadOnly, old: "frozen", new: "frozen + more", wantAllow: false, wantReason: ReasonReadOnly},
		{name: "append-only allows pure append", mode: markdown.ModeAppendOnly, old: "head", new: "head\nmore", wantAllow: true},
		{name: "append-only allows no-op", mode: markdown.ModeAppendOnly, old: "head", new: "head", wantAllow: true},
		{name: "append-only denies truncate", mode: markdown.ModeAppendOnly, old: "head\nbody", new: "head", wantAllow: false, wantReason: ReasonAppendOnlyOverwrite},
		{name: "append-only denies interior change", mode: markdown.ModeAppendOnly, old: "head\nbody", new: "HEAD\nbody", wantAllow: false, wantReason: ReasonAppendOnlyOverwrite},
		// An unrecognised/retired effective mode fails closed to the append-only
		// default, never living (ADR-0031): pure append allowed, truncate denied.
		{name: "unknown mode allows pure append", mode: markdown.WriteMode("section-replace"), old: "head", new: "head\nmore", wantAllow: true},
		{name: "unknown mode denies truncate", mode: markdown.WriteMode("section-replace"), old: "head\nbody", new: "head", wantAllow: false, wantReason: ReasonAppendOnlyOverwrite},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluatePrefixInvariant(key, tc.mode, []byte(tc.old), []byte(tc.new), ReasonAppendOnlyOverwrite)
			if got.Allow != tc.wantAllow {
				t.Fatalf("Allow = %v, want %v", got.Allow, tc.wantAllow)
			}
			if got.Allow {
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
			assertLooseningStance(t, got.Message)
		})
	}
}

func TestEvaluatePrefixInvariantUnparsedIsReadOnly(t *testing.T) {
	const key = "notes/n.md"

	// A note whose frontmatter does not parse resolves to ModeUnparsed and must be
	// held to the most-restrictive read-only treatment, never the append-only
	// default that would silently keep it writable-by-append (memento-o0a).
	deny := EvaluatePrefixInvariant(key, markdown.ModeUnparsed, []byte("head"), []byte("head\nmore"), ReasonAppendOnlyOverwrite)
	if deny.Allow {
		t.Fatalf("Allow = true, want deny: an unparsed note must be held read-only, not append-only")
	}
	if deny.ReasonCode != ReasonUnparsedMode {
		t.Fatalf("ReasonCode = %q, want %q", deny.ReasonCode, ReasonUnparsedMode)
	}
	for _, want := range []string{key, "does not parse", "denied again", "Fix the frontmatter"} {
		if !strings.Contains(deny.Message, want) {
			t.Fatalf("Message = %q, want it to contain %q", deny.Message, want)
		}
	}

	// An identical-bytes no-op is still allowed (read-only ≡ new == old).
	if allow := EvaluatePrefixInvariant(key, markdown.ModeUnparsed, []byte("head"), []byte("head"), ReasonAppendOnlyOverwrite); !allow.Allow {
		t.Fatalf("identical-bytes write on unparsed note denied (%+v), want allow", allow)
	}
}

func TestEvaluatePrefixInvariantBrokenReason(t *testing.T) {
	got := EvaluatePrefixInvariant("notes/n.md", markdown.ModeAppendOnly, []byte("head\nbody"), []byte("head\nBODY"), ReasonAppendOnlyInterior)
	if got.Allow || got.ReasonCode != ReasonAppendOnlyInterior {
		t.Fatalf("got %+v, want deny with %q", got, ReasonAppendOnlyInterior)
	}
}

func TestEvaluateVaultWrite(t *testing.T) {
	const key = "notes/n.md"

	tests := []struct {
		name       string
		mode       markdown.WriteMode
		old        string
		new        string
		exists     bool
		ratified   bool
		granted    bool
		wantAllow  bool
		wantReason string
	}{
		{name: "new note allowed (US5)", mode: markdown.ModeReadOnly, old: "", new: "---\nmode: read-only\n---\nbody", exists: false, ratified: false, wantAllow: true},
		{name: "unratified existing note allowed (US6)", mode: markdown.ModeReadOnly, old: "frozen", new: "edited", exists: true, ratified: false, wantAllow: true},
		{name: "active grant re-opens read-only", mode: markdown.ModeReadOnly, old: "frozen", new: "edited", exists: true, ratified: true, granted: true, wantAllow: true},
		{name: "ratified read-only edit denied (US1)", mode: markdown.ModeReadOnly, old: "frozen", new: "edited", exists: true, ratified: true, wantAllow: false, wantReason: ReasonReadOnly},
		{name: "ratified append-only truncate denied (US2)", mode: markdown.ModeAppendOnly, old: "head\nbody", new: "head", exists: true, ratified: true, wantAllow: false, wantReason: ReasonAppendOnlyOverwrite},
		{name: "ratified append-only append allowed (US2)", mode: markdown.ModeAppendOnly, old: "head", new: "head\nmore", exists: true, ratified: true, wantAllow: true},
		// A ratified note whose effective mode is unrecognised/retired is held to the
		// append-only default, not let through as living (ADR-0031 bug fix).
		{name: "ratified unknown-mode truncate denied", mode: markdown.WriteMode("section-replace"), old: "head\nbody", new: "head", exists: true, ratified: true, wantAllow: false, wantReason: ReasonAppendOnlyOverwrite},
		{name: "ratified unknown-mode append allowed", mode: markdown.WriteMode("section-replace"), old: "head", new: "head\nmore", exists: true, ratified: true, wantAllow: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateVaultWrite(key, tc.mode, []byte(tc.old), []byte(tc.new), tc.exists, tc.ratified, tc.granted, ReasonAppendOnlyOverwrite)
			if got.Allow != tc.wantAllow {
				t.Fatalf("Allow = %v, want %v (%+v)", got.Allow, tc.wantAllow, got)
			}
			if !tc.wantAllow && got.ReasonCode != tc.wantReason {
				t.Fatalf("ReasonCode = %q, want %q", got.ReasonCode, tc.wantReason)
			}
		})
	}
}
