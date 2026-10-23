//go:build integration

package integration_test

import (
	"os"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestDayActivityDemoWorkflow_SQLite(t *testing.T) {
	t.Run("supplies varied long history and a populated current event", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		anchor := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
		if _, err := db.Exec(`CREATE TEMP TABLE demo_clock AS SELECT ? AS at`, anchor.Unix()); err != nil {
			t.Fatal(err)
		}
		seed, err := os.ReadFile("../scripts/demo_activity.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(seed)); err != nil {
			t.Fatal(err)
		}
		var events, thoughts, ongoing, current, invalid int
		err = db.QueryRow(`SELECT (SELECT count(*) FROM events), (SELECT count(*) FROM thoughts),
   (SELECT count(*) FROM events WHERE ended_at IS NULL),
   (SELECT count(*) FROM thoughts WHERE observed_at>=(SELECT started_at FROM events WHERE ended_at IS NULL)),
   (SELECT count(*) FROM events WHERE started_at>? OR ended_at>?) + (SELECT count(*) FROM thoughts WHERE observed_at>?)`, anchor.Unix(), anchor.Unix(), anchor.Unix()).Scan(&events, &thoughts, &ongoing, &current, &invalid)
		if err != nil {
			t.Fatal(err)
		}
		if events < 3000 || thoughts < 20000 || ongoing != 1 || current != 200 || invalid != 0 {
			t.Fatalf("fixture events/thoughts/ongoing/current/future: got %d/%d/%d/%d/%d, want >=3000/>=20000/1/200/0", events, thoughts, ongoing, current, invalid)
		}
		store := data.NewSQLiteMetricsStore(db)
		first, err := store.ActivityStart(t.Context(), "demo", anchor.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if !first.Before(anchor.AddDate(0, 0, -700)) {
			t.Fatalf("first activity: got %v, want over 700 days ago", first)
		}
		boundaries := make([]time.Time, 121)
		for i := range boundaries {
			boundaries[i] = anchor.Add(-12*time.Hour).AddDate(0, 0, i-120)
		}
		counts, err := store.ActivityCounts(t.Context(), "demo", boundaries, anchor.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		var quiet, eventHeavy, thoughtHeavy, crowded bool
		for _, count := range counts {
			quiet = quiet || count.EventDays == 0 && count.Thoughts == 0
			eventHeavy = eventHeavy || count.EventDays >= 8 && count.Thoughts < 15
			thoughtHeavy = thoughtHeavy || count.EventDays <= 2 && count.Thoughts >= 60
			crowded = crowded || count.EventDays >= 18 && count.Thoughts >= 80
		}
		if !quiet || !eventHeavy || !thoughtHeavy || !crowded {
			t.Fatalf("day profiles quiet/Event-heavy/Thought-heavy/crowded: got %v/%v/%v/%v, want all true", quiet, eventHeavy, thoughtHeavy, crowded)
		}
		recentCounts := make(map[data.ActivityCount]bool)
		for _, count := range counts[len(counts)-14:] {
			recentCounts[count] = true
		}
		if got := len(recentCounts); got < 6 {
			t.Fatalf("recent day profiles: got %d, want at least six distinct mountain samples", got)
		}

		rows, err := db.Query(`PRAGMA foreign_key_check`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Fatal("fixture has a foreign-key violation")
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	})
}
