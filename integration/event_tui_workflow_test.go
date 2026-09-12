//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/metrics"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

func TestEventTUIWorkflow_SQLite(t *testing.T) {
	t.Run("superseded reads leave the connection queue and writes still succeed", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		events := event.NewService(data.NewSQLiteEventStore(db))
		thoughts := data.NewSQLiteThoughtStore(db)
		counts := metrics.NewService(data.NewSQLiteMetricsStore(db))
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		var m tea.Model = tui.NewModel(ctx, &data.User{UserID: "u"}, subject.NewService(data.NewSQLiteSubjectStore(db)), thought.NewService(thoughts), counts, events, timeline.NewService(events, thoughts, counts), logger)
		// Hold the only connection until both real startup reads are queued.
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		m, cmd := m.Update(m.Init()())
		batch := cmd().(tea.BatchMsg)[0]().(tea.BatchMsg)
		replies := make(chan tea.Msg, len(batch))
		waits := db.Stats().WaitCount
		for _, read := range batch {
			go func() { replies <- read() }()
		}
		for db.Stats().WaitCount < waits+int64(len(batch)) {
			if ctx.Err() != nil {
				t.Fatal("reads did not reach the connection queue")
			}
			runtime.Gosched()
		}
		m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r'}))
		before := m.View().Content
		for range batch {
			select {
			case reply := <-replies:
				m, _ = m.Update(reply)
			case <-ctx.Done():
				t.Fatal("superseded reads did not leave the connection queue")
			}
		}
		if m.View().Content != before || logs.Len() != 0 {
			t.Fatalf("got stale view change or logs %q, want unchanged loading and no cancellation log", logs.String())
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
		m = executeEventData(t, m, cmd)
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'n'}))
		m, _ = m.Update(tea.PasteMsg{Content: "Recovered after read cancellation"})
		m, cmd = m.Update(tuiKey(tea.KeyEnter))
		m = executeEventData(t, m, cmd)
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'e'}))
		m, cmd = m.Update(tuiKey(tea.KeyEnter))
		m = executeEventData(t, m, cmd)
		var completed int
		if err := db.QueryRow("SELECT COUNT(*) FROM events WHERE ended_at IS NOT NULL").Scan(&completed); err != nil {
			t.Fatal(err)
		}
		if completed != 1 || strings.Contains(m.View().Content, "Loading events") || db.Stats().InUse != 0 {
			t.Fatalf("got completed %d, in-use %d, view %q; want one completed event, no held connection or pending load", completed, db.Stats().InUse, m.View().Content)
		}
	})
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
		m, _ = m.Update(tuiKey(tea.KeyHome))
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
		m, _ = m.Update(tea.PasteMsg{Content: "new activity"})
		m, cmd = m.Update(tuiKey(tea.KeyEnter))
		m = executeEventData(t, m, cmd)
		view := m.View().Content
		if box, now := strings.Index(view, "new activity"), strings.Index(view, "Now ·"); box < 0 || now < box {
			t.Fatalf("got view %q, want new event visible above Now immediately after saving", view)
		}
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
