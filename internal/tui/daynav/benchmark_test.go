package daynav

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
	_ "modernc.org/sqlite"
)

// Uses an explicitly supplied disposable, migrated stress fixture. Setup and
// rendering are separate from the database timing; navigation uses loaded data.
func BenchmarkDayActivity(b *testing.B) {
	dsn := os.Getenv("THOUGHTS_STRESS_DSN")
	expectedEvents, expectedThoughts := 721, 20000
	if activity := os.Getenv("THOUGHTS_ACTIVITY_DSN"); activity != "" {
		dsn = activity
		expectedEvents, expectedThoughts = 3927, 29479
	}
	if dsn == "" {
		b.Skip("set THOUGHTS_STRESS_DSN or THOUGHTS_ACTIVITY_DSN to its migrated synthetic fixture")
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var events, thoughts int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM events WHERE user_id='demo'),(SELECT count(*) FROM thoughts WHERE user_id='demo')`).Scan(&events, &thoughts); err != nil {
		b.Fatal(err)
	}
	if events != expectedEvents || thoughts != expectedThoughts {
		b.Fatalf("fixture: got %d Events, %d Thoughts; want %d/%d", events, thoughts, expectedEvents, expectedThoughts)
	}
	store := data.NewSQLiteMetricsStore(db)
	ctx := context.Background()
	cutoff := time.Now()
	today := displaytime.Day(cutoff)
	first, err := store.ActivityStart(ctx, "demo", cutoff)
	if err != nil {
		b.Fatal(err)
	}
	first = displaytime.Day(first)
	for _, width := range []int{80, 160} {
		name := "80"
		if width == 160 {
			name = "160"
		}
		days := data.CalendarDays(first, today.AddDate(0, 0, 1))
		bins := min(data.MaxActivityBuckets, width*2, days)
		boundaries := make([]time.Time, bins+1)
		for i := range boundaries {
			boundaries[i] = first.AddDate(0, 0, days*i/bins)
		}
		b.Run("Read/"+name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := store.ActivityCounts(ctx, "demo", boundaries, cutoff); err != nil {
					b.Fatal(err)
				}
			}
		})
		m := New(ctx, "demo", store)
		m.Resize(width, 40)
		m = m.Update(m.Sync(today, true, false)())
		defer m.Close()
		b.Run("Load/"+name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				cmd := m.Sync(today, true, true)
				m = m.Update(cmd())
			}
		})
		m.SetFocused(true)
		b.Run("Rebuild/"+name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				m.rebuild()
			}
		})
		b.Run("Navigate/"+name, func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				day := today.AddDate(0, 0, -i%10)
				i++
				if cmd := m.Sync(day, true, false); cmd != nil {
					b.Fatal("navigation queried history")
				}
				_ = m.View()
				_ = m.Marker()
			}
		})
	}
}
