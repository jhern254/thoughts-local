package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
)

func subjectAliasConflictModel(t *testing.T) Model {
	t.Helper()
	service := subject.NewService(testutils.NewFakeSubjectStore())
	original, err := service.Create(t.Context(), "local-user-id", "Go Programming")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateAlias(t.Context(), "local-user-id", original.SubjectID, "Golang"); err != nil {
		t.Fatal(err)
	}
	model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(service)))
	model.subjects.input.SetValue("Golang")
	return runModelCommand(t, model, enterKey())
}

func aliasCreationKey() tea.KeyPressMsg { return runeKey('d') }

type subjectFromAliasServiceStub struct {
	SubjectService
	createFromAlias func(context.Context, string, int64, int64, string) (*data.Subject, error)
}

func (s *subjectFromAliasServiceStub) CreateFromAlias(ctx context.Context, userID string, subjectID, aliasID int64, name string) (*data.Subject, error) {
	return s.createFromAlias(ctx, userID, subjectID, aliasID, name)
}

func TestSubjectModel_CreateFromAlias(t *testing.T) {
	t.Run("offers confirmation then creates a separate empty Subject", func(t *testing.T) {
		model := subjectAliasConflictModel(t)
		if !strings.Contains(model.View().Content, "d: remove alias and create") {
			t.Fatalf("conflict view: got %q, want explicit action", model.View().Content)
		}
		updated, _ := model.Update(aliasCreationKey())
		model = updated.(Model)
		if model.screen != screenSubjectFromAlias {
			t.Fatalf("screen: got %v, want alias replacement confirmation", model.screen)
		}
		for _, width := range []int{80, 32} {
			updated, _ = model.Update(tea.WindowSizeMsg{Width: width, Height: 18})
			model = updated.(Model)
			view := model.View().Content
			for _, label := range []string{"Golang", "Go Programming", "new empty Subject", "original Subject", "y: remove alias and create", "n/Q/Esc: back"} {
				if !strings.Contains(strings.Join(strings.Fields(ansi.Strip(view)), " "), label) {
					t.Fatalf("confirmation: got %q, want %q", view, label)
				}
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > width {
					t.Fatalf("confirmation width: got %d, want <=%d", ansi.StringWidth(line), width)
				}
			}
			if strings.Count(view, "\n")+1 > 18 {
				t.Fatalf("confirmation too tall: got %q", view)
			}
		}
		aliases, err := model.subjects.service.ListAliases(t.Context(), "local-user-id", 1)
		if err != nil || len(aliases) != 1 {
			t.Fatalf("before confirmation: got %+v,%v, want existing alias", aliases, err)
		}
		model = runModelCommand(t, model, runeKey('y'))
		if model.screen != screenSubjectDetail || model.subjects.selected.SubjectID == 1 || model.subjects.selected.SubjectName != "Golang" {
			t.Fatalf("new Subject: got %+v on %v, want separate Golang detail", model.subjects.selected, model.screen)
		}
		aliases, err = model.subjects.service.ListAliases(t.Context(), "local-user-id", 1)
		if err != nil || len(aliases) != 0 {
			t.Fatalf("remaining aliases: got %+v,%v, want none", aliases, err)
		}
		if !model.subjects.listStale {
			t.Fatal("Subject list not marked for refresh")
		}
	})
	t.Run("cancel retains the alias and entered name", func(t *testing.T) {
		for _, key := range []tea.KeyPressMsg{escapeKey(), runeKey('n'), runeKey('q')} {
			model := subjectAliasConflictModel(t)
			updated, _ := model.Update(aliasCreationKey())
			model = updated.(Model)
			updated, cmd := model.Update(key)
			model = updated.(Model)
			if cmd == nil || model.screen != screenSubjectCreate || !model.subjects.input.Focused() || model.subjects.input.Value() != "Golang" {
				t.Fatal("cancel lost creation form, input, or focus")
			}
			aliases, err := model.subjects.service.ListAliases(t.Context(), "local-user-id", 1)
			if err != nil || len(aliases) != 1 {
				t.Fatalf("canceled replacement: got %+v,%v, want unchanged alias", aliases, err)
			}
		}
	})
	t.Run("editing the pending name invalidates the alias action", func(t *testing.T) {
		model := subjectAliasConflictModel(t)
		model.subjects.input.SetValue("Different name")
		model.subjects.input.CursorEnd()
		updated, _ := model.Update(aliasCreationKey())
		model = updated.(Model)
		if model.screen != screenSubjectCreate || strings.Contains(model.View().Content, "d: remove alias") {
			t.Fatal("stale conflict still offered removal after name changed")
		}
		if got := model.subjects.input.Value(); got != "Different named" {
			t.Fatalf("ordinary typing: got %q, want Different named", got)
		}
	})
	t.Run("d remains text without an alias conflict", func(t *testing.T) {
		model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(subject.NewService(testutils.NewFakeSubjectStore()))))
		updated, _ := model.Update(runeKey('d'))
		model = updated.(Model)
		if model.screen != screenSubjectCreate || model.subjects.input.Value() != "d" {
			t.Fatal("d did not remain ordinary form text")
		}
	})
	t.Run("failed confirmation permits retry and duplicate submits cannot run", func(t *testing.T) {
		model := subjectAliasConflictModel(t)
		calls := 0
		base := model.subjects.service
		model.subjects.service = &subjectFromAliasServiceStub{SubjectService: base, createFromAlias: func(ctx context.Context, user string, subjectID, aliasID int64, name string) (*data.Subject, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("PRIVATE-FAILURE")
			}
			return base.CreateFromAlias(ctx, user, subjectID, aliasID, name)
		}}
		updated, _ := model.Update(aliasCreationKey())
		model = updated.(Model)
		updated, cmd := model.Update(runeKey('y'))
		model = updated.(Model)
		updated, duplicate := model.Update(runeKey('y'))
		model = updated.(Model)
		if duplicate != nil {
			t.Fatal("duplicate confirmation started another write")
		}
		model = applyCommand(t, model, cmd)
		if model.screen != screenSubjectFromAlias || model.subjects.loading || !strings.Contains(model.View().Content, "Could not create the subject.") || strings.Contains(model.View().Content, "PRIVATE") {
			t.Fatalf("failure feedback: got %q", model.View().Content)
		}
		model = runModelCommand(t, model, runeKey('y'))
		if calls != 2 || model.screen != screenSubjectDetail {
			t.Fatalf("retry: got %d calls on %v, want 2 and Subject detail", calls, model.screen)
		}
	})
	t.Run("a late result cannot replace a newer creation form", func(t *testing.T) {
		model := subjectAliasConflictModel(t)
		updated, _ := model.Update(aliasCreationKey())
		model = updated.(Model)
		updated, cmd := model.Update(runeKey('y'))
		model = updated.(Model)
		message := cmd()
		updated, _ = model.openSubjectCreate("New draft")
		model = updated.(Model)
		updated, next := model.Update(message)
		model = updated.(Model)
		if next != nil || model.screen != screenSubjectCreate || model.subjects.input.Value() != "New draft" {
			t.Fatal("late result replaced newer form")
		}
	})
}
