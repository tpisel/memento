//go:build !windows

package cli

// retryableRenameErr is the non-Windows half of renameReplace's platform split: on
// POSIX rename(2) atomically replaces and never returns a transient sharing error,
// so nothing is retryable and renameReplace stays single-shot. See rename_windows.go
// for the case this exists to cover.
func retryableRenameErr(error) bool { return false }
