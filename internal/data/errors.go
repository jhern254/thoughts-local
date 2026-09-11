// internal/data/errors.go
package data

import "errors"

var ErrRecordNotFound = errors.New("record not found")
var ErrDuplicateRecord = errors.New("record already exists")

// ErrEventOverlap rejects an interval intersecting another active event.
var ErrEventOverlap = errors.New("event overlaps another event")

// ErrInvalidEventInterval identifies reversed intervals, missing required ends,
// or an end supplied to StartEvent. Calendar/input validation belongs to services.
var ErrInvalidEventInterval = errors.New("invalid event interval")

// ErrEventVersionConflict requires reloading before an event edit can be saved.
var ErrEventVersionConflict = errors.New("event version changed")

// ErrEventStateConflict rejects ending an already completed event or using an
// edit to change between ongoing and completed states.
var ErrEventStateConflict = errors.New("event state does not permit this operation")

// ErrDatabaseBusy identifies contention with another SQLite connection.
var ErrDatabaseBusy = errors.New("database busy")

// ErrDatabaseReadOnly identifies a write rejected by a read-only database.
var ErrDatabaseReadOnly = errors.New("database read-only")

// ErrVersionConflict requires reloading a thought before updating or deleting it.
var ErrVersionConflict = errors.New("record version changed")
