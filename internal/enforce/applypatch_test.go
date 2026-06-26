package enforce

import (
	"errors"
	"strings"
	"testing"
)

func TestParseApplyPatchAddFile(t *testing.T) {
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: notes/new.md",
		"+---",
		"+mode: living",
		"+---",
		"+# New",
		"*** End Patch",
	}, "\n")

	ops, err := ParseApplyPatch(patch)
	if err != nil {
		t.Fatalf("ParseApplyPatch: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("ops = %d, want 1", len(ops))
	}
	op := ops[0]
	if op.Kind != PatchAdd || op.Path != "notes/new.md" {
		t.Fatalf("op = %+v, want Add notes/new.md", op)
	}
	want := []string{"---", "mode: living", "---", "# New"}
	if strings.Join(op.Added, "\n") != strings.Join(want, "\n") {
		t.Fatalf("added = %q, want %q", op.Added, want)
	}
}

func TestParseApplyPatchDeleteAndMove(t *testing.T) {
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Delete File: gone.md",
		"*** Update File: old.md",
		"*** Move to: new.md",
		"@@",
		" context",
		"-drop",
		"+keep",
		"*** End Patch",
	}, "\n")

	ops, err := ParseApplyPatch(patch)
	if err != nil {
		t.Fatalf("ParseApplyPatch: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("ops = %d, want 2", len(ops))
	}
	if ops[0].Kind != PatchDelete || ops[0].Path != "gone.md" {
		t.Fatalf("ops[0] = %+v, want Delete gone.md", ops[0])
	}
	if ops[1].Kind != PatchUpdate || ops[1].Path != "old.md" || ops[1].MoveTo != "new.md" {
		t.Fatalf("ops[1] = %+v, want Update old.md -> new.md", ops[1])
	}
}

func TestParseApplyPatchMalformed(t *testing.T) {
	cases := map[string]string{
		"missing begin": "*** Update File: x.md\n*** End Patch",
		"missing end":   "*** Begin Patch\n*** Add File: x.md\n+a",
		"add without +": "*** Begin Patch\n*** Add File: x.md\nnot an addition\n*** End Patch",
		"bare body":     "*** Begin Patch\n*** Update File: x.md\nno prefix line\n*** End Patch",
	}
	for name, patch := range cases {
		if _, err := ParseApplyPatch(patch); !errors.Is(err, ErrPatchMalformed) {
			t.Fatalf("%s: err = %v, want ErrPatchMalformed", name, err)
		}
	}
}

func TestApplyHunksInteriorReplace(t *testing.T) {
	old := []byte("# Log\n\nEntry one.\nEntry two.\n")
	hunks := []PatchHunk{{Lines: []PatchLine{
		{Kind: ' ', Text: ""},
		{Kind: '-', Text: "Entry one."},
		{Kind: '+', Text: "Edited one."},
		{Kind: ' ', Text: "Entry two."},
	}}}

	got, err := ApplyHunks(old, true, hunks)
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	want := "# Log\n\nEdited one.\nEntry two.\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestApplyHunksTailAppendPreservesPrefix(t *testing.T) {
	old := []byte("# Log\n\nEntry one.\n")
	hunks := []PatchHunk{{Lines: []PatchLine{
		{Kind: ' ', Text: "Entry one."},
		{Kind: '+', Text: "Entry two."},
	}}}

	got, err := ApplyHunks(old, true, hunks)
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	want := "# Log\n\nEntry one.\nEntry two.\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !strings.HasPrefix(string(got), string(old)) {
		t.Fatalf("tail append broke the old-bytes prefix: %q", got)
	}
}

func TestApplyHunksMissingFile(t *testing.T) {
	if _, err := ApplyHunks(nil, false, []PatchHunk{{}}); !errors.Is(err, ErrPatchUpdateMissing) {
		t.Fatalf("err = %v, want ErrPatchUpdateMissing", err)
	}
}

func TestApplyHunksContextNotFound(t *testing.T) {
	old := []byte("# Log\n\nEntry one.\n")
	hunks := []PatchHunk{{Lines: []PatchLine{
		{Kind: '-', Text: "no such line"},
		{Kind: '+', Text: "x"},
	}}}
	if _, err := ApplyHunks(old, true, hunks); !errors.Is(err, ErrPatchContextNotFound) {
		t.Fatalf("err = %v, want ErrPatchContextNotFound", err)
	}
}
