package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
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
