package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

type subjectServiceStub struct {
	update func(context.Context, string, int64, string) (*data.Subject, error)
	delete func(context.Context, string, int64) error
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

func (stub *subjectServiceStub) Update(ctx context.Context, user string, id int64, name string) (*data.Subject, error) {
	return stub.update(ctx, user, id, name)
}
func (stub *subjectServiceStub) Delete(ctx context.Context, user string, id int64) error {
	return stub.delete(ctx, user, id)
}

func TestSubjectModel_List(t *testing.T) {
	t.Run("shows Create action for empty list", func(t *testing.T) {
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return nil, nil
		}}
		model := openSubjects(t, newSubjectTestModel(service))

		rows := model.subjects.list.Items()

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

		rows := model.subjects.list.Items()

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
			if description := row.Description(); description != "Thought count unavailable" {
				t.Fatalf("got Subject row description %q, want unavailable count when metrics omit the subject", description)
			}
		}
	})

	t.Run("shows list error", func(t *testing.T) {
		want := errors.New("list failed")
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return nil, want
		}}
		model := openSubjects(t, newSubjectTestModel(service))

		if !errors.Is(model.subjects.err, want) {
			t.Fatalf("got error %v, want original error %v", model.subjects.err, want)
		}
		if view := model.View().Content; !strings.Contains(view, "Could not list subjects.") {
			t.Fatalf("got view %q, want safe message %q", view, "Could not list subjects.")
		}
	})
}

func TestSubjectModel_Create(t *testing.T) {
	t.Run("passes entered name unchanged and displays created subject", func(t *testing.T) {
		const name = "  learn Go  "
		createdAt := time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)
		service := &subjectServiceStub{
			list: func(context.Context, string) ([]data.Subject, error) { return nil, nil },
			create: func(_ context.Context, userID, gotName string) (*data.Subject, error) {
				if userID != "local-user-id" || gotName != name {
					t.Fatalf("got user ID %q and name %q", userID, gotName)
				}
				return &data.Subject{SubjectID: 7, UserID: userID, SubjectName: gotName, CreatedAt: createdAt}, nil
			},
		}
		model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(service)))
		model.subjects.input.SetValue(name)

		model = runModelCommand(t, model, enterKey())

		if model.screen != screenSubjectDetail || !model.subjects.listStale {
			t.Fatalf("got screen %v and stale=%v, want created detail with stale list", model.screen, model.subjects.listStale)
		}
		for _, want := range []string{name, "Added: Jan 2, 2026"} {
			if view := model.View().Content; !strings.Contains(view, want) {
				t.Fatalf("view %q does not contain %q", view, want)
			}
		}
		if view := model.View().Content; strings.Contains(view, "ID:") {
			t.Fatalf("view %q contains Subject ID", view)
		}
	})

	t.Run("preserves input and displays service error", func(t *testing.T) {
		want := errors.New("create failed")
		service := &subjectServiceStub{
			list:   func(context.Context, string) ([]data.Subject, error) { return nil, nil },
			create: func(context.Context, string, string) (*data.Subject, error) { return nil, want },
		}
		model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(service)))
		model.subjects.input.SetValue("invalid")

		model = runModelCommand(t, model, enterKey())

		if model.screen != screenSubjectCreate || model.subjects.input.Value() != "invalid" {
			t.Fatalf("got screen %v and input %q", model.screen, model.subjects.input.Value())
		}
		if !errors.Is(model.subjects.err, want) {
			t.Fatalf("got error %v, want original error %v", model.subjects.err, want)
		}
		if view := model.View().Content; !strings.Contains(view, "Could not save the subject.") {
			t.Fatalf("got view %q, want safe message %q", view, "Could not save the subject.")
		}
	})

	t.Run("treats q as form input", func(t *testing.T) {
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }}
		model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(service)))

		updated, _ := model.Update(runeKey('q'))
		model = updated.(Model)

		if model.screen != screenSubjectCreate || model.subjects.input.Value() != "q" {
			t.Fatalf("got screen %v and input %q, want create form containing q", model.screen, model.subjects.input.Value())
		}
	})

	t.Run("cancels form without calling service", func(t *testing.T) {
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }}
		model := openCreateSubject(t, openSubjects(t, newSubjectTestModel(service)))
		model.subjects.input.SetValue("discard me")

		updated, command := model.Update(escapeKey())
		model = updated.(Model)

		if command != nil {
			t.Fatal("got command, want cached subject list")
		}
		if service.createCalls != 0 || model.screen != screenSubjectList || model.subjects.input.Value() != "" {
			t.Fatalf(
				"got %d create calls, screen %v, and input %q",
				service.createCalls,
				model.screen,
				model.subjects.input.Value(),
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
		model.subjects.input.SetValue("coding")
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

		if service.listCalls != 2 || model.subjects.listStale {
			t.Fatalf("got %d list calls and stale=%v, want 2 calls and fresh list", service.listCalls, model.subjects.listStale)
		}
	})
}

func TestSubjectModel_Get(t *testing.T) {
	t.Run("passes bootstrapped local user ID + selected Subject ID for a typed Subject row", func(t *testing.T) {
		createdAt := time.Date(2025, time.March, 4, 15, 4, 5, 0, time.UTC)
		service := &subjectServiceStub{
			list: func(_ context.Context, userID string) ([]data.Subject, error) {
				return []data.Subject{{SubjectID: 9, UserID: userID, SubjectName: createSubjectLabel}}, nil
			},
			get: func(_ context.Context, userID string, subjectID int64) (*data.Subject, error) {
				if userID != "local-user-id" || subjectID != 9 {
					t.Fatalf("got bootstrapped local user ID %q and selected Subject ID %d", userID, subjectID)
				}
				return &data.Subject{
					SubjectID:   subjectID,
					UserID:      userID,
					SubjectName: createSubjectLabel,
					CreatedAt:   createdAt,
				}, nil
			},
		}
		model := openSubjects(t, newSubjectTestModel(service))
		model.subjects.list.Select(1)

		model = runModelCommand(t, model, enterKey())

		if service.createCalls != 0 || service.getCalls != 1 || model.screen != screenSubjectDetail {
			t.Fatalf("got %d create calls, %d get calls, and screen %v", service.createCalls, service.getCalls, model.screen)
		}
		view := model.View().Content
		if !strings.Contains(view, "Added: Mar 4, 2025") {
			t.Fatalf("view %q does not contain added date", view)
		}
		if strings.Contains(view, "ID:") {
			t.Fatalf("view %q contains Subject ID", view)
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
		model.subjects.list.Select(1)

		model = runModelCommand(t, model, enterKey())

		if model.screen != screenSubjectList {
			t.Fatalf("got screen %v, want subject list", model.screen)
		}
		if !errors.Is(model.subjects.err, want) {
			t.Fatalf("got error %v, want original error %v", model.subjects.err, want)
		}
		if view := model.View().Content; !strings.Contains(view, "Could not retrieve the subject.") {
			t.Fatalf("got view %q, want safe message %q", view, "Could not retrieve the subject.")
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
		model.subjects.list.Select(1)
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

func newSubjectTestModel(service SubjectService) Model {
	return NewModel(context.Background(), &data.User{UserID: "local-user-id"}, service, thought.NewService(testutils.NewFakeThoughtStore()), &metricsStub{}, logging.Nop())
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
