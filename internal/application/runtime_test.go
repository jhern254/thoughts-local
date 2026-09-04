package application

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	_ "modernc.org/sqlite"
)

func TestRuntime_Open(t *testing.T) {
	t.Run("opens and composes application dependencies", func(t *testing.T) {
		db, err := sql.Open("sqlite", "file::memory:")
		if err != nil {
			t.Fatal(err)
		}
		openCalls := 0
		ensureCalls := 0
		localUser := &data.User{UserID: "local-user"}

		runtime, err := open(
			context.Background(),
			"file:test.db",
			func(context.Context, string) (*sql.DB, error) {
				openCalls++
				return db, nil
			},
			func(context.Context, *sql.DB) (*data.User, error) {
				ensureCalls++
				return localUser, nil
			},
		)

		if err != nil {
			t.Fatal(err)
		}
		if openCalls != 1 || ensureCalls != 1 {
			t.Fatalf("got %d open calls and %d bootstrap calls, want 1 each", openCalls, ensureCalls)
		}
		if runtime.LocalUser() != localUser {
			t.Fatalf("got local user %#v, want %#v", runtime.LocalUser(), localUser)
		}
		if runtime.Subjects() == nil {
			t.Fatal("got nil subject service")
		}

		if err := runtime.Close(); err != nil {
			t.Fatal(err)
		}
		if err := db.Ping(); err == nil {
			t.Fatal("expected SQLite to be closed")
		}
	})

	t.Run("returns database open error", func(t *testing.T) {
		want := errors.New("open failed")
		ensureCalled := false

		_, err := open(
			context.Background(),
			"file:test.db",
			func(context.Context, string) (*sql.DB, error) {
				return nil, want
			},
			func(context.Context, *sql.DB) (*data.User, error) {
				ensureCalled = true
				return nil, nil
			},
		)

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if ensureCalled {
			t.Fatal("bootstrapped user after database open failure")
		}
	})

	t.Run("closes SQLite after bootstrap failure", func(t *testing.T) {
		db, err := sql.Open("sqlite", "file::memory:")
		if err != nil {
			t.Fatal(err)
		}
		want := errors.New("bootstrap failed")

		_, err = open(
			context.Background(),
			"file:test.db",
			func(context.Context, string) (*sql.DB, error) {
				return db, nil
			},
			func(context.Context, *sql.DB) (*data.User, error) {
				return nil, want
			},
		)

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if err := db.Ping(); err == nil {
			t.Fatal("expected SQLite to be closed after bootstrap failure")
		}
	})
}

func TestSQLiteDSNWithForeignKeys(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{name: "adds query", dsn: "file:thoughts.db", want: "file:thoughts.db?_pragma=foreign_keys(1)"},
		{name: "appends to query", dsn: "file:thoughts.db?mode=rwc", want: "file:thoughts.db?mode=rwc&_pragma=foreign_keys(1)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sqliteDSNWithForeignKeys(tt.dsn); got != tt.want {
				t.Fatalf("got DSN %q, want %q", got, tt.want)
			}
		})
	}
}
