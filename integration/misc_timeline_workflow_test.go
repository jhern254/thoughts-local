//go:build integration

package integration_test

import (
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/metrics"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
	"strings"
)

func TestMiscTimelineWorkflow_SQLite(t *testing.T) {
	t.Run("groups uncovered timestamps independently of subject and explicit event links", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other", "deleted")
		for _, query := range []string{
			`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','Work')`,
			`INSERT INTO events(event_id,user_id,started_at,ended_at,deleted_at) VALUES (1,'u',100,200,NULL),(2,'u',250,250,NULL),(3,'u',300,NULL,NULL),(4,'other',200,250,NULL),(5,'u',200,250,unixepoch())`,
			`INSERT INTO thoughts(user_id,subject_id,event_id,thought,observed_at) VALUES ('u',NULL,NULL,'before day',49),('u',1,NULL,'assigned',50),('u',NULL,1,'explicitly linked',50),('u',NULL,NULL,'event start',100),('u',NULL,NULL,'inside event',199),('u',NULL,NULL,'event end',200),('u',NULL,NULL,'zero duration',250),('u',NULL,NULL,'ongoing start',300),('u',NULL,NULL,'current second',400),('u',NULL,NULL,'future',401),('other',NULL,NULL,'foreign',50),('deleted',NULL,NULL,'hidden owner',50)`,
			`INSERT INTO thoughts(user_id,thought,observed_at,deleted_at) VALUES ('u','hidden thought',50,unixepoch())`,
			`UPDATE users SET deleted_at=unixepoch(),updated_at=unixepoch() WHERE user_id='deleted'`,
		} {
			if _, err := db.Exec(query); err != nil {
				t.Fatal(err)
			}
		}
		store := data.NewSQLiteMetricsStore(db)
		for _, user := range []string{"u", "other", "deleted", "missing"} {
			groups, err := store.MiscThoughtGroups(t.Context(), user, time.Unix(50, 0), time.Unix(500, 0), time.Unix(401, 0))
			if err != nil {
				t.Fatal(err)
			}
			var got [][2]int64
			for _, group := range groups {
				got = append(got, [2]int64{group.ObservedAt.Unix(), group.Count})
			}
			var want [][2]int64
			if user == "u" {
				want = [][2]int64{{50, 2}, {200, 1}, {250, 1}}
			}
			if user == "other" {
				want = [][2]int64{{50, 1}}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("groups for %s: got %v, want %v", user, got, want)
			}
		}
	})
	t.Run("shows thoughts without events and regroups after event changes without writing synthetic events", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`INSERT INTO thoughts(user_id,thought,observed_at) VALUES ('u','first',100),('u','same second',100),('u','later',200)`); err != nil {
			t.Fatal(err)
		}
		thoughts := data.NewSQLiteThoughtStore(db)
		events := event.NewService(data.NewSQLiteEventStore(db))
		service := timeline.NewService(events, thoughts, metrics.NewService(data.NewSQLiteMetricsStore(db)))
		from, until := time.Unix(50, 0), time.Unix(300, 0)
		entries, err := service.Day(t.Context(), "u", from, until)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 || entries[0].Misc == nil || entries[0].Misc.Count != 2 || entries[1].Misc.ObservedAt.Unix() != 200 {
			t.Fatalf("day: got %+v, want two Misc groups", entries)
		}
		scope, err := service.OpenMiscThoughtsView(t.Context(), "u", time.Unix(100, 0))
		if err != nil {
			t.Fatal(err)
		}
		page, err := service.BrowseThoughtsView(t.Context(), "u", scope, data.ThoughtSummaryViewRequest{})
		if err != nil || len(page.Items) != 2 {
			t.Fatalf("Misc page: got %+v, %v, want two thoughts", page, err)
		}
		stats, err := service.ThoughtStats(t.Context(), "u", scope)
		if err != nil || stats.Count != 2 {
			t.Fatalf("Misc stats: got %+v, %v, want count 2", stats, err)
		}
		if _, err := service.BrowseThoughtsView(t.Context(), "other", scope, data.ThoughtSummaryViewRequest{}); err != data.ErrRecordNotFound {
			t.Fatalf("foreign scope: got %v, want record not found", err)
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("persisted events: got %d, want 0", count)
		}
		real, err := events.CreatePast(t.Context(), "u", "Walking", time.Unix(100, 0), time.Unix(200, 0), nil)
		if err != nil {
			t.Fatal(err)
		}
		entries, err = service.Day(t.Context(), "u", from, until)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 || entries[0].Event == nil || entries[0].Event.EventID != real.EventID || entries[1].Misc == nil {
			t.Fatalf("regrouped day: got %+v, want real event then Misc", entries)
		}
		if _, err := service.OpenMiscThoughtsView(t.Context(), "u", time.Unix(100, 0)); err != data.ErrRecordNotFound {
			t.Fatalf("covered Misc: got %v, want record not found", err)
		}
		if _, err := db.Exec(`UPDATE events SET deleted_at=updated_at WHERE event_id=?`, real.EventID); err != nil {
			t.Fatal(err)
		}
		entries, err = service.Day(t.Context(), "u", from, until)
		if err != nil || len(entries) != 2 || entries[0].Misc == nil {
			t.Fatalf("after deletion: got %+v, %v, want two Misc groups", entries, err)
		}
	})
}

func TestMiscTimelineTUIWorkflow_SQLite(t *testing.T) {
	t.Run("opens a thought on an event-free day through root dispatch", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		// Yesterday's last second is always historical, even just after midnight.
		at := displaytime.Day(time.Now()).Add(-time.Second)
		if _, err := db.Exec(`INSERT INTO thoughts(user_id,thought,observed_at) VALUES ('u','Uncovered observation',?)`, at.Unix()); err != nil {
			t.Fatal(err)
		}
		events := event.NewService(data.NewSQLiteEventStore(db))
		thoughts := data.NewSQLiteThoughtStore(db)
		counts := metrics.NewService(data.NewSQLiteMetricsStore(db))
		var m tea.Model = tui.NewModel(t.Context(), &data.User{UserID: "u"}, subject.NewService(data.NewSQLiteSubjectStore(db)), thought.NewService(thoughts), counts, events, timeline.NewService(events, thoughts, counts), logging.Nop())
		m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
		m, cmd := m.Update(m.Init()())
		m = executeEventData(t, m, cmd().(tea.BatchMsg)[0])
		m, cmd = m.Update(tuiKey(tea.KeyLeft))
		m = executeEventData(t, m, cmd)
		if view := m.View().Content; !strings.Contains(view, "Misc") || !strings.Contains(view, "1 thought") || strings.Contains(view, "No events on this day") {
			t.Fatalf("got timeline %q, want a Misc card with one thought", view)
		}
		m, cmd = m.Update(tuiKey(tea.KeyEnter))
		m = executeEventData(t, m, cmd)
		expanded := m.View().Content
		if !strings.Contains(expanded, "Uncovered observation") {
			t.Fatalf("got expanded %q, want thought preview", expanded)
		}
		m, cmd = m.Update(tuiKey(tea.KeyEnter))
		m = executeEventData(t, m, cmd)
		if view := m.View().Content; strings.Contains(view, "Newest first") || !strings.Contains(view, "Uncovered observation") {
			t.Fatalf("got detail %q, want standalone thought", view)
		}
		m, cmd = m.Update(tuiKey(tea.KeyLeft))
		m = executeEventData(t, m, cmd)
		if got := m.View().Content; got != expanded {
			t.Fatalf("detail return: got %q, want originating expanded card %q", got, expanded)
		}
	})
}
