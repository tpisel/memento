package enforce

import (
	"errors"
	"fmt"
	"strings"
)

// codex apply_patch envelope directives. The envelope is the line-oriented patch
// format codex emits for structured edits: a `*** Begin Patch` / `*** End Patch`
// frame wrapping one or more file sections (ADR-0031 — codex's derivable-write
// class, the analogue of Claude's Edit/MultiEdit).
const (
	patchBegin   = "*** Begin Patch"
	patchEnd     = "*** End Patch"
	patchAddPfx  = "*** Add File: "
	patchUpdPfx  = "*** Update File: "
	patchDelPfx  = "*** Delete File: "
	patchMovePfx = "*** Move to: "
)

// Patch-derivation abort sentinels. Like the replay sentinels, each marks a case
// where no faithful new-bytes can be derived from the envelope + disk, so
// check-write fails the verdict closed rather than gating invented bytes.
var (
	// ErrPatchMalformed marks an envelope check-write cannot parse — a missing
	// frame marker, a body line without a recognised prefix, or an add-file line
	// that is not an addition.
	ErrPatchMalformed = errors.New("malformed apply_patch envelope")
	// ErrPatchUpdateMissing fires when an Update File section targets a file that
	// does not exist on disk: the hunks have nothing to apply against.
	ErrPatchUpdateMissing = errors.New("apply_patch updates a file that does not exist")
	// ErrPatchContextNotFound fires when a hunk's context/removed lines are absent
	// from the current file, so the hunk cannot be located — the patch would not
	// apply, and any derived bytes would be invented.
	ErrPatchContextNotFound = errors.New("apply_patch hunk context not found in file")
)

// PatchOpKind is the operation a single apply_patch file section performs.
type PatchOpKind int

const (
	PatchAdd PatchOpKind = iota
	PatchUpdate
	PatchDelete
)

// PatchOp is one file section of an apply_patch envelope: the operation, its
// target path (vault-relative resolution is the caller's job), an optional
// rename target for Update, the added lines for Add, and the hunks for Update.
type PatchOp struct {
	Kind   PatchOpKind
	Path   string
	MoveTo string
	Added  []string
	Hunks  []PatchHunk
}

// PatchHunk is one `@@`-delimited group of body lines within an Update section.
type PatchHunk struct {
	Lines []PatchLine
}

// PatchLine is one body line of a hunk: context (Kind ' '), removed ('-'), or
// added ('+'). Text is the line with its one-byte prefix stripped.
type PatchLine struct {
	Kind byte
	Text string
}

// ParseApplyPatch parses a codex apply_patch envelope into its file sections.
// Lines outside the `*** Begin Patch` / `*** End Patch` frame are ignored; a
// missing frame, an unrecognised body line, or an add-file line without a '+'
// is a malformed envelope (fail-closed for the caller).
func ParseApplyPatch(text string) ([]PatchOp, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) != patchBegin {
		i++
	}
	if i >= len(lines) {
		return nil, fmt.Errorf("%w: missing %q", ErrPatchMalformed, patchBegin)
	}
	i++ // step past the begin marker

	var ops []PatchOp
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.TrimSpace(line) == patchEnd:
			return ops, nil

		case strings.HasPrefix(line, patchAddPfx):
			op := PatchOp{Kind: PatchAdd, Path: strings.TrimSpace(line[len(patchAddPfx):])}
			i++
			for i < len(lines) && !isPatchDirective(lines[i]) {
				l := lines[i]
				if !strings.HasPrefix(l, "+") {
					return nil, fmt.Errorf("%w: add-file line %q lacks a '+'", ErrPatchMalformed, l)
				}
				op.Added = append(op.Added, l[1:])
				i++
			}
			ops = append(ops, op)

		case strings.HasPrefix(line, patchDelPfx):
			ops = append(ops, PatchOp{Kind: PatchDelete, Path: strings.TrimSpace(line[len(patchDelPfx):])})
			i++

		case strings.HasPrefix(line, patchUpdPfx):
			op := PatchOp{Kind: PatchUpdate, Path: strings.TrimSpace(line[len(patchUpdPfx):])}
			i++
			if i < len(lines) && strings.HasPrefix(lines[i], patchMovePfx) {
				op.MoveTo = strings.TrimSpace(lines[i][len(patchMovePfx):])
				i++
			}
			hunks, next, err := parseUpdateHunks(lines, i)
			if err != nil {
				return nil, err
			}
			op.Hunks = hunks
			i = next
			ops = append(ops, op)

		default:
			return nil, fmt.Errorf("%w: unexpected line %q", ErrPatchMalformed, line)
		}
	}
	return nil, fmt.Errorf("%w: missing %q", ErrPatchMalformed, patchEnd)
}

// parseUpdateHunks consumes the body of an Update section starting at index from,
// grouping body lines into hunks at each `@@` marker, and returns the hunks plus
// the index of the next directive (or end of input).
func parseUpdateHunks(lines []string, from int) ([]PatchHunk, int, error) {
	var hunks []PatchHunk
	started := false
	i := from
	for i < len(lines) && !isPatchDirective(lines[i]) {
		l := lines[i]
		if strings.HasPrefix(l, "@@") {
			hunks = append(hunks, PatchHunk{})
			started = true
			i++
			continue
		}
		if !started {
			hunks = append(hunks, PatchHunk{})
			started = true
		}
		pl, err := parsePatchBodyLine(l)
		if err != nil {
			return nil, 0, err
		}
		hunks[len(hunks)-1].Lines = append(hunks[len(hunks)-1].Lines, pl)
		i++
	}
	return hunks, i, nil
}

// parsePatchBodyLine classifies one hunk body line by its leading byte. An empty
// line is a blank context line; any other leading byte is malformed.
func parsePatchBodyLine(l string) (PatchLine, error) {
	if l == "" {
		return PatchLine{Kind: ' ', Text: ""}, nil
	}
	switch l[0] {
	case ' ':
		return PatchLine{Kind: ' ', Text: l[1:]}, nil
	case '-':
		return PatchLine{Kind: '-', Text: l[1:]}, nil
	case '+':
		return PatchLine{Kind: '+', Text: l[1:]}, nil
	}
	return PatchLine{}, fmt.Errorf("%w: hunk line %q lacks a context/'-'/'+' prefix", ErrPatchMalformed, l)
}

func isPatchDirective(l string) bool { return strings.HasPrefix(l, "*** ") }

// ApplyHunks derives the bytes an Update section would land by applying its hunks
// against old. Each hunk's context+removed lines are located as a contiguous block
// (searching forward from the previous hunk), then replaced with its context+added
// lines. A missing file or an unlocatable hunk aborts so the caller fails closed.
// Line splitting is round-trip exact (split/join on "\n"), so a file's trailing
// newline is preserved.
func ApplyHunks(old []byte, exists bool, hunks []PatchHunk) ([]byte, error) {
	if !exists {
		return nil, ErrPatchUpdateMissing
	}
	lines := strings.Split(string(old), "\n")
	cursor := 0
	for _, h := range hunks {
		var search, replace []string
		for _, pl := range h.Lines {
			switch pl.Kind {
			case ' ':
				search = append(search, pl.Text)
				replace = append(replace, pl.Text)
			case '-':
				search = append(search, pl.Text)
			case '+':
				replace = append(replace, pl.Text)
			}
		}
		idx := indexOfLines(lines, search, cursor)
		if idx < 0 {
			return nil, ErrPatchContextNotFound
		}
		out := make([]string, 0, len(lines)-len(search)+len(replace))
		out = append(out, lines[:idx]...)
		out = append(out, replace...)
		out = append(out, lines[idx+len(search):]...)
		lines = out
		cursor = idx + len(replace)
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// indexOfLines returns the first index >= from where needle occurs as a contiguous
// run within hay, or -1. An empty needle (a pure insertion) anchors at from.
func indexOfLines(hay, needle []string, from int) int {
	if len(needle) == 0 {
		if from > len(hay) {
			return len(hay)
		}
		return from
	}
	for i := from; i+len(needle) <= len(hay); i++ {
		match := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
