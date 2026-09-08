// internal/data/errors.go
package data

import "errors"

var ErrRecordNotFound = errors.New("record not found")
var ErrDuplicateRecord = errors.New("record already exists")

// ErrDatabaseBusy identifies contention with another SQLite connection.
var ErrDatabaseBusy = errors.New("database busy")

// ErrDatabaseReadOnly identifies a write rejected by a read-only database.
var ErrDatabaseReadOnly = errors.New("database read-only")
