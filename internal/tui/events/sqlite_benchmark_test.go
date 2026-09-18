//go:build integration

package events

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/metrics"
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
	_ "modernc.org/sqlite"
)

// Measures read readiness, not terminal rendering or the loading-feedback timer.
// List and Counts run as concurrent Bubble Tea commands; Latest starts only
// after List reveals the ongoing event. ListOnly is a matched contention control.
func BenchmarkTimelineDaySwitch(b *testing.B) {
	dsn := os.Getenv("THOUGHTS_STRESS_DSN")
	if dsn == "" {
		b.Skip("set THOUGHTS_STRESS_DSN to a disposable stress-seeded database")
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	db, err := sql.Open("sqlite", dsn+separator+"_pragma=foreign_keys(1)")
	if err != nil {
		b.Fatal(err)
	}
	// Match application.openSQLite, retaining access to pool stats for guards.
	db.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = db.Close() })
	var eventsTotal, thoughtsTotal, ongoingTotal int
	if err := db.QueryRowContext(b.Context(), `SELECT
		(SELECT COUNT(*) FROM events WHERE user_id='demo' AND deleted_at IS NULL),
		(SELECT COUNT(*) FROM thoughts WHERE user_id='demo' AND deleted_at IS NULL),
		(SELECT COUNT(*) FROM events WHERE user_id='demo' AND deleted_at IS NULL AND ended_at IS NULL)`).
		Scan(&eventsTotal, &thoughtsTotal, &ongoingTotal); err != nil {
		b.Fatal(err)
	}
	if eventsTotal != 721 || thoughtsTotal != 20000 || ongoingTotal != 1 {
		b.Fatalf("fixture: got %d events, %d thoughts, %d ongoing; want 721, 20000, 1", eventsTotal, thoughtsTotal, ongoingTotal)
	}
	events := event.NewService(data.NewSQLiteEventStore(db))
	view := timeline.NewService(events, data.NewSQLiteThoughtStore(db), metrics.NewService(data.NewSQLiteMetricsStore(db)))
	ongoing, err := events.Get(b.Context(), "demo", 721)
	if err != nil || ongoing == nil || ongoing.EndedAt != nil {
		b.Fatalf("event 721: got %v, %v; want ongoing stress event", ongoing, err)
	}
	today := displaytime.Day(ongoing.StartedAt.Add(time.Hour))
	for _, scenario := range []struct {
		name    string
		days    [2]time.Time
		current bool
	}{
		{"Historical", [2]time.Time{today.AddDate(0, 0, -3), today.AddDate(0, 0, -2)}, false},
		{"Current", [2]time.Time{today, today}, true},
	} {
		// Validate both navigation destinations before timing. Counts must cover
		// exactly the listed event IDs, including zero-count events.
		for _, day := range scenario.days {
			items, err := events.List(b.Context(), "demo", day, day.AddDate(0, 0, 1))
			if err != nil || len(items) == 0 {
				b.Fatalf("%s List: got %d events, %v; want populated day", day, len(items), err)
			}
			counts, err := view.ThoughtCounts(b.Context(), "demo", day, day.AddDate(0, 0, 1))
			if err != nil || len(counts) != len(items) {
				b.Fatalf("%s Counts: got %d, %v; want %d events", day, len(counts), err, len(items))
			}
			ids := make(map[int64]int64, len(counts))
			for _, count := range counts {
				ids[count.EventID] = count.Count
			}
			foundOngoing := false
			for _, item := range items {
				if _, ok := ids[item.EventID]; !ok {
					b.Fatalf("%s counts missing listed event %d", day, item.EventID)
				}
				foundOngoing = foundOngoing || item.EndedAt == nil
			}
			if foundOngoing != scenario.current || (scenario.current && ids[721] != 200) {
				b.Fatalf("%s: want ongoing=%t and 200 ongoing thoughts; got ongoing=%t, count=%d", day, scenario.current, foundOngoing, ids[721])
			}
		}
		if scenario.current {
			latest, err := view.LatestThought(b.Context(), "demo", 721)
			if err != nil || latest == nil || latest.ThoughtID != 20000 {
				b.Fatalf("latest: got %v, %v; want stress thought 20000", latest, err)
			}
		}
		for _, concurrent := range []bool{false, true} {
			name := scenario.name + "/ListOnly"
			if concurrent {
				name = scenario.name + "/Batch"
			}
			b.Run(name, func(b *testing.B) {
				type completion struct {
					msg   tea.Msg
					ready time.Duration
				}
				var listReady, countsReady, latestReady, settled time.Duration
				var countsFirst, iteration int
				waitsBefore := db.Stats().WaitCount
				ctx := b.Context()
				b.ReportAllocs()
				for b.Loop() {
					day := scenario.days[iteration%2]
					iteration++
					replies := make(chan completion, 3)
					var workers sync.WaitGroup
					start := time.Now()
					launch := func(cmd tea.Cmd) {
						workers.Add(1)
						go func() {
							defer workers.Done()
							msg := cmd()
							replies <- completion{msg, time.Since(start)}
						}()
					}
					// Same launch order and independent commands as loadDay's batch.
					launch(func() tea.Msg {
						items, err := events.List(ctx, "demo", day, day.AddDate(0, 0, 1))
						return listedMsg{items: items, err: err}
					})
					pending := 1
					if concurrent {
						pending++
						launch(func() tea.Msg {
							items, err := view.ThoughtCounts(ctx, "demo", day, day.AddDate(0, 0, 1))
							return countsMsg{items: items, err: err}
						})
					}
					var listAt, countAt, fullAt time.Duration
					var readErr error
					for pending > 0 {
						reply := <-replies
						pending--
						fullAt = max(fullAt, reply.ready)
						switch result := reply.msg.(type) {
						case listedMsg:
							listAt = reply.ready
							if result.err != nil {
								readErr = result.err
							}
							if concurrent {
								for _, item := range result.items {
									if item.EndedAt == nil {
										pending++
										launch(func() tea.Msg {
											latest, err := view.LatestThought(ctx, "demo", item.EventID)
											return latestMsg{item: latest, err: err}
										})
									}
								}
							}
						case countsMsg:
							countAt = reply.ready
							if result.err != nil {
								readErr = result.err
							}
						case latestMsg:
							latestReady += reply.ready
							if result.err != nil {
								readErr = result.err
							}
						}
					}
					workers.Wait() // Drain every command, including on errors.
					if readErr != nil {
						b.Fatal(readErr)
					}
					listReady += listAt
					countsReady += countAt
					settled += fullAt
					if concurrent && countAt < listAt {
						countsFirst++
					}
				}
				if stats := db.Stats(); stats.InUse != 0 || stats.MaxOpenConnections != 1 {
					b.Fatalf("pool: got %+v; want one-connection limit and no pending reads", stats)
				}
				b.ReportMetric(float64(listReady)/float64(b.N)/1e6, "list-ready-ms/op")
				b.ReportMetric(float64(db.Stats().WaitCount-waitsBefore)/float64(b.N), "pool-waits/op")
				if concurrent {
					b.ReportMetric(float64(countsReady)/float64(b.N)/1e6, "counts-ready-ms/op")
					b.ReportMetric(100*float64(countsFirst)/float64(b.N), "counts-first-%")
					if scenario.current {
						b.ReportMetric(float64(latestReady)/float64(b.N)/1e6, "latest-ready-ms/op")
					}
				}
				b.ReportMetric(float64(settled)/float64(b.N)/1e6, "full-settle-ms/op")
			})
		}
	}
}

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
		replies := make(chan tea.Msg, 120)
		pending := 0
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
				pending++
				go func() { replies <- read() }()
			}
			_ = m.View()
		}
		for i := 0; i < pending; i++ {
			var cmd tea.Cmd
			reply := <-replies
			if reply == nil {
				continue // Bubble Tea also ignores nil command results.
			}
			if listed, ok := reply.(listedMsg); ok && listed.request != m.load.generation && listed.err == nil {
				obsolete++
			}
			m, cmd = m.Update(reply)
			m = execute(b, m, cmd)
			_ = m.View()
			if finalKey != (time.Time{}) && !m.load.eventsPending && !m.load.countsPending && !m.load.latestPending {
				latency += time.Since(finalKey)
				finalKey = time.Time{}
			}
		}
		if !m.day.Equal(day) || m.load.eventsPending || m.err != nil || m.load.countErr != nil || m.load.latestErr != nil {
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
