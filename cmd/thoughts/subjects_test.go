package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type subjectServiceStub struct {
	create       func(context.Context, string, string) (*data.Subject, error)
	createCalled bool
}

func (s *subjectServiceStub) Create(ctx context.Context, userID, name string) (*data.Subject, error) {
	s.createCalled = true
	return s.create(ctx, userID, name)
}

func TestSubjectsCommand_Create(t *testing.T) {
	t.Run("creates subject", func(t *testing.T) {
		type contextKey string
		ctx := context.WithValue(context.Background(), contextKey("request"), "test-value")
		var out bytes.Buffer
		service := &subjectServiceStub{create: func(gotCtx context.Context, userID, name string) (*data.Subject, error) {
			if gotCtx.Value(contextKey("request")) != "test-value" || userID != "user-1" || name != "  learn Go  " {
				t.Fatalf("got context %v, user ID %q, and name %q", gotCtx, userID, name)
			}
			return &data.Subject{SubjectID: 7, UserID: userID, SubjectName: name}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(ctx, []string{"subjects", "create", "  learn Go  "})

		if err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "Created subject 7:   learn Go  \n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
	})

	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "requires subject name", args: []string{"subjects", "create"}},
		{name: "rejects extra arguments", args: []string{"subjects", "create", "coding", "extra"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &subjectServiceStub{create: func(context.Context, string, string) (*data.Subject, error) {
				return nil, errors.New("unexpected service call")
			}}
			app := &application{subjects: service, userID: "user-1", out: &bytes.Buffer{}}

			err := newSubjectsCommand(app).Run(context.Background(), tt.args)

			if err == nil {
				t.Fatal("expected argument error")
			}
			if service.createCalled {
				t.Fatal("service called with invalid arguments")
			}
		})
	}

	t.Run("returns service error", func(t *testing.T) {
		want := errors.New("create failed")
		var out bytes.Buffer
		service := &subjectServiceStub{create: func(context.Context, string, string) (*data.Subject, error) {
			return nil, want
		}}
		app := &application{subjects: service, userID: "user-1", out: &out}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "create", "coding"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if out.Len() != 0 {
			t.Fatalf("got unexpected output %q", out.String())
		}
	})

	t.Run("returns output error", func(t *testing.T) {
		want := errors.New("write failed")
		service := &subjectServiceStub{create: func(_ context.Context, userID, name string) (*data.Subject, error) {
			return &data.Subject{SubjectID: 7, UserID: userID, SubjectName: name}, nil
		}}
		app := &application{subjects: service, userID: "user-1", out: errorWriter{err: want}}

		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "create", "coding"})

		if !errors.Is(err, want) {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
