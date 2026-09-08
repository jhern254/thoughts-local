//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestErrorContractsWorkflow_SQLite(t *testing.T) {
	t.Run("leaves unrecognized driver failures unchanged", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		_, err := db.Exec("SELECT * FROM missing_table")
		var cause *sqlite.Error
		if !errors.As(err, &cause) {
			t.Fatalf("cause: got %v, want *sqlite.Error", err)
		}
		if got := data.TranslateSQLiteError(err); got != err {
			t.Fatalf("translation: got %v, want original driver error", got)
		}
	})
	t.Run("recognizes a competing writer and preserves the driver cause", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		writer := openSQLite(t, dsn)
		if _, err := writer.Exec("BEGIN IMMEDIATE"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := writer.Exec("ROLLBACK"); err != nil {
				t.Error(err)
			}
		})
		_, err := data.NewSQLiteSubjectStore(db).CreateSubject(context.Background(), &data.Subject{UserID: "owner", SubjectName: "private subject"})
		assertDatabaseFailure(t, err, data.ErrDatabaseBusy, sqlite3.SQLITE_BUSY)
	})
	t.Run("recognizes a write through a read-only connection", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		readOnly := openSQLite(t, dsn+"?mode=ro")
		_, err := data.NewSQLiteSubjectStore(readOnly).CreateSubject(context.Background(), &data.Subject{UserID: "owner", SubjectName: "private subject"})
		assertDatabaseFailure(t, err, data.ErrDatabaseReadOnly, sqlite3.SQLITE_READONLY)
	})
	t.Run("propagates read-only local-user bootstrap failure", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(context.Background(), dsn+"?mode=ro")
		if runtime != nil {
			defer runtime.Close()
			t.Fatalf("runtime: got %v, want nil", runtime)
		}
		assertDatabaseFailure(t, err, data.ErrDatabaseReadOnly, sqlite3.SQLITE_READONLY)
	})
}

func assertDatabaseFailure(t *testing.T, err, want error, code int) {
	t.Helper()
	wrapped := fmt.Errorf("operation: %w", err)
	if !errors.Is(wrapped, want) {
		t.Fatalf("error: got %v, want errors.Is(%v)", wrapped, want)
	}
	var cause *sqlite.Error
	if !errors.As(wrapped, &cause) {
		t.Fatalf("cause: got %v, want *sqlite.Error", wrapped)
	}
	if got := cause.Code() & 0xff; got != code {
		t.Fatalf("SQLite code: got %d, want %d", got, code)
	}
	if got := data.TranslateSQLiteError(err); got != err {
		t.Fatalf("repeated translation: got %v, want original translated error", got)
	}
	// Translating a wrapped driver cause must preserve both identity and text.
	private := fmt.Errorf("PRIVATE-DRIVER-CAUSE: %w", cause)
	translated := data.TranslateSQLiteError(private)
	if !errors.Is(translated, private) || !errors.Is(translated, want) {
		t.Fatalf("translated error: got %v, want original cause and %v", translated, want)
	}
	if got, want := translated.Error(), private.Error(); got != want {
		t.Fatalf("error text: got %q, want %q", got, want)
	}
}
