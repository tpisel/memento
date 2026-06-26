package enforce

import (
	"strings"
	"testing"

	"github.com/tpisel/memento/internal/markdown"
)

func TestEvaluateMode(t *testing.T) {
	const key = "notes/n.md"

	cases := []struct {
		name       string
		mode       markdown.WriteMode
		op         Operation
		wantAllow  bool
		wantReason string
	}{
		{"read-only append", markdown.ModeReadOnly, OpAppend, false, ReasonReadOnly},
		{"read-only overwrite", markdown.ModeReadOnly, OpOverwrite, false, ReasonReadOnly},
		{"append-only append", markdown.ModeAppendOnly, OpAppend, true, ""},
		{"append-only overwrite", markdown.ModeAppendOnly, OpOverwrite, false, ReasonAppendOnlyOverwrite},
		{"living append", markdown.ModeLiving, OpAppend, true, ""},
		{"living overwrite", markdown.ModeLiving, OpOverwrite, true, ""},
		// An unrecognised/retired mode token (e.g. ADR-0031 flips section-replace to
		// invalid) must fail closed to the append-only default, never living: append
		// allowed, overwrite denied.
		{"unknown mode append allowed", markdown.WriteMode("section-replace"), OpAppend, true, ""},
		{"unknown mode overwrite denied", markdown.WriteMode("section-replace"), OpOverwrite, false, ReasonAppendOnlyOverwrite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateMode(key, tc.mode, tc.op)
			if got.Allow != tc.wantAllow {
				t.Fatalf("EvaluateMode Allow = %v, want %v", got.Allow, tc.wantAllow)
			}
			if got.ReasonCode != tc.wantReason {
				t.Fatalf("EvaluateMode ReasonCode = %q, want %q", got.ReasonCode, tc.wantReason)
			}
			if tc.wantAllow {
				if got.Message != "" {
					t.Fatalf("EvaluateMode Message = %q, want empty on allow", got.Message)
				}
			} else if !strings.Contains(got.Message, key) {
				t.Fatalf("EvaluateMode Message = %q, want it to name key %q", got.Message, key)
			}
		})
	}
}
