package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
)

type cliRuntimeStub struct {
	localUser *data.User
	subjects  *subject.Service
	close     func() error
}

func (stub *cliRuntimeStub) LocalUser() *data.User {
	return stub.localUser
}

func (stub *cliRuntimeStub) Subjects() *subject.Service {
	return stub.subjects
}

func (stub *cliRuntimeStub) Close() error {
	if stub.close == nil {
		return nil
	}
	return stub.close()
}

func TestCLI_DatabaseDSNPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		envDSN  string
		args    []string
		wantDSN string
	}{
		{
			name:    "uses default",
			args:    []string{"thoughts", "subjects", "create", "coding"},
			wantDSN: defaultSQLiteDSN,
		},
		{
			name:    "uses environment variable",
			envDSN:  "file:environment.db",
			args:    []string{"thoughts", "subjects", "create", "coding"},
			wantDSN: "file:environment.db",
		},
		{
			name:    "flag overrides environment variable",
			envDSN:  "file:environment.db",
			args:    []string{"thoughts", "--db-dsn", "file:flag.db", "subjects", "create", "coding"},
			wantDSN: "file:flag.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("THOUGHTS_DB_DSN", tt.envDSN)
			want := errors.New("stop after resolving DSN")
			var gotDSN string
			app := newApplication(io.Discard, io.Discard)
			app.openRuntime = func(_ context.Context, dsn string) (cliRuntime, error) {
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
	t.Run("opens runtime once and clears dependencies after command", func(t *testing.T) {
		openCalls := 0
		closeCalls := 0
		app := newApplication(io.Discard, io.Discard)
		app.openRuntime = func(context.Context, string) (cliRuntime, error) {
			openCalls++
			return &cliRuntimeStub{
				localUser: &data.User{UserID: "local-user"},
				close: func() error {
					closeCalls++
					return nil
				},
			}, nil
		}

		err := newCLI(app).Run(context.Background(), []string{
			"thoughts", "subjects", "create",
		})

		if err == nil || !strings.Contains(err.Error(), "exactly one subject name") {
			t.Fatalf("got error %v, want subject name argument error", err)
		}
		if openCalls != 1 {
			t.Fatalf("opened runtime %d times, want 1", openCalls)
		}
		if closeCalls != 1 {
			t.Fatalf("closed runtime %d times, want 1", closeCalls)
		}
		if app.runtime != nil || app.subjects != nil || app.userID != "" {
			t.Fatal("runtime dependencies were not cleared after command")
		}
	})

	t.Run("rejects obsolete user ID flag", func(t *testing.T) {
		openCalls := 0
		app := newApplication(io.Discard, io.Discard)
		app.openRuntime = func(context.Context, string) (cliRuntime, error) {
			openCalls++
			return nil, errors.New("unexpected database open")
		}

		err := newCLI(app).Run(context.Background(), []string{"thoughts", "--user-id", "user-1", "subjects", "create", "coding"})

		if err == nil || !strings.Contains(err.Error(), "user-id") {
			t.Fatalf("got error %v, want unsupported user ID flag error", err)
		}
		if openCalls != 0 {
			t.Fatalf("opened SQLite %d times, want 0", openCalls)
		}
	})

	t.Run("returns runtime open failure", func(t *testing.T) {
		want := errors.New("bootstrap failed")
		app := newApplication(io.Discard, io.Discard)
		app.openRuntime = func(context.Context, string) (cliRuntime, error) {
			return nil, want
		}

		err := newCLI(app).Run(context.Background(), []string{"thoughts", "subjects", "list"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})
}
