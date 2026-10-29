//go:build integration

package integration_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/tui/goals"
	"github.com/jhern254/go-thoughts/internal/tui/listfilter"
)

func TestGoalsTUIWorkflow_SQLite(t *testing.T) {
	t.Run("root navigation creates edits and replaces multiple Subject links", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer runtime.Close()
		user := runtime.LocalUser().UserID
		for _, name := range []string{"Coding", "Reading"} {
			if _, err := runtime.Subjects().Create(t.Context(), user, name); err != nil {
				t.Fatal(err)
			}
		}
		m := tui.NewModel(t.Context(), runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), runtime.Events(), runtime.TimelineView(), logging.Nop())
		m.ConfigureGoals(runtime.Goals(), runtime.GoalAssignments(), runtime.SubjectGoals())
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
			_, back := reply.(goals.BackMsg)
			_, filtered := reply.(listfilter.Reply)
			if back {
				// Home starts a recurring clock; navigation is observable without running it.
				next, _ := m.Update(reply)
				m = next.(tui.Model)
				return
			}
			if goals.Owns(reply) || filtered {
				next, cmd := m.Update(reply)
				m = next.(tui.Model)
				deliver(cmd)
			}
		}
		send := func(msg tea.Msg) { next, cmd := m.Update(msg); m = next.(tui.Model); deliver(cmd) }
		key := func(code rune) { send(tea.KeyPressMsg{Code: code}) }
		text := func(value string) { send(tea.PasteMsg{Content: value}) }
		key(tea.KeyTab)
		key(tea.KeyLeft)
		key(tea.KeyLeft)
		key(tea.KeyEnter)
		if !strings.Contains(m.View().Content, "Goals") {
			t.Fatalf("Goals navigation: %s", m.View().Content)
		}
		send(tea.KeyPressMsg{Code: 'n', Text: "n"})
		text("Practice")
		key(tea.KeyTab)
		text("1")
		for range 5 {
			key(tea.KeyTab)
		}
		key(tea.KeyEnter)
		key(tea.KeyDown)
		key(tea.KeySpace)
		key(tea.KeyDown)
		key(tea.KeySpace)
		key(tea.KeyEnter)
		send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		items, err := runtime.Goals().List(t.Context(), user)
		if err != nil || len(items) != 1 {
			t.Fatalf("created Goals: got %+v, %v; want one", items, err)
		}
		item := items[0]
		live, err := runtime.SubjectGoals().ListLiveSubjectsForGoal(t.Context(), user, item.GoalID)
		if err != nil || len(live) != 2 {
			t.Fatalf("created membership: got %+v, %v; want two", live, err)
		}
		send(tea.KeyPressMsg{Code: 'e', Text: "e"})
		key(tea.KeyTab)
		text("0")
		send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		after, err := runtime.Goals().Get(t.Context(), user, item.GoalID)
		if err != nil || after.Version != item.Version+1 || after.TargetSeconds != 36000 {
			t.Fatalf("edited Goal: got %+v, %v; want version increment and 10 hours", after, err)
		}
		send(tea.KeyPressMsg{Code: 's', Text: "s"})
		key(tea.KeyDown)
		key(tea.KeySpace)
		key(tea.KeyEnter)
		live, err = runtime.SubjectGoals().ListLiveSubjectsForGoal(t.Context(), user, item.GoalID)
		if err != nil || len(live) != 1 {
			t.Fatalf("replaced membership: got %+v, %v; want one", live, err)
		}
		key(tea.KeyEscape)
		key(tea.KeyEscape)
		if !strings.Contains(m.View().Content, "Events") {
			t.Fatalf("return Home: %s", m.View().Content)
		}
	})
}
