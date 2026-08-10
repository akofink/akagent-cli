package store

import (
	"errors"
	"fmt"
)

// ErrorKind classifies store failures so callers can translate them into
// protocol errors or take direct recovery action.
type ErrorKind string

const (
	// KindUsage means the caller passed invalid input, such as an unsafe task ID.
	KindUsage ErrorKind = "usage"
	// KindNotFound means the requested record does not exist.
	KindNotFound ErrorKind = "not_found"
	// KindLocked means another writer holds the per-task lock.
	KindLocked ErrorKind = "lock_contention"
	// KindMalformed means a stored record is unreadable or uses an unsupported
	// schema version.
	KindMalformed ErrorKind = "malformed"
	// KindUnsafe means the on-disk store is accessible by other users.
	KindUnsafe ErrorKind = "unsafe_permissions"
	// KindUnsafePath means a store-owned path is a symbolic link or otherwise
	// escapes the configured state root.
	KindUnsafePath ErrorKind = "unsafe_path"
	// KindPartial means the intended state change only partially completed.
	KindPartial ErrorKind = "partial"
	// KindInternal means an unexpected storage failure occurred.
	KindInternal ErrorKind = "internal"
)

// Error is a typed store failure with recovery guidance. It implements error,
// so it can be returned and wrapped like any other Go error.
type Error struct {
	Kind      ErrorKind
	Message   string
	Retryable bool
	Recovery  string
	Err       error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap exposes the wrapped underlying cause, if any.
func (e *Error) Unwrap() error { return e.Err }

// IsKind reports whether err is a store Error of the given kind.
func IsKind(err error, kind ErrorKind) bool {
	var storeErr *Error
	return errors.As(err, &storeErr) && storeErr.Kind == kind
}

func newError(kind ErrorKind, message, recovery string) *Error {
	return &Error{Kind: kind, Message: message, Recovery: recovery}
}

func retryableError(message, recovery string) *Error {
	return &Error{Kind: KindLocked, Message: message, Retryable: true, Recovery: recovery}
}

func malformedError(message, recovery string) *Error {
	return &Error{Kind: KindMalformed, Message: message, Recovery: recovery}
}

func unsafeError(message, recovery string) *Error {
	return &Error{Kind: KindUnsafe, Message: message, Recovery: recovery}
}

func unsafePathError(message, recovery string) *Error {
	return &Error{Kind: KindUnsafePath, Message: message, Recovery: recovery}
}

func internalError(message, recovery string) *Error {
	return &Error{Kind: KindInternal, Message: message, Recovery: recovery}
}
