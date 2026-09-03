package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type subjectCreatorStub struct {
	create func(context.Context, string, string) (*data.Subject, error)
}

func (s *subjectCreatorStub) Create(ctx context.Context, userID, name string) (*data.Subject, error) {
	return s.create(ctx, userID, name)
}

func TestSubjectCreateCommand_Run(t *testing.T) {
	t.Run("creates subject and writes confirmation", func(t *testing.T) {
		type contextKey string
		ctx := context.WithValue(context.Background(), contextKey("request"), "test-value")
		var out bytes.Buffer
		creator := &subjectCreatorStub{create: func(gotCtx context.Context, userID, name string) (*data.Subject, error) {
			if gotCtx != ctx || userID != "user-1" || name != "  learn Go  " {
				t.Fatalf("got context %v, user ID %q, and name %q", gotCtx, userID, name)
			}
			return &data.Subject{SubjectID: 7, UserID: userID, SubjectName: name}, nil
		}}
		command := SubjectCreateCommand{subjects: creator, userID: "user-1", out: &out}

		err := command.Run(ctx, "  learn Go  ")

		if err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "Created subject 7:   learn Go  \n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
	})

	t.Run("returns service error without success output", func(t *testing.T) {
		want := errors.New("create failed")
		var out bytes.Buffer
		creator := &subjectCreatorStub{create: func(context.Context, string, string) (*data.Subject, error) {
			return nil, want
		}}
		command := SubjectCreateCommand{subjects: creator, userID: "user-1", out: &out}

		err := command.Run(context.Background(), "coding")

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if out.Len() != 0 {
			t.Fatalf("got unexpected output %q", out.String())
		}
	})

	t.Run("returns output error", func(t *testing.T) {
		want := errors.New("write failed")
		creator := &subjectCreatorStub{create: func(_ context.Context, userID, name string) (*data.Subject, error) {
			return &data.Subject{SubjectID: 7, UserID: userID, SubjectName: name}, nil
		}}
		command := SubjectCreateCommand{subjects: creator, userID: "user-1", out: errorWriter{err: want}}

		err := command.Run(context.Background(), "coding")

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
