package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

type subjectServiceStub struct {
	list   func(context.Context, string) ([]data.Subject, error)
	create func(context.Context, string, string) (*data.Subject, error)
	get    func(context.Context, string, int64) (*data.Subject, error)

	listCalls   int
	createCalls int
	getCalls    int
}

func (stub *subjectServiceStub) List(ctx context.Context, userID string) ([]data.Subject, error) {
	stub.listCalls++
	return stub.list(ctx, userID)
}

func (stub *subjectServiceStub) Create(ctx context.Context, userID, name string) (*data.Subject, error) {
	stub.createCalls++
	return stub.create(ctx, userID, name)
}

func (stub *subjectServiceStub) Get(ctx context.Context, userID string, subjectID int64) (*data.Subject, error) {
	stub.getCalls++
	return stub.get(ctx, userID, subjectID)
}

func TestSubjectModel_List(t *testing.T) {
	t.Run("shows Create action for empty list", func(t *testing.T) {
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return nil, nil
		}}
		model := openSubjects(t, newSubjectTestModel(service))

		rows := model.subjectList.Items()

		if len(rows) != 1 || rows[0].(subjectRow).kind != subjectRowCreate {
			t.Fatalf("got rows %#v, want only Create action", rows)
		}
	})

	t.Run("lists typed Create and Subject rows", func(t *testing.T) {
		service := &subjectServiceStub{list: func(_ context.Context, userID string) ([]data.Subject, error) {
			if userID != "local-user-id" {
				t.Fatalf("got user ID %q", userID)
			}
			return []data.Subject{
				{SubjectID: 1, UserID: userID, SubjectName: "coding"},
				{SubjectID: 2, UserID: userID, SubjectName: createSubjectLabel},
			}, nil
		}}
		model := openSubjects(t, newSubjectTestModel(service))

		rows := model.subjectList.Items()

		if service.listCalls != 1 {
			t.Fatalf("got %d List calls, want 1", service.listCalls)
		}
		if len(rows) != 3 {
			t.Fatalf("got %d rows, want 3", len(rows))
		}
		createRow := rows[0].(subjectRow)
		if createRow.kind != subjectRowCreate {
			t.Fatalf("got first row kind %v, want Create", createRow.kind)
		}
		for index, item := range rows[1:] {
			row := item.(subjectRow)
			if row.kind != subjectRowRecord {
				t.Fatalf("got row %d kind %v, want Subject", index+1, row.kind)
			}
		}
	})

	t.Run("shows list error", func(t *testing.T) {
		want := errors.New("list failed")
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return nil, want
		}}
		model := openSubjects(t, newSubjectTestModel(service))

		if view := model.View().Content; !strings.Contains(view, want.Error()) {
			t.Fatalf("view %q does not contain %q", view, want)
		}
	})
}

func TestSubjectModel_Create(t *testing.T) {
	t.Run("passes entered name unchanged and displays created subject", func(t *testing.T) {
		const name = "  learn Go  "
		service := &subjectServiceStub{
			list: func(context.Context, string) ([]data.Subject, error) { return nil, nil },
			create: func(_ context.Context, userID, gotName string) (*data.Subject, error) {
				if userID != "local-user-id" || gotName != name {
					t.Fatalf("got user ID %q and name %q", userID, gotName)
				}
				return &data.Subject{SubjectID: 7, UserID: userID, SubjectName: gotName}, nil
			},
		}
		model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(service)))
		model.subjectInput.SetValue(name)

		model = runModelCommand(t, model, enterKey())

		if model.screen != screenSubjectDetail || !model.subjectListStale {
			t.Fatalf("got screen %v and stale=%v, want created detail with stale list", model.screen, model.subjectListStale)
		}
		for _, want := range []string{"7", name} {
			if view := model.View().Content; !strings.Contains(view, want) {
				t.Fatalf("view %q does not contain %q", view, want)
			}
		}
	})

	t.Run("preserves input and displays service error", func(t *testing.T) {
		want := errors.New("create failed")
		service := &subjectServiceStub{
			list:   func(context.Context, string) ([]data.Subject, error) { return nil, nil },
			create: func(context.Context, string, string) (*data.Subject, error) { return nil, want },
		}
		model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(service)))
		model.subjectInput.SetValue("invalid")

		model = runModelCommand(t, model, enterKey())

		if model.screen != screenSubjectCreate || model.subjectInput.Value() != "invalid" {
			t.Fatalf("got screen %v and input %q", model.screen, model.subjectInput.Value())
		}
		if view := model.View().Content; !strings.Contains(view, want.Error()) {
			t.Fatalf("view %q does not contain %q", view, want)
		}
	})

	t.Run("treats q as form input", func(t *testing.T) {
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }}
		model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(service)))

		updated, _ := model.Update(runeKey('q'))
		model = updated.(Model)

		if model.screen != screenSubjectCreate || model.subjectInput.Value() != "q" {
			t.Fatalf("got screen %v and input %q, want create form containing q", model.screen, model.subjectInput.Value())
		}
	})

	t.Run("cancels form without calling service", func(t *testing.T) {
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }}
		model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(service)))
		model.subjectInput.SetValue("discard me")

		updated, command := model.Update(escapeKey())
		model = updated.(Model)

		if command != nil {
			t.Fatal("got command, want cached subject list")
		}
		if service.createCalls != 0 || model.screen != screenSubjectList || model.subjectInput.Value() != "" {
			t.Fatalf(
				"got %d create calls, screen %v, and input %q",
				service.createCalls,
				model.screen,
				model.subjectInput.Value(),
			)
		}
	})

	t.Run("refreshes list when leaving created detail", func(t *testing.T) {
		service := &subjectServiceStub{
			list: func(context.Context, string) ([]data.Subject, error) { return nil, nil },
			create: func(_ context.Context, userID, name string) (*data.Subject, error) {
				return &data.Subject{SubjectID: 7, UserID: userID, SubjectName: name}, nil
			},
		}
		model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(service)))
		model.subjectInput.SetValue("coding")
		model = runModelCommand(t, model, enterKey())
		if service.listCalls != 1 {
			t.Fatalf("got %d list calls before returning, want 1", service.listCalls)
		}

		updated, command := model.Update(escapeKey())
		model = updated.(Model)
		if command == nil {
			t.Fatal("got nil command, want list refresh")
		}
		model = applyCommand(t, model, command)

		if service.listCalls != 2 || model.subjectListStale {
			t.Fatalf("got %d list calls and stale=%v, want 2 calls and fresh list", service.listCalls, model.subjectListStale)
		}
	})
}

func TestSubjectModel_Get(t *testing.T) {
	t.Run("passes bootstrapped local user ID + selected Subject ID for a typed Subject row", func(t *testing.T) {
		service := &subjectServiceStub{
			list: func(_ context.Context, userID string) ([]data.Subject, error) {
				return []data.Subject{{SubjectID: 9, UserID: userID, SubjectName: createSubjectLabel}}, nil
			},
			get: func(_ context.Context, userID string, subjectID int64) (*data.Subject, error) {
				if userID != "local-user-id" || subjectID != 9 {
					t.Fatalf("got bootstrapped local user ID %q and selected Subject ID %d", userID, subjectID)
				}
				return &data.Subject{SubjectID: subjectID, UserID: userID, SubjectName: createSubjectLabel}, nil
			},
		}
		model := openSubjects(t, newSubjectTestModel(service))
		model.subjectList.Select(1)

		model = runModelCommand(t, model, enterKey())

		if service.createCalls != 0 || service.getCalls != 1 || model.screen != screenSubjectDetail {
			t.Fatalf("got %d create calls, %d get calls, and screen %v", service.createCalls, service.getCalls, model.screen)
		}
	})

	t.Run("displays get error", func(t *testing.T) {
		want := errors.New("record not found")
		service := &subjectServiceStub{
			list: func(_ context.Context, userID string) ([]data.Subject, error) {
				return []data.Subject{{SubjectID: 9, UserID: userID, SubjectName: "coding"}}, nil
			},
			get: func(context.Context, string, int64) (*data.Subject, error) { return nil, want },
		}
		model := openSubjects(t, newSubjectTestModel(service))
		model.subjectList.Select(1)

		model = runModelCommand(t, model, enterKey())

		if model.screen != screenSubjectList {
			t.Fatalf("got screen %v, want subject list", model.screen)
		}
		if view := model.View().Content; !strings.Contains(view, want.Error()) {
			t.Fatalf("view %q does not contain %q", view, want)
		}
	})

	t.Run("does not refresh list after viewing detail", func(t *testing.T) {
		service := &subjectServiceStub{
			list: func(_ context.Context, userID string) ([]data.Subject, error) {
				return []data.Subject{{SubjectID: 9, UserID: userID, SubjectName: "coding"}}, nil
			},
			get: func(_ context.Context, userID string, subjectID int64) (*data.Subject, error) {
				return &data.Subject{SubjectID: subjectID, UserID: userID, SubjectName: "coding"}, nil
			},
		}
		model := openSubjects(t, newSubjectTestModel(service))
		model.subjectList.Select(1)
		model = runModelCommand(t, model, enterKey())

		updated, command := model.Update(escapeKey())
		model = updated.(Model)

		if command != nil {
			t.Fatal("got command after read-only detail, want cached list")
		}
		if service.listCalls != 1 || model.screen != screenSubjectList {
			t.Fatalf("got %d list calls and screen %v", service.listCalls, model.screen)
		}
	})
}

func newSubjectTestModel(service SubjectService) Model {
	return NewModel(context.Background(), &data.User{UserID: "local-user-id"}, service)
}

func openSubjects(t *testing.T, model Model) Model {
	t.Helper()
	return runModelCommand(t, model, enterKey())
}

func openCreateSubject(t *testing.T, model Model) Model {
	t.Helper()
	updated, _ := model.Update(enterKey())
	return updated.(Model)
}

func runModelCommand(t *testing.T, model Model, message tea.Msg) Model {
	t.Helper()
	updated, command := model.Update(message)
	if command == nil {
		t.Fatal("got nil command")
	}
	return applyCommand(t, updated.(Model), command)
}

func applyCommand(t *testing.T, model Model, command tea.Cmd) Model {
	t.Helper()
	updated, _ := model.Update(command())
	return updated.(Model)
}
