//go:build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/tui/goals"
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

func TestGoalActivityLineageTUIWorkflow_SQLite(t *testing.T) {
	t.Run("opens matching Thoughts in shared Details changes Mindset and restores activity selection", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer runtime.Close()
		user := runtime.LocalUser().UserID
		subject, err := runtime.Subjects().Create(t.Context(), user, "Programming")
		if err != nil {
			t.Fatal(err)
		}
		_, err = runtime.GoalAssignments().Create(t.Context(), user, goal.CreateGoalInput{Name: "Build lineage", TargetSeconds: 3600}, []int64{subject.SubjectID})
		if err != nil {
			t.Fatal(err)
		}
		_, err = runtime.Events().CreatePast(t.Context(), user, "Implement history", time.Unix(100, 0), time.Unix(200, 0), []int64{subject.SubjectID})
		if err != nil {
			t.Fatal(err)
		}
		thought, err := runtime.Thoughts().Create(t.Context(), user, "Events own the authoritative interval.", &subject.SubjectID, time.Unix(150, 0))
		if err != nil {
			t.Fatal(err)
		}
		m := tui.NewModel(t.Context(), runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), runtime.Events(), runtime.TimelineView(), logging.Nop())
		m.ConfigureGoals(runtime.Goals(), runtime.GoalAssignments(), runtime.SubjectGoals(), runtime.GoalProgress())
		var deliver func(tea.Cmd)
		deliver = func(cmd tea.Cmd) {
			if cmd == nil {
				return
			}
			reply := cmd()
			if batch, ok := reply.(tea.BatchMsg); ok {
				for _, child := range batch {
					deliver(child)
				}
				return
			}
			accept := goals.Owns(reply)
			switch reply.(type) {
			case goals.OpenLineageThoughtMsg, thoughts.Result, thoughts.MindsetResult, thoughts.MindsetChanged, listfilter.Reply:
				accept = true
			}
			if accept {
				next, cmd := m.Update(reply)
				m = next.(tui.Model)
				deliver(cmd)
			}
		}
		send := func(msg tea.Msg) { next, cmd := m.Update(msg); m = next.(tui.Model); deliver(cmd) }
		key := func(code rune) { send(tea.KeyPressMsg{Code: code}) }
		send(tea.WindowSizeMsg{Width: 100, Height: 35})
		for _, code := range []rune{tea.KeyTab, tea.KeyLeft, tea.KeyLeft, tea.KeyEnter, tea.KeyEnter} {
			key(code)
		}
		if !strings.Contains(m.View().Content, "ACTIVITY HISTORY") || !strings.Contains(m.View().Content, "Implement history") {
			t.Fatalf("history: %s", m.View().Content)
		}
		key(tea.KeyEnter)
		key(tea.KeyDown)
		key(tea.KeyEnter)
		if !strings.Contains(m.View().Content, "m: mindset") || strings.Contains(m.View().Content, "ACTIVITY HISTORY") {
			t.Fatalf("shared Thought Details: %s", m.View().Content)
		}
		key('m')
		current, err := runtime.Thoughts().Get(t.Context(), user, thought.ThoughtID)
		if err != nil || !current.IsMindset {
			t.Fatalf("Mindset persisted: got %+v,%v, want active", current, err)
		}
		key(tea.KeyEscape)
		if !strings.Contains(m.View().Content, "ACTIVITY HISTORY") || !strings.Contains(m.View().Content, "M · Events own") {
			t.Fatalf("return to selected preview: %s", m.View().Content)
		}
		key(tea.KeyEnter)
		if !strings.Contains(m.View().Content, "m: mindset") {
			t.Fatal("return lost selected Thought")
		}
		key(tea.KeyEscape)
		key(tea.KeyEscape)
		if strings.Contains(m.View().Content, "Events own the authoritative") {
			t.Fatal("Escape did not collapse selected Event")
		}
		key(tea.KeyEscape)
		if !strings.Contains(m.View().Content, "Priority:") {
			t.Fatalf("return to Goals list: %s", m.View().Content)
		}
	})
}
