package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
)

func TestSubjectModel_Edit(t *testing.T) {
	t.Run("opens a prefilled form and cancels without changing the subject", func(t *testing.T) {
		model := newSubjectTestModel(&subjectServiceStub{})
		model.screen = screenSubjectDetail
		model.subjects.selected = &data.Subject{SubjectID: 7, SubjectName: "original"}
		updated, _ := model.Update(runeKey('e'))
		model = updated.(Model)
		if got := model.subjects.input.Value(); got != "original" || !strings.Contains(model.View().Content, "Edit subject") {
			t.Fatalf("got input %q and view %q, want prefilled edit form", got, model.View().Content)
		}
		model.subjects.input.SetValue("changed")
		updated, cmd := model.Update(escapeKey())
		model = updated.(Model)
		if cmd != nil || model.screen != screenSubjectDetail || model.subjects.selected.SubjectName != "original" {
			t.Fatal("got mutation or wrong screen after cancel, want original detail")
		}
	})
	t.Run("saves through the service and refreshes the list on return", func(t *testing.T) {
		service := &subjectServiceStub{update: func(_ context.Context, user string, id int64, name string) (*data.Subject, error) {
			if user != "local-user-id" || id != 7 || name != "changed" {
				t.Fatalf("got update (%q, %d, %q), want local user, 7, changed", user, id, name)
			}
			return &data.Subject{SubjectID: id, SubjectName: name}, nil
		}, list: func(context.Context, string) ([]data.Subject, error) {
			return []data.Subject{{SubjectID: 7, SubjectName: "changed"}}, nil
		}}
		model := newSubjectTestModel(service)
		model.screen = screenSubjectDetail
		model.subjects.selected = &data.Subject{SubjectID: 7, SubjectName: "original"}
		updated, _ := model.Update(runeKey('e'))
		model = updated.(Model)
		model.subjects.input.SetValue("changed")
		model = runModelCommand(t, model, enterKey())
		if model.subjects.selected.SubjectName != "changed" || !strings.Contains(model.View().Content, "Updated subject") {
			t.Fatalf("got view %q, want updated detail", model.View().Content)
		}
		model = runModelCommand(t, model, escapeKey())
		if model.subjects.listStale || !strings.Contains(model.View().Content, "changed") {
			t.Fatalf("got view %q, want refreshed list", model.View().Content)
		}
	})
}

func TestSubjectModel_Delete(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{escapeKey(), runeKey('n')} {
		t.Run("cancels confirmation with "+key.String(), func(t *testing.T) {
			model := newSubjectTestModel(&subjectServiceStub{})
			model.screen = screenSubjectDetail
			model.subjects.selected = &data.Subject{SubjectID: 7, SubjectName: "keep"}
			updated, cmd := model.Update(runeKey('d'))
			model = updated.(Model)
			if cmd != nil || !strings.Contains(model.View().Content, "Existing thoughts will be kept") {
				t.Fatalf("got view %q, want confirmation without mutation", model.View().Content)
			}
			updated, cmd = model.Update(enterKey())
			model = updated.(Model)
			if cmd != nil || model.screen != screenSubjectDelete {
				t.Fatal("got Enter confirmation, want no mutation")
			}
			updated, cmd = model.Update(key)
			model = updated.(Model)
			if cmd != nil || model.screen != screenSubjectDetail || model.subjects.selected.SubjectName != "keep" {
				t.Fatal("got changed selection after cancellation, want original detail")
			}
		})
	}
	t.Run("confirms deletion and reloads the list", func(t *testing.T) {
		service := &subjectServiceStub{delete: func(_ context.Context, user string, id int64) error {
			if user != "local-user-id" || id != 7 {
				t.Fatalf("got delete (%q,%d), want local user and 7", user, id)
			}
			return nil
		}, list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }}
		model := newSubjectTestModel(service)
		model.screen = screenSubjectDetail
		model.subjects.selected = &data.Subject{SubjectID: 7, SubjectName: "remove"}
		updated, _ := model.Update(runeKey('d'))
		model = updated.(Model)
		updated, cmd := model.Update(runeKey('y'))
		model = updated.(Model)
		if cmd == nil {
			t.Fatal("got nil command, want delete")
		}
		updated, refresh := model.Update(cmd())
		model = updated.(Model)
		if model.subjects.selected != nil || model.screen != screenSubjectList || refresh == nil {
			t.Fatal("got stale selection or no refresh, want list reload")
		}
		model = applyCommand(t, model, refresh)
		if strings.Contains(model.View().Content, "remove") || model.subjects.listStale {
			t.Fatal("got deleted subject in list, want refreshed empty list")
		}
	})
}

func TestSubjectModel_Mutations(t *testing.T) {
	for _, action := range []struct {
		name         string
		open, submit tea.KeyPressMsg
	}{
		{"update", runeKey('e'), enterKey()}, {"delete", runeKey('d'), runeKey('y')},
	} {
		t.Run(action.name+" prevents repeated submissions and navigation while running", func(t *testing.T) {
			model := newSubjectTestModel(&subjectServiceStub{})
			model.screen = screenSubjectDetail
			model.subjects.selected = &data.Subject{SubjectID: 7, SubjectName: "private input"}
			updated, _ := model.Update(action.open)
			model = updated.(Model)
			updated, cmd := model.Update(action.submit)
			model = updated.(Model)
			if cmd == nil || !model.subjects.loading {
				t.Fatal("got no pending mutation, want loading command")
			}
			screen := model.screen
			for _, key := range []tea.KeyPressMsg{action.submit, escapeKey(), runeKey('n'), runeKey('e'), runeKey('d')} {
				updated, cmd = model.Update(key)
				model = updated.(Model)
				if cmd != nil || model.screen != screen {
					t.Fatal("got action while loading, want blocked navigation/submission")
				}
			}
			_, quit := model.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
			if quit == nil {
				t.Fatal("got nil quit command, want global Ctrl+C")
			}
		})
		t.Run(action.name+" preserves failures and logs metadata without authored content", func(t *testing.T) {
			const private = "PRIVATE-SUBJECT-ACTION-MARKER"
			failure := errors.New(private)
			service := &subjectServiceStub{update: func(context.Context, string, int64, string) (*data.Subject, error) { return nil, failure }, delete: func(context.Context, string, int64) error { return failure }}
			var logs bytes.Buffer
			logger, err := logging.New(&logs, "test", "info")
			if err != nil {
				t.Fatal(err)
			}
			model := newSubjectTestModel(service)
			model.logger = logger
			model.screen = screenSubjectDetail
			model.subjects.selected = &data.Subject{SubjectID: 7, SubjectName: private}
			updated, _ := model.Update(action.open)
			model = updated.(Model)
			screen := model.screen
			model = runModelCommand(t, model, action.submit)
			if !errors.Is(model.subjects.err, failure) || model.subjects.loading || model.screen != screen {
				t.Fatal("got lost failure state, want original error and retry screen")
			}
			if action.name == "update" && model.subjects.input.Value() != private {
				t.Fatal("got changed edit input, want original content")
			}
			if strings.Contains(model.subjects.errMessage+logs.String(), private) {
				t.Fatal("got private diagnostic, want fixed message and metadata")
			}
			if !strings.Contains(logs.String(), "subject_"+action.name) || strings.Count(logs.String(), "operation failed") != 1 {
				t.Fatalf("got logs %q, want one operation failure", logs.String())
			}
			if !strings.Contains(model.View().Content, model.subjects.errMessage) {
				t.Fatal("got missing diagnostic in view, want safe failure")
			}
		})
	}
	t.Run("edit validation preserves actionable feedback and permits retry", func(t *testing.T) {
		originalService := subject.NewService(nil)
		service := &subjectServiceStub{update: func(ctx context.Context, user string, id int64, name string) (*data.Subject, error) {
			if name == "corrected" {
				return &data.Subject{SubjectID: id, SubjectName: name}, nil
			}
			return originalService.Update(ctx, user, id, name)
		}}
		model := newSubjectTestModel(service)
		model.screen = screenSubjectDetail
		model.subjects.selected = &data.Subject{SubjectID: 7, SubjectName: "original"}
		updated, _ := model.Update(runeKey('e'))
		model = updated.(Model)
		model.subjects.input.SetValue(strings.Repeat("PRIVATE-VALIDATION", 30))
		model = runModelCommand(t, model, enterKey())
		if !strings.Contains(model.subjects.errMessage, "subject_name: must be between") || !model.subjects.input.Focused() {
			t.Fatalf("got feedback %q, want validation rule and focused input", model.subjects.errMessage)
		}
		model.subjects.input.SetValue("corrected")
		model = runModelCommand(t, model, enterKey())
		if model.subjects.err != nil || model.subjects.selected.SubjectName != "corrected" {
			t.Fatal("got failed retry, want corrected subject")
		}
	})
}
