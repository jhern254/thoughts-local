package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
	cli "github.com/urfave/cli/v3"
)

func TestCLI_DiagnosticOutput(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	t.Run("framework usage errors do not echo arguments", func(t *testing.T) {
		var output, diagnostics bytes.Buffer
		app := newApplication(&output, &diagnostics, logging.Nop())
		err := newCLI(app).Run(context.Background(), []string{"thoughts", "--" + private})
		if err == nil {
			t.Fatal("got nil error, want usage error")
		}
		if strings.Contains(output.String()+diagnostics.String(), private) {
			t.Fatal("got private flag in output, want safe usage")
		}
	})
	t.Run("runtime errors remain available without printing them", func(t *testing.T) {
		failure := errors.New(private)
		var diagnostics bytes.Buffer
		app := newApplication(io.Discard, &diagnostics, logging.Nop())
		app.openRuntime = func(context.Context, string) (cliRuntime, error) { return nil, failure }
		err := newCLI(app).Run(context.Background(), []string{"thoughts", "subjects", "list"})
		if !errors.Is(err, failure) {
			t.Fatalf("got error %v, want original error", err)
		}
		if strings.Contains(diagnostics.String(), private) {
			t.Fatal("got private error in diagnostics, want safe output")
		}
	})
}

func TestCLI_SubjectDiagnostics(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	for _, tc := range []struct {
		name   string
		err    error
		want   string
		logged bool
	}{
		{"wrapped missing", fmt.Errorf(private+": %w", data.ErrRecordNotFound), "The requested resource was not found.", false},
		{"wrapped duplicate", fmt.Errorf(private+": %w", data.ErrDuplicateRecord), "A subject with that name already exists.", false},
		{"unknown", errors.New(private), "Could not save the subject.", true},
		{"mixed", errors.Join(data.ErrDuplicateRecord, errors.New(private)), "Could not complete the command.", true},
		{"controlled validation", &subject.ValidationError{Fields: map[string]string{"subject_name": "must be between 1 and 255 characters long"}}, "Subject name must be between 1 and 255 characters long.", false},
		{"untrusted validation", &subject.ValidationError{Fields: map[string]string{private: private}}, "The subject details are invalid.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output, diagnostics, logs bytes.Buffer
			app := newApplication(&output, &diagnostics, newTestLogger(t, &logs))
			app.subjects = &subjectServiceStub{create: func(context.Context, string, string) (*data.Subject, error) { return nil, tc.err }}
			command := newSubjectsCommand(app)
			app.configureDiagnostics(command)
			err := command.Run(context.Background(), []string{"subjects", "create", private})
			if !errors.Is(err, tc.err) {
				t.Fatalf("got error %v, want original error", err)
			}
			if got, want := app.reportError(err), 1; got != want {
				t.Fatalf("got exit code %d, want %d", got, want)
			}
			if got, want := diagnostics.String(), tc.want+"\n"; got != want {
				t.Fatalf("got diagnostic %q, want %q", got, want)
			}
			if strings.Contains(logs.String(), private) {
				t.Fatal("got private marker in logs, want metadata only")
			}
			wantEvents := 0
			if tc.logged {
				wantEvents = 1
			}
			if got := strings.Count(logs.String(), "operation failed"); got != wantEvents {
				t.Fatalf("got failure events %d, want %d", got, wantEvents)
			}
			if output.Len() != 0 {
				t.Fatalf("got normal output %q, want empty on failure", output.String())
			}
		})
	}
	t.Run("preserves requested subject name separately from diagnostics", func(t *testing.T) {
		var output, diagnostics, logs bytes.Buffer
		app := newApplication(&output, &diagnostics, newTestLogger(t, &logs))
		app.subjects = &subjectServiceStub{get: func(context.Context, string, int64) (*data.Subject, error) {
			return &data.Subject{SubjectID: 1, SubjectName: private}, nil
		}}
		if err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "get", "1"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), private) {
			t.Fatalf("got output %q, want requested subject name", output.String())
		}
		if strings.Contains(diagnostics.String()+logs.String(), private) {
			t.Fatal("got content in diagnostics, want separate output")
		}
	})
	t.Run("invalid IDs are not echoed", func(t *testing.T) {
		var diagnostics bytes.Buffer
		app := newApplication(io.Discard, &diagnostics, logging.Nop())
		err := newSubjectsCommand(app).Run(context.Background(), []string{"subjects", "get", private})
		if err == nil {
			t.Fatal("got nil error, want invalid ID")
		}
		app.reportError(err)
		if got, want := diagnostics.String(), "Subject ID must be a positive integer.\n"; got != want {
			t.Fatalf("got diagnostic %q, want %q", got, want)
		}
	})
}

func TestCLI_ShutdownDiagnostics(t *testing.T) {
	t.Run("combined action and shutdown failure retains errors and exit code", func(t *testing.T) {
		const private = "PRIVATE-DIAGNOSTIC-MARKER"
		actionErr := cli.Exit(private, 7)
		closeErr := errors.New(private + " close")
		var output, diagnostics bytes.Buffer
		app := newApplication(&output, &diagnostics, logging.Nop())
		app.openRuntime = func(context.Context, string) (cliRuntime, error) {
			return &cliRuntimeStub{localUser: &data.User{UserID: "local"}, close: func() error { return closeErr }}, nil
		}
		command := newCLI(app)
		command.Action = func(context.Context, *cli.Command) error { return actionErr }
		err := command.Run(context.Background(), []string{"thoughts"})
		multi, ok := err.(cli.MultiError)
		if !ok {
			t.Fatalf("got error %T, want framework aggregate", err)
		}
		var foundAction, foundClose bool
		for _, child := range multi.Errors() {
			foundAction = foundAction || errors.Is(child, actionErr)
			foundClose = foundClose || errors.Is(child, closeErr)
		}
		if !foundAction || !foundClose {
			t.Fatalf("got retained action=%t close=%t, want both true", foundAction, foundClose)
		}
		if got, want := app.reportError(err), 7; got != want {
			t.Fatalf("got exit code %d, want %d", got, want)
		}
		if got, want := diagnostics.String(), "Could not complete the command.\n"; got != want {
			t.Fatalf("got diagnostic %q, want %q", got, want)
		}
		if strings.Contains(output.String(), private) {
			t.Fatal("got private framework output, want no raw error")
		}
	})
}
