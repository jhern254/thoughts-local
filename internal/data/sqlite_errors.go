package data

import (
	"errors"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// TranslateSQLiteError adds a stable identity without changing the error text
// or losing the driver cause. Unrecognized errors pass through unchanged.
func TranslateSQLiteError(err error) error {
	// Do not translate just one branch of a combined failure, or rewrap an
	// already translated cause. Classification handles combined failures safely.
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		switch cause.(type) {
		case interface{ Unwrap() []error }, *databaseError:
			return err
		}
	}
	var cause *sqlite.Error
	if !errors.As(err, &cause) {
		return err
	}
	var kind error
	// Extended result codes retain the primary code in the low eight bits.
	switch cause.Code() & 0xff {
	case sqlite3.SQLITE_BUSY:
		kind = ErrDatabaseBusy
	case sqlite3.SQLITE_READONLY:
		kind = ErrDatabaseReadOnly
	default:
		return err
	}
	return &databaseError{cause: err, kind: kind}
}

type databaseError struct {
	cause error
	kind  error
}

func (err *databaseError) Error() string        { return err.cause.Error() }
func (err *databaseError) Unwrap() error        { return err.cause }
func (err *databaseError) Is(target error) bool { return target == err.kind }
