package orient

import (
	"bytes"
	"testing"
)

func TestBaselineContainsTriggeredPreconditionsMarker(t *testing.T) {
	marker := []byte("<!-- memento:triggered-preconditions -->")
	if got := bytes.Count(Baseline(), marker); got != 1 {
		t.Fatalf("Baseline marker count = %d, want 1", got)
	}
}
