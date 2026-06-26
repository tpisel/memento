package enforce

import (
	"bytes"
	"errors"
	"fmt"
)

// Edit is one Claude Edit/MultiEdit operation: replace OldString with NewString,
// substituting every occurrence when ReplaceAll is set (otherwise the match must
// be unique). A single Edit is the one-element case of a MultiEdit.
type Edit struct {
	OldString  string
	NewString  string
	ReplaceAll bool
}

// Replay-abort sentinels. Each marks a case where Claude's Edit/MultiEdit would
// itself refuse to apply, so no faithful new-bytes can be derived. check-write
// turns any of these into a fail-closed deny rather than gating invented bytes.
var (
	// ErrReplayCreateViaEdit fires when an edit would have to create the file —
	// the target does not exist, or an edit carries an empty old_string. ADR-0031
	// reserves creation to Write; Edit/MultiEdit only mutate existing bytes.
	ErrReplayCreateViaEdit = errors.New("edits cannot create a file")
	// ErrReplayNoMatch fires when old_string is absent from the current content.
	ErrReplayNoMatch = errors.New("old_string not found")
	// ErrReplayAmbiguous fires when old_string matches more than once and
	// replace_all is unset — Claude aborts rather than guess which occurrence.
	ErrReplayAmbiguous = errors.New("old_string is not unique")
)

// ReplayEdits replays Claude's Edit/MultiEdit apply algorithm against old to
// derive the bytes that would land on disk: edits are applied in order, each
// against the result of the previous (ADR-0031). The replay is faithful to
// Claude's unpublished, version-drifting contract — a missing match aborts, a
// non-unique match aborts unless replace_all is set, and creation is reserved to
// Write — so the only correctness oracle is the captured golden fixtures, not a
// proof; the PostToolUse drift alarm is the runtime backstop for divergence.
func ReplayEdits(old []byte, exists bool, edits []Edit) ([]byte, error) {
	if !exists {
		return nil, ErrReplayCreateViaEdit
	}
	cur := old
	for i, e := range edits {
		if e.OldString == "" {
			return nil, fmt.Errorf("edit %d: %w", i, ErrReplayCreateViaEdit)
		}
		next, err := applyEdit(cur, e)
		if err != nil {
			return nil, fmt.Errorf("edit %d: %w", i, err)
		}
		cur = next
	}
	return cur, nil
}

// applyEdit performs a single replacement against content, enforcing Claude's
// uniqueness rule when replace_all is unset.
func applyEdit(content []byte, e Edit) ([]byte, error) {
	old := []byte(e.OldString)
	count := bytes.Count(content, old)
	if count == 0 {
		return nil, ErrReplayNoMatch
	}
	if e.ReplaceAll {
		return bytes.ReplaceAll(content, old, []byte(e.NewString)), nil
	}
	if count > 1 {
		return nil, fmt.Errorf("%w: matches %d times", ErrReplayAmbiguous, count)
	}
	return bytes.Replace(content, old, []byte(e.NewString), 1), nil
}
