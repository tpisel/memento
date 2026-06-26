package enforce

import (
	"errors"
	"testing"
)

// TestReplayEditsGolden is the captured correctness oracle for the Edit/MultiEdit
// apply algorithm (ADR-0031): each case pins disk-old + edits to the exact bytes
// Claude lands, so a future divergence from Claude's unpublished contract shows up
// here rather than as a silently mis-gated write.
func TestReplayEditsGolden(t *testing.T) {
	cases := []struct {
		name  string
		old   string
		edits []Edit
		want  string
	}{
		{
			name:  "single unique replace",
			old:   "# Log\n\nEntry one.\n",
			edits: []Edit{{OldString: "Entry one.", NewString: "Entry two."}},
			want:  "# Log\n\nEntry two.\n",
		},
		{
			name:  "tail append keeps prefix",
			old:   "# Log\n\nEntry one.\n",
			edits: []Edit{{OldString: "Entry one.\n", NewString: "Entry one.\nEntry two.\n"}},
			want:  "# Log\n\nEntry one.\nEntry two.\n",
		},
		{
			name:  "interior rewrite breaks prefix",
			old:   "# Log\n\nEntry one.\nEntry two.\n",
			edits: []Edit{{OldString: "Entry one.", NewString: "Edited."}},
			want:  "# Log\n\nEdited.\nEntry two.\n",
		},
		{
			name:  "replace_all substitutes every occurrence",
			old:   "a x a x a\n",
			edits: []Edit{{OldString: "a", NewString: "b", ReplaceAll: true}},
			want:  "b x b x b\n",
		},
		{
			name: "multiedit applies sequentially",
			old:  "one two three\n",
			edits: []Edit{
				{OldString: "one", NewString: "1"},
				{OldString: "two", NewString: "2"},
				{OldString: "three", NewString: "3"},
			},
			want: "1 2 3\n",
		},
		{
			name: "later edit sees earlier edit's output",
			old:  "alpha\n",
			edits: []Edit{
				{OldString: "alpha", NewString: "beta"},
				{OldString: "beta", NewString: "gamma"},
			},
			want: "gamma\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReplayEdits([]byte(tc.old), true, tc.edits)
			if err != nil {
				t.Fatalf("ReplayEdits returned error: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("ReplayEdits =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestReplayEditsAbortsMatchClaude(t *testing.T) {
	cases := []struct {
		name   string
		old    string
		exists bool
		edits  []Edit
		want   error
	}{
		{
			name:   "missing file cannot be created by edit",
			exists: false,
			edits:  []Edit{{OldString: "x", NewString: "y"}},
			want:   ErrReplayCreateViaEdit,
		},
		{
			name:   "empty old_string is reserved to Write",
			old:    "body\n",
			exists: true,
			edits:  []Edit{{OldString: "", NewString: "body\nmore\n"}},
			want:   ErrReplayCreateViaEdit,
		},
		{
			name:   "old_string absent",
			old:    "body\n",
			exists: true,
			edits:  []Edit{{OldString: "nope", NewString: "y"}},
			want:   ErrReplayNoMatch,
		},
		{
			name:   "non-unique match without replace_all aborts",
			old:    "a a a\n",
			exists: true,
			edits:  []Edit{{OldString: "a", NewString: "b"}},
			want:   ErrReplayAmbiguous,
		},
		{
			name:   "abort surfaces from a later edit in the sequence",
			old:    "a a\n",
			exists: true,
			edits: []Edit{
				{OldString: "a a", NewString: "c c"}, // unique now
				{OldString: "c", NewString: "d"},     // ambiguous after the first edit
			},
			want: ErrReplayAmbiguous,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReplayEdits([]byte(tc.old), tc.exists, tc.edits)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ReplayEdits error = %v, want %v", err, tc.want)
			}
		})
	}
}
