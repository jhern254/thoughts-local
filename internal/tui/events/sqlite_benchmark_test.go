//go:build integration

package events

import (
	"context"
	"database/sql"
	"os"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
	_ "modernc.org/sqlite"
)

type countedEventReads struct {
	Service
	calls atomic.Int64
}

func (s *countedEventReads) List(ctx context.Context, user string, from, until time.Time) ([]data.Event, error) {
	s.calls.Add(1)
	return s.Service.List(ctx, user, from, until)
}

// Each burst launches ordinary List/Counts batches concurrently, but postpones
// result delivery while 40 alternating day keys arrive. The newest request
// must win independently of completion order. This models an overloaded input
// burst, not the latency of a single key on an otherwise idle terminal.
func BenchmarkTimelineBurst(b *testing.B) {
	dsn := os.Getenv("THOUGHTS_STRESS_DSN")
	if dsn == "" {
		b.Skip("set THOUGHTS_STRESS_DSN to a disposable seeded database")
	}
	runtime, err := application.Open(b.Context(), dsn)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = runtime.Close() })
	service := &countedEventReads{Service: runtime.Events()}
	m := New(b.Context(), "demo", service, runtime.TimelineView(), runtime.Thoughts(), logging.Nop())
	current, err := runtime.Events().Get(b.Context(), "demo", 721)
	if err != nil {
		b.Fatal(err)
	}
	at := current.StartedAt.Add(time.Hour)
	m.now = func() time.Time { return at }
	m.clock, m.day = at, displaytime.Day(at)
	m.active = true
	day := m.day
	var latency time.Duration
	var obsolete int64
	b.ReportAllocs()
	for b.Loop() {
		replies := make(chan tea.Msg, 80)
		var finalKey time.Time
		for i := 0; i < 40; i++ {
			key := "left"
			if i%2 != 0 {
				key = "right"
			}
			finalKey = time.Now()
			var cmd tea.Cmd
			m, cmd = m.Update(eventKey(key))
			for _, read := range cmd().(tea.BatchMsg) {
				go func() { replies <- read() }()
			}
		}
		for i := 0; i < 80; i++ {
			var cmd tea.Cmd
			reply := <-replies
			if listed, ok := reply.(Listed); ok && listed.request != m.request && listed.err == nil {
				obsolete++
			}
			m, cmd = m.Update(reply)
			m = execute(b, m, cmd)
			_ = m.View()
			if finalKey != (time.Time{}) && !m.loading && !m.countPending && !m.latestPending {
				latency += time.Since(finalKey)
				finalKey = time.Time{}
			}
		}
		if !m.day.Equal(day) || m.loading || m.err != nil || m.countErr != nil || m.latestErr != nil {
			b.Fatal("burst did not settle on the correct day without errors")
		}
	}
	m.Close()
	b.ReportMetric(float64(service.calls.Load())/float64(b.N), "list_calls/burst")
	b.ReportMetric(float64(obsolete)/float64(b.N), "obsolete_lists/burst")
	b.ReportMetric(float64(latency.Nanoseconds())/float64(b.N)/1e6, "settle-ms/burst")
}

// Run explicitly against a disposable migrated, seeded DB. No setup or writes
// occur in the timed loops. Both demo seeds use the same owned local user.
func BenchmarkTimelineSQLite(b *testing.B) {
	dsn := os.Getenv("THOUGHTS_STRESS_DSN")
	if dsn == "" {
		b.Skip("set THOUGHTS_STRESS_DSN to a disposable seeded database")
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		b.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = db.Close() })
	var start int64
	if err := db.QueryRow("SELECT started_at FROM events WHERE user_id='demo' AND ended_at IS NULL").Scan(&start); err != nil {
		b.Fatal(err)
	}
	now := time.Unix(start+3600, 0)
	day := displaytime.Day(now)
	events, metrics, thoughts := data.NewSQLiteEventStore(db), data.NewSQLiteMetricsStore(db), data.NewSQLiteThoughtStore(db)
	b.Run("List", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := events.ListEvents(b.Context(), "demo", day, day.AddDate(0, 0, 1)); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Counts", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := metrics.ThoughtCountsByEvent(b.Context(), "demo", day, day.AddDate(0, 0, 1), now.Add(time.Second)); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Latest", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := thoughts.LatestThoughtInRange(b.Context(), "demo", time.Unix(start, 0), now.Add(time.Second)); err != nil {
				b.Fatal(err)
			}
		}
	})
}
