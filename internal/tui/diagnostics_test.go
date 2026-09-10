package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
)

func TestSubjectModel_DiagnosticOutput(t *testing.T) {
	t.Run("retains error while rendering safe list failure", func(t *testing.T) {
		failure := errors.New("PRIVATE-DIAGNOSTIC-MARKER")
		model := newSubjectTestModel(&subjectServiceStub{})
		updated, _ := model.handleSubjectsListed(subjectsListedMsg{err: failure})
		got := updated.(Model)
		if !errors.Is(got.subjects.err, failure) {
			t.Fatalf("got error %v, want original error", got.subjects.err)
		}
		view := got.viewSubjectList()
		if strings.Contains(view, failure.Error()) {
			t.Fatal("got private error in view, want safe diagnostic")
		}
		if !strings.Contains(view, "Could not list subjects.") {
			t.Fatalf("got view %q, want safe list error", view)
		}
	})
}

func TestSubjectModel_FormDiagnostics(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"wrapped duplicate", fmt.Errorf(private+": %w", data.ErrDuplicateRecord), "A subject with that name already exists."},
		{"mixed failure", errors.Join(data.ErrDuplicateRecord, errors.New(private)), "Could not save the subject."},
	} {
		t.Run(tc.name+" preserves input and displays safe status", func(t *testing.T) {
			model := newSubjectTestModel(&subjectServiceStub{})
			model.subjects.input.SetValue(private)
			updated, _ := model.handleSubjectCreated(subjectCreatedMsg{err: tc.err})
			got := updated.(Model)
			if !errors.Is(got.subjects.err, tc.err) {
				t.Fatalf("got error %v, want original error", got.subjects.err)
			}
			if got.subjects.errMessage != tc.want {
				t.Fatalf("got diagnostic %q, want %q", got.subjects.errMessage, tc.want)
			}
			if value := got.subjects.input.Value(); value != private {
				t.Fatalf("got form input %q, want %q", value, private)
			}
			if view := got.viewSubjectCreate(); !strings.Contains(view, tc.want) {
				t.Fatalf("got view %q, want safe status %q", view, tc.want)
			}
		})
	}
}

func TestSubjectModel_ValidatorFeedback(t *testing.T) {
	t.Run("shows a safe rule and retains private editing input and original error", func(t *testing.T) {
		const private = "PRIVATE-VALIDATION-MARKER"
		input := strings.Repeat(private, 20)
		_, original := subject.NewService(nil).Create(context.Background(), "local", input)
		failure := fmt.Errorf(private+": %w", original)
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		model := newSubjectTestModel(&subjectServiceStub{})
		model.logger = logger
		model.subjects.input.SetValue(input)
		updated, _ := model.handleSubjectCreated(subjectCreatedMsg{err: failure})
		got := updated.(Model)
		if !errors.Is(got.subjects.err, original) {
			t.Fatalf("got error %v, want original error", got.subjects.err)
		}
		want := "subject_name: must be between 1 and 255 characters long"
		if got.subjects.errMessage != want {
			t.Fatalf("got diagnostic %q, want %q", got.subjects.errMessage, want)
		}
		if !strings.Contains(got.viewSubjectCreate(), want) {
			t.Fatal("got no validation rule in view, want actionable guidance")
		}
		if got.subjects.input.Value() != input {
			t.Fatal("got changed form content, want original input")
		}
		if strings.Contains(got.subjects.errMessage+logs.String(), private) || strings.Contains(logs.String(), "subject_name") {
			t.Fatal("got private input or validation payload in diagnostics/logs, want safe rule and metadata")
		}
	})
}
