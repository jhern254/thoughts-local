package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

type subjectAliasServiceStub struct {
	listAliases func(context.Context, string, int64) ([]data.SubjectAlias, error)
	createAlias func(context.Context, string, int64, string) (*data.SubjectAlias, error)
	updateAlias func(context.Context, string, int64, int64, string) (*data.SubjectAlias, error)
	removeAlias func(context.Context, string, int64, int64) error
}

func (s *subjectAliasServiceStub) ListAliases(ctx context.Context, user string, id int64) ([]data.SubjectAlias, error) {
	return s.listAliases(ctx, user, id)
}
func (s *subjectAliasServiceStub) CreateAlias(ctx context.Context, user string, id int64, name string) (*data.SubjectAlias, error) {
	return s.createAlias(ctx, user, id, name)
}
func (s *subjectAliasServiceStub) UpdateAlias(ctx context.Context, user string, subjectID, aliasID int64, name string) (*data.SubjectAlias, error) {
	return s.updateAlias(ctx, user, subjectID, aliasID, name)
}
func (s *subjectAliasServiceStub) RemoveAlias(ctx context.Context, user string, subjectID, aliasID int64) error {
	return s.removeAlias(ctx, user, subjectID, aliasID)
}

func aliasTestModel(t *testing.T) Model {
	t.Helper()
	service := subject.NewService(testutils.NewFakeSubjectStore())
	created, err := service.Create(t.Context(), "local-user-id", "Go Programming")
	if err != nil {
		t.Fatal(err)
	}
	model := newSubjectTestModel(service)
	model.screen = screenSubjectDetail
	model.subjects.selected = created
	return model
}

func TestSubjectModel_Aliases(t *testing.T) {
	t.Run("highlights the Subject below the heading and returns without resetting thoughts", func(t *testing.T) {
		model := aliasTestModel(t)
		thoughtService := thought.NewService(testutils.NewFakeThoughtStore())
		subjectID := model.subjects.selected.SubjectID
		for _, text := range []string{"First thought", "Second thought", "Third thought"} {
			if _, err := thoughtService.Create(t.Context(), "local-user-id", text, &subjectID, time.Time{}); err != nil {
				t.Fatal(err)
			}
		}
		model.thoughts = thoughts.New(t.Context(), "local-user-id", thoughtService, logging.Nop())
		cmd := model.thoughts.Open(subjectID)
		model.thoughts, _ = model.thoughts.Update(cmd())
		for range 2 {
			updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
			model = updated.(Model)
		}
		before := model.thoughts.View()
		model = runAliasDataCommand(t, model, runeKey('a'))
		view := ansi.Strip(model.View().Content)
		for _, label := range []string{"Names & Aliases", "Go Programming", "Aliases for subject:", "+ Add alias"} {
			if !strings.Contains(view, label) {
				t.Fatalf("alias view: got %q, want %q", view, label)
			}
		}
		lines := strings.Split(view, "\n")
		if len(lines) < 6 || strings.TrimSpace(lines[0]) != "Names & Aliases" || strings.TrimSpace(lines[2]) != "Go Programming" || strings.TrimSpace(lines[4]) != "Aliases for subject:" {
			t.Fatalf("alias layout: got %q, want heading, Subject, then alias section on separate rows", view)
		}
		if strings.Contains(view, "Primary") {
			t.Fatalf("alias layout: got %q, want no Primary label", view)
		}
		styledSubject := model.subjects.list.Styles.Title.Render("Go Programming")
		if styledSubject == ansi.Strip(styledSubject) || !strings.Contains(model.View().Content, styledSubject) {
			t.Fatalf("Subject style: got %q, want the existing title color block %q", model.View().Content, styledSubject)
		}
		if model.screen != screenSubjectAliases {
			t.Fatalf("screen: got %v, want aliases", model.screen)
		}
		updated, cmd := model.Update(escapeKey())
		model = updated.(Model)
		if cmd != nil || model.screen != screenSubjectDetail || model.thoughts.View() != before {
			t.Fatal("return reset the underlying Thought browser")
		}
	})
	t.Run("pages overflow aliases and keeps the selected row through editing", func(t *testing.T) {
		model := aliasTestModel(t)
		for index := 1; index <= 40; index++ {
			if _, err := model.subjects.service.CreateAlias(t.Context(), "local-user-id", 1, fmt.Sprintf("Alias %02d", index)); err != nil {
				t.Fatal(err)
			}
		}
		model = runAliasDataCommand(t, model, runeKey('a'))
		updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
		model = updated.(Model)
		if got := model.subjects.aliases.list.Index(); got != 40 {
			t.Fatalf("last row: got %d, want 40", got)
		}
		if !strings.Contains(model.View().Content, "Alias 40") {
			t.Fatal("last alias is not visible after paging")
		}
		updated, _ = model.Update(enterKey())
		model = updated.(Model)
		updated, _ = model.Update(escapeKey())
		model = updated.(Model)
		if got := model.subjects.aliases.list.Index(); got != 40 {
			t.Fatalf("returned selection: got %d, want 40", got)
		}
	})
	t.Run("creates edits and removes with alias-only confirmation", func(t *testing.T) {
		model := runAliasDataCommand(t, aliasTestModel(t), runeKey('a'))
		updated, _ := model.Update(enterKey())
		model = updated.(Model)
		model.subjects.aliases.input.SetValue("Golang")
		model = runAliasDataCommand(t, model, enterKey())
		if !strings.Contains(model.View().Content, "Golang") || model.subjects.aliases.list.Index() != 1 {
			t.Fatal("created alias was not selected")
		}
		updated, _ = model.Update(enterKey())
		model = updated.(Model)
		if model.subjects.aliases.input.Value() != "Golang" {
			t.Fatal("edit was not prefilled")
		}
		model.subjects.aliases.input.SetValue("Go development")
		model = runAliasDataCommand(t, model, enterKey())
		updated, cmd := model.Update(runeKey('d'))
		model = updated.(Model)
		if cmd != nil || !strings.Contains(model.View().Content, "Only this alias") {
			t.Fatalf("removal view: got %q, want alias-only warning", model.View().Content)
		}
		updated, cmd = model.Update(enterKey())
		model = updated.(Model)
		if cmd != nil {
			t.Fatal("Enter removed an alias without y confirmation")
		}
		updated, _ = model.Update(runeKey('n'))
		model = updated.(Model)
		if !strings.Contains(model.View().Content, "Go development") {
			t.Fatal("cancel removed alias")
		}
		updated, _ = model.Update(runeKey('d'))
		model = updated.(Model)
		model = runAliasDataCommand(t, model, runeKey('y'))
		aliases, err := model.subjects.service.ListAliases(t.Context(), "local-user-id", 1)
		if err != nil || len(aliases) != 0 || model.subjects.selected.SubjectName != "Go Programming" {
			t.Fatalf("after removal: got %+v,%v; want empty aliases and retained Subject", aliases, err)
		}
	})
	t.Run("cancels forms and treats shortcut letters as input", func(t *testing.T) {
		model := runAliasDataCommand(t, aliasTestModel(t), runeKey('a'))
		updated, _ := model.Update(enterKey())
		model = updated.(Model)
		for _, r := range "qda" {
			updated, _ = model.Update(runeKey(r))
			model = updated.(Model)
		}
		if model.subjects.aliases.input.Value() != "qda" {
			t.Fatalf("input: got %q, want qda", model.subjects.aliases.input.Value())
		}
		updated, cmd := model.Update(escapeKey())
		model = updated.(Model)
		if cmd != nil || model.subjects.aliases.mode != aliasBrowse {
			t.Fatal("form cancellation did not return to alias list")
		}
	})
	t.Run("retains failed input and safe feedback for conflict and validation", func(t *testing.T) {
		model := runAliasDataCommand(t, aliasTestModel(t), runeKey('a'))
		updated, _ := model.Update(enterKey())
		model = updated.(Model)
		model.subjects.aliases.input.SetValue("Go Programming")
		model = runAliasDataCommand(t, model, enterKey())
		if !errors.Is(model.subjects.aliases.err, data.ErrDuplicateRecord) || !model.subjects.aliases.input.Focused() || !strings.Contains(model.View().Content, "already in use") {
			t.Fatalf("conflict: got %q, want focused input and fixed guidance", model.View().Content)
		}
		model.subjects.aliases.input.SetValue("   ")
		model = runAliasDataCommand(t, model, enterKey())
		if !strings.Contains(model.View().Content, "alias_name: must be between") {
			t.Fatalf("validation: got %q, want fixed guidance", model.View().Content)
		}
	})
	t.Run("blocks repeated writes and navigation while a mutation is pending", func(t *testing.T) {
		model := runAliasDataCommand(t, aliasTestModel(t), runeKey('a'))
		updated, _ := model.Update(enterKey())
		model = updated.(Model)
		model.subjects.aliases.input.SetValue("Golang")
		updated, write := model.Update(enterKey())
		model = updated.(Model)
		if write == nil {
			t.Fatal("missing write command")
		}
		for _, key := range []tea.KeyPressMsg{enterKey(), escapeKey(), runeKey('q'), runeKey('d')} {
			updated, cmd := model.Update(key)
			model = updated.(Model)
			if cmd != nil || !model.subjects.aliases.loading || model.subjects.aliases.mode != aliasEdit {
				t.Fatal("pending mutation accepted navigation or duplicate submission")
			}
		}
	})
	t.Run("cancels retired reads and ignores late replies from an earlier opening", func(t *testing.T) {
		var firstContext context.Context
		service := &subjectServiceStub{}
		service.listAliases = func(ctx context.Context, u string, id int64) ([]data.SubjectAlias, error) {
			firstContext = ctx
			return []data.SubjectAlias{{AliasID: 1, AliasName: "Old"}}, nil
		}
		model := newSubjectTestModel(service)
		model.screen = screenSubjectDetail
		model.subjects.selected = &data.Subject{SubjectID: 1, SubjectName: "Primary"}
		updated, cmd := model.Update(runeKey('a'))
		model = updated.(Model)
		late := cmd()
		updated, _ = model.Update(escapeKey())
		model = updated.(Model)
		if firstContext.Err() != context.Canceled {
			t.Fatal("read retained ownership after leaving aliases")
		}
		updated, _ = model.Update(runeKey('a'))
		model = updated.(Model)
		updated, _ = model.Update(late)
		model = updated.(Model)
		if !model.subjects.aliases.loading || strings.Contains(model.View().Content, "Old") {
			t.Fatal("old read published into a newer opening")
		}
	})
	t.Run("does not execute a read canceled before its command starts", func(t *testing.T) {
		model := newSubjectTestModel(&subjectServiceStub{})
		model.screen = screenSubjectDetail
		model.subjects.selected = &data.Subject{SubjectID: 1, SubjectName: "Primary"}
		updated, read := model.Update(runeKey('a'))
		model = updated.(Model)
		updated, _ = model.Update(escapeKey())
		model = updated.(Model)
		if got := read(); got != nil {
			t.Fatalf("retired command: got %#v, want nil without service call", got)
		}
	})
	t.Run("ignores a real write result from a retired opening", func(t *testing.T) {
		model := runAliasDataCommand(t, aliasTestModel(t), runeKey('a'))
		updated, _ := model.Update(enterKey())
		model = updated.(Model)
		model.subjects.aliases.input.SetValue("Golang")
		updated, write := model.Update(enterKey())
		model = updated.(Model)
		late := write()
		// Simulate an externally replaced screen/session, rather than allowing
		// ordinary navigation to escape the pending-write guard.
		model.screen = screenSubjectDetail
		model = runAliasDataCommand(t, model, runeKey('a'))
		before := model.View().Content
		updated, cmd := model.Update(late)
		model = updated.(Model)
		if cmd != nil || model.View().Content != before || model.subjects.aliases.loading {
			t.Fatal("retired write changed the new alias screen")
		}
	})
	t.Run("reports load and write failures without private diagnostics and permits retry", func(t *testing.T) {
		const private = "PRIVATE-ALIAS-FAILURE"
		failure := errors.New(private)
		service := &subjectServiceStub{}
		service.listAliases = func(context.Context, string, int64) ([]data.SubjectAlias, error) { return nil, failure }
		service.createAlias = func(context.Context, string, int64, string) (*data.SubjectAlias, error) { return nil, failure }
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		model := newSubjectTestModel(service)
		model.logger = logger
		model.screen = screenSubjectDetail
		model.subjects.selected = &data.Subject{SubjectID: 1, SubjectName: "Primary"}
		model = runAliasDataCommand(t, model, runeKey('a'))
		if !errors.Is(model.subjects.aliases.err, failure) || strings.Contains(model.subjects.aliases.errMessage+logs.String(), private) {
			t.Fatal("load failure lost its cause or leaked private diagnostics")
		}
		service.listAliases = func(context.Context, string, int64) ([]data.SubjectAlias, error) { return nil, nil }
		model = runAliasDataCommand(t, model, runeKey('r'))
		updated, _ := model.Update(enterKey())
		model = updated.(Model)
		model.subjects.aliases.input.SetValue("Unchanged input")
		model = runAliasDataCommand(t, model, enterKey())
		if model.subjects.aliases.input.Value() != "Unchanged input" || !model.subjects.aliases.input.Focused() || !errors.Is(model.subjects.aliases.err, failure) || strings.Contains(model.subjects.aliases.errMessage+logs.String(), private) {
			t.Fatal("write failure lost input/cause/focus or leaked diagnostics")
		}
		if got := strings.Count(logs.String(), "operation failed"); got != 2 {
			t.Fatalf("failure log count: got %d, want 2", got)
		}
	})
	t.Run("fits long Unicode names at normal and narrow widths", func(t *testing.T) {
		for _, width := range []int{80, 32} {
			model := aliasTestModel(t)
			model.subjects.selected.SubjectName = strings.Repeat("界", 100)
			if _, err := model.subjects.service.CreateAlias(t.Context(), "local-user-id", 1, strings.Repeat("語", 100)); err != nil {
				t.Fatal(err)
			}
			updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 18})
			model = updated.(Model)
			model = runAliasDataCommand(t, model, runeKey('a'))
			for _, line := range strings.Split(model.View().Content, "\n") {
				if got := ansi.StringWidth(line); got > width {
					t.Fatalf("row width: got %d, want <=%d: %q", got, width, line)
				}
			}
			if got := strings.Count(model.View().Content, "\n") + 1; got > 18 {
				t.Fatalf("row count: got %d, want <=18", got)
			}
			model.subjects.aliases.list.Select(1)
			updated, _ = model.Update(runeKey('d'))
			model = updated.(Model)
			if got := strings.Count(model.View().Content, "\n") + 1; got > 18 {
				t.Fatalf("confirmation row count: got %d, want <=18", got)
			}
		}
	})
	t.Run("keeps Subject Details alias and return shortcuts visible at narrow widths", func(t *testing.T) {
		model := aliasTestModel(t)
		updated, _ := model.Update(tea.WindowSizeMsg{Width: 32, Height: 18})
		model = updated.(Model)
		help := model.subjectDetailHelp()
		if !strings.Contains(help, "a: aliases") || !strings.Contains(help, "Q/Esc: subjects") {
			t.Fatalf("help: got %q, want alias and return actions", help)
		}
		for _, line := range strings.Split(help, "\n") {
			if got := ansi.StringWidth(line); got > 32 {
				t.Fatalf("help row: got %d columns, want <=32: %q", got, line)
			}
		}
	})
}

var _ SubjectService = (*subject.Service)(nil)

func runAliasDataCommand(t *testing.T, model Model, message tea.Msg) Model {
	t.Helper()
	updated, cmd := model.Update(message)
	if cmd == nil {
		t.Fatal("missing alias data command")
	}
	return applyAliasDataCommand(t, updated.(Model), cmd)
}

func applyAliasDataCommand(t *testing.T, model Model, command tea.Cmd) Model {
	t.Helper()
	message := command()
	if batch, ok := message.(tea.BatchMsg); ok {
		for _, cmd := range batch {
			model = applyAliasDataCommand(t, model, cmd)
		}
		return model
	}
	updated, next := model.Update(message)
	model = updated.(Model)
	if saved, ok := message.(subjectAliasSavedMsg); ok && saved.err == nil && next != nil {
		return applyAliasDataCommand(t, model, next)
	}
	return model
}
