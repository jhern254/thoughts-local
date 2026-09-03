package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCLI_DatabaseDSNPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		envDSN  string
		args    []string
		wantDSN string
	}{
		{
			name:    "uses default",
			args:    []string{"thoughts", "--user-id", "user-1", "subjects", "create", "coding"},
			wantDSN: defaultSQLiteDSN,
		},
		{
			name:    "uses environment variable",
			envDSN:  "file:environment.db",
			args:    []string{"thoughts", "--user-id", "user-1", "subjects", "create", "coding"},
			wantDSN: "file:environment.db",
		},
		{
			name:    "flag overrides environment variable",
			envDSN:  "file:environment.db",
			args:    []string{"thoughts", "--user-id", "user-1", "--db-dsn", "file:flag.db", "subjects", "create", "coding"},
			wantDSN: "file:flag.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("THOUGHTS_DB_DSN", tt.envDSN)
			want := errors.New("stop after resolving DSN")
			var gotDSN string
			app := newApplication(io.Discard, io.Discard)
			app.openDatabase = func(_ context.Context, dsn string) (*sql.DB, error) {
				gotDSN = dsn
				return nil, want
			}

			err := newCLI(app).Run(context.Background(), tt.args)

			if err != want {
				t.Fatalf("got error %v, want %v", err, want)
			}
			if gotDSN != tt.wantDSN {
				t.Fatalf("got DSN %q, want %q", gotDSN, tt.wantDSN)
			}
		})
	}
}

func TestCLI_RuntimeLifecycle(t *testing.T) {
	t.Run("opens and closes SQLite once around command", func(t *testing.T) {
		db, err := sql.Open("sqlite", "file::memory:")
		if err != nil {
			t.Fatal(err)
		}
		openCalls := 0
		app := newApplication(io.Discard, io.Discard)
		app.openDatabase = func(context.Context, string) (*sql.DB, error) {
			openCalls++
			return db, nil
		}

		err = newCLI(app).Run(context.Background(), []string{
			"thoughts", "--user-id", "user-1", "subjects", "create",
		})

		if err == nil || !strings.Contains(err.Error(), "exactly one subject name") {
			t.Fatalf("got error %v, want subject name argument error", err)
		}
		if openCalls != 1 {
			t.Fatalf("opened SQLite %d times, want 1", openCalls)
		}
		if err := db.Ping(); err == nil {
			t.Fatal("expected SQLite to be closed after command")
		}
	})

	t.Run("requires user ID", func(t *testing.T) {
		db, err := sql.Open("sqlite", "file::memory:")
		if err != nil {
			t.Fatal(err)
		}
		app := newApplication(io.Discard, io.Discard)
		app.openDatabase = func(context.Context, string) (*sql.DB, error) {
			return db, nil
		}

		err = newCLI(app).Run(context.Background(), []string{"thoughts", "subjects", "create", "coding"})

		if err == nil || !strings.Contains(err.Error(), "user-id") {
			t.Fatalf("got error %v, want missing user ID error", err)
		}
		if err := db.Ping(); err == nil {
			t.Fatal("expected SQLite to be closed after command")
		}
	})
}
