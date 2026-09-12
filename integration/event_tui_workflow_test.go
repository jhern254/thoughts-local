//go:build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

func TestEventTUIWorkflow_SQLite(t *testing.T) {
	t.Run("Start atomically closes its predecessor and End persists the selected event", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := runtime.Close(); err != nil {
				t.Error(err)
			}
		})
		previous, err := runtime.Events().Create(t.Context(), runtime.LocalUser().UserID, "previous", time.Now().Add(-time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		var m tea.Model = tui.NewModel(t.Context(), runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), runtime.Events(), runtime.TimelineView(), logging.Nop())
		m, cmd := m.Update(m.Init()())
		// The other startup command is the minute timer; data execution below
		// deliberately leaves time progression to the clock unit tests.
		m = executeEventData(t, m, cmd().(tea.BatchMsg)[0])
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
		m, _ = m.Update(tea.PasteMsg{Content: "new activity"})
		m, cmd = m.Update(tuiKey(tea.KeyEnter))
		m = executeEventData(t, m, cmd)
		var id, start, oldEnd int64
		if err := db.QueryRow(`SELECT event_id,started_at FROM events WHERE ended_at IS NULL`).Scan(&id, &start); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT ended_at FROM events WHERE event_id=?`, previous.EventID).Scan(&oldEnd); err != nil {
			t.Fatal(err)
		}
		if oldEnd != start || id == previous.EventID {
			t.Fatalf("got old end %d, new start %d, id %d; want matching boundary and new id", oldEnd, start, id)
		}
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
		if !strings.Contains(m.View().Content, "End event") {
			t.Fatalf("got %q, want End form", m.View().Content)
		}
		m, cmd = m.Update(tuiKey(tea.KeyEnter))
		m = executeEventData(t, m, cmd)
		var end int64
		if err := db.QueryRow(`SELECT ended_at FROM events WHERE event_id=?`, id).Scan(&end); err != nil {
			t.Fatal(err)
		}
		if end < start {
			t.Fatalf("got end %d, want >= start %d", end, start)
		}
		if !strings.Contains(m.View().Content, displaytime.Format(time.Now(), "January 2, 2006")) {
			t.Fatal("home lost selected date")
		}
	})
}

func executeEventData(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			m = executeEventData(t, m, child)
		}
		return m
	}
	m, next := m.Update(msg)
	return executeEventData(t, m, next)
}
