package vault

import (
	"errors"
	"fmt"
)

// Sentinel errors for vault operations.
var (
	ErrPathTraversal   = errors.New("path traversal attempt")
	ErrSymlinkEscape   = errors.New("symlink escapes vault boundary")
	ErrNotFound        = errors.New("not found")
	ErrConfirmMismatch = errors.New("confirmation path does not match")
	ErrAlreadyExists   = errors.New("already exists")
	ErrAmbiguousPath   = errors.New("ambiguous path: multiple case-insensitive matches")
	ErrPathRestricted  = errors.New("path is restricted")
)

// PathError records an error and the path and operation that caused it.
// It mirrors the structure of os.PathError.
type PathError struct {
	Op   string // e.g. "read", "write", "resolve"
	Path string // the problematic path
	Err  error  // the underlying sentinel error
}

// Error returns a string in the format "<op> <path>: <err message>".
func (e *PathError) Error() string {
	return fmt.Sprintf("%s %s: %s", e.Op, e.Path, e.Err.Error())
}

// Unwrap returns the underlying error, enabling errors.Is and errors.As.
func (e *PathError) Unwrap() error {
	return e.Err
}
