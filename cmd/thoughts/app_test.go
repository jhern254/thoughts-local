package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestCLI_SubjectsCreate(t *testing.T) {
	t.Run("parses configuration and subject name", func(t *testing.T) {
		var out bytes.Buffer
		var gotDSN string
		creator := &subjectCreatorStub{create: func(_ context.Context, userID, name string) (*data.Subject, error) {
			if userID != "user-1" || name != "  learn Go  " {
				t.Fatalf("got user ID %q and name %q", userID, name)
			}
			return &data.Subject{SubjectID: 7, UserID: userID, SubjectName: name}, nil
		}}
		factory := func(_ context.Context, dsn string) (SubjectCreator, io.Closer, error) {
			gotDSN = dsn
			return creator, io.NopCloser(strings.NewReader("")), nil
		}

		err := newCLI(&out, factory).Run(context.Background(), []string{
			"thoughts", "--user-id", "user-1", "--db-dsn", "file:test.db", "subjects", "create", "  learn Go  ",
		})

		if err != nil {
			t.Fatal(err)
		}
		if gotDSN != "file:test.db" {
			t.Fatalf("got DSN %q, want %q", gotDSN, "file:test.db")
		}
		if got, want := out.String(), "Created subject 7:   learn Go  \n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
	})

	t.Run("requires user ID", func(t *testing.T) {
		factoryCalled := false
		factory := func(context.Context, string) (SubjectCreator, io.Closer, error) {
			factoryCalled = true
			return nil, nil, nil
		}

		err := newCLI(io.Discard, factory).Run(context.Background(), []string{"thoughts", "subjects", "create", "coding"})

		if err == nil {
			t.Fatal("expected missing user ID error")
		}
		if factoryCalled {
			t.Fatal("service factory called without required user ID")
		}
	})

	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "requires subject name", args: []string{"thoughts", "--user-id", "user-1", "subjects", "create"}},
		{name: "rejects extra arguments", args: []string{"thoughts", "--user-id", "user-1", "subjects", "create", "coding", "extra"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			factoryCalled := false
			factory := func(context.Context, string) (SubjectCreator, io.Closer, error) {
				factoryCalled = true
				return nil, nil, nil
			}

			err := newCLI(io.Discard, factory).Run(context.Background(), tt.args)

			if err == nil {
				t.Fatal("expected argument error")
			}
			if factoryCalled {
				t.Fatal("service factory called with invalid arguments")
			}
		})
	}

	t.Run("returns service factory error", func(t *testing.T) {
		want := errors.New("database unavailable")
		factory := func(context.Context, string) (SubjectCreator, io.Closer, error) {
			return nil, nil, want
		}

		err := newCLI(io.Discard, factory).Run(context.Background(), []string{
			"thoughts", "--user-id", "user-1", "subjects", "create", "coding",
		})

		if !errors.Is(err, want) {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})

	t.Run("returns database close error", func(t *testing.T) {
		want := errors.New("close failed")
		creator := &subjectCreatorStub{create: func(_ context.Context, userID, name string) (*data.Subject, error) {
			return &data.Subject{SubjectID: 1, UserID: userID, SubjectName: name}, nil
		}}
		factory := func(context.Context, string) (SubjectCreator, io.Closer, error) {
			return creator, errorCloser{err: want}, nil
		}

		err := newCLI(io.Discard, factory).Run(context.Background(), []string{
			"thoughts", "--user-id", "user-1", "subjects", "create", "coding",
		})

		if !errors.Is(err, want) {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})
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
			var gotDSN string
			creator := &subjectCreatorStub{create: func(_ context.Context, userID, name string) (*data.Subject, error) {
				return &data.Subject{SubjectID: 1, UserID: userID, SubjectName: name}, nil
			}}
			factory := func(_ context.Context, dsn string) (SubjectCreator, io.Closer, error) {
				gotDSN = dsn
				return creator, io.NopCloser(strings.NewReader("")), nil
			}

			if err := newCLI(io.Discard, factory).Run(context.Background(), tt.args); err != nil {
				t.Fatal(err)
			}
			if gotDSN != tt.wantDSN {
				t.Fatalf("got DSN %q, want %q", gotDSN, tt.wantDSN)
			}
		})
	}
}

type errorCloser struct {
	err error
}

func (c errorCloser) Close() error {
	return c.err
}
