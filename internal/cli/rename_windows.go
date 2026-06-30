//go:build windows

package cli

import (
	"errors"
	"syscall"
)

// errorSharingViolation is Win32 ERROR_SHARING_VIOLATION (32). syscall exports
// ERROR_ACCESS_DENIED but not the sharing-violation code, so it is named here rather
// than pulling in golang.org/x/sys/windows for a single constant.
const errorSharingViolation = syscall.Errno(32)

// retryableRenameErr reports whether a MoveFileEx replace failure is the transient,
// destination-momentarily-held kind renameReplace should retry rather than surface.
// ACCESS_DENIED and SHARING_VIOLATION are what a competing replace, an open reader,
// or an antivirus scan produces; any other error is a real failure and returned as-is.
func retryableRenameErr(err error) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, errorSharingViolation)
}
