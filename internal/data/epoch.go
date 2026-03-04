// internal/data/epoch.go
package data

import (
	"database/sql"
	"fmt"
	"time"
)

// TimeFromUnixSec converts an epoch seconds integer to time.Time (UTC).
func TimeFromUnixSec(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}

// UnixSec returns epoch seconds for a time (UTC).
// TODO: add to insert, update logic
func UnixSec(t time.Time) int64 {
	if t.IsZero() {
		return time.Now().UTC().Unix()
	}
	return t.UTC().Unix()
}

// ScanUnixNullable is a small helper if you ever have nullable INTEGER timestamps.
// It scans into *int64 (which may be nil) and returns a zero time on NULL.
func ScanUnixNullable(src any) (time.Time, error) {
	switch v := src.(type) {
	case nil:
		return time.Time{}, nil
	case int64:
		return time.Unix(v, 0).UTC(), nil
	case []byte:
		// SQLite can sometimes hand back text; ignore here unless you use text timestamps.
		return time.Time{}, fmt.Errorf("unexpected []byte for epoch seconds")
	case string:
		return time.Time{}, fmt.Errorf("unexpected string for epoch seconds")
	default:
		return time.Time{}, fmt.Errorf("unsupported type %T for epoch seconds", src)
	}
}

// EnableSQLiteFK ensures SQLite enforces foreign keys. Call once after opening DB.
func EnableSQLiteFK(db *sql.DB) error {
	_, err := db.Exec(`PRAGMA foreign_keys = ON;`)
	return err
}

