package data

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

type failedRow struct{ err error }

func (row failedRow) Scan(...any) error { return row.err }

func TestRowScanners_Errors(t *testing.T) {
	for _, scanner := range []struct {
		name string
		scan func(failedRow) error
	}{
		{"user", func(row failedRow) error { _, err := scanUser(row); return err }},
		{"subject", func(row failedRow) error { _, err := scanSubject(row); return err }},
		{"thought", func(row failedRow) error { _, err := scanThought(row); return err }},
		{"event", func(row failedRow) error { _, err := scanEvent(row); return err }},
		{"goal", func(row failedRow) error { _, err := scanGoal(row); return err }},
	} {
		t.Run(scanner.name+" preserves scan errors without assigning domain meaning", func(t *testing.T) {
			for _, want := range []error{sql.ErrNoRows, fmt.Errorf("wrapped: %w", sql.ErrNoRows), errors.New("scan failure")} {
				if got := scanner.scan(failedRow{err: want}); got != want {
					t.Fatalf("scan error: got %v, want original %v", got, want)
				}
			}
		})
	}
}
