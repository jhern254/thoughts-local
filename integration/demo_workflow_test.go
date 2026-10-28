//go:build integration

package integration_test

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestDemoWorkflow_SQLite(t *testing.T) {
	t.Run("stress diary has valid intervals and enough thoughts for bounded browsing", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		const anchor = 1789151820
		if _, err := db.Exec(`CREATE TEMP TABLE demo_clock AS SELECT ? AS at`, anchor); err != nil {
			t.Fatal(err)
		}
		seed, err := os.ReadFile("../scripts/demo_stress.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(seed)); err != nil {
			t.Fatal(err)
		}
		var events, ongoing, thoughts, misc, current, invalid int
		err = db.QueryRow(`SELECT
			(SELECT COUNT(*) FROM events),
			(SELECT COUNT(*) FROM events WHERE ended_at IS NULL),
			(SELECT COUNT(*) FROM thoughts),
			(SELECT COUNT(*) FROM thoughts WHERE subject_id IS NULL),
			(SELECT COUNT(*) FROM thoughts WHERE observed_at >= (SELECT started_at FROM events WHERE ended_at IS NULL)),
			(SELECT COUNT(*) FROM events e WHERE e.started_at > ? OR e.ended_at > ?
			 OR EXISTS (SELECT 1 FROM events p WHERE p.event_id < e.event_id AND p.ended_at > e.started_at))
			 + (SELECT COUNT(*) FROM thoughts WHERE observed_at > ? OR user_id <> 'demo')`, anchor, anchor, anchor).
			Scan(&events, &ongoing, &thoughts, &misc, &current, &invalid)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := [6]int{events, ongoing, thoughts, misc, current, invalid}, [6]int{721, 1, 20000, 4000, 200, 0}; got != want {
			t.Fatalf("got events/ongoing/thoughts/Misc/current/invalid %v, want %v", got, want)
		}
		var integrity string
		if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
			t.Fatal(err)
		}
		if integrity != "ok" {
			t.Fatalf("got integrity %q, want ok", integrity)
		}
	})
	t.Run("stress diary assigns matching event subjects and supplies daily time metrics", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		const anchor = 1789151820
		if _, err := db.Exec(`CREATE TEMP TABLE demo_clock AS SELECT ? AS at`, anchor); err != nil {
			t.Fatal(err)
		}
		seed, err := os.ReadFile("../scripts/demo_stress.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(seed)); err != nil {
			t.Fatal(err)
		}
		rows, err := db.QueryContext(t.Context(), `SELECT e.activity_type, COALESCE(s.subject_name, '')
FROM events e LEFT JOIN subjects s ON s.subject_id=e.subject_id AND s.user_id=e.user_id
GROUP BY e.activity_type, s.subject_name`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		assignments := map[string]string{}
		for rows.Next() {
			var activity, subject string
			if err := rows.Scan(&activity, &subject); err != nil {
				t.Fatal(err)
			}
			if _, exists := assignments[activity]; exists {
				t.Fatalf("activity %q has inconsistent subjects", activity)
			}
			assignments[activity] = subject
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		wantAssignments := map[string]string{
			"Read my own source code":           "Building myself",
			"Build the thoughts app":            "Building myself",
			"Test what I confidently broke":     "Testing reality",
			"Review a design decision":          "Design decisions",
			"Write docs for future me":          "Documentation",
			"Defragment my imaginary desk":      "",
			"Make my own timeline feel instant": "Building myself",
		}
		if !reflect.DeepEqual(assignments, wantAssignments) {
			t.Fatalf("event subjects: got %v, want %v", assignments, wantAssignments)
		}
		zone, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			t.Fatal(err)
		}
		from := time.Date(2026, time.September, 11, 0, 0, 0, 0, zone)
		got, err := data.NewSQLiteMetricsStore(db).SubjectTimeInRange(t.Context(), "demo", from, from.AddDate(0, 0, 1), time.Unix(anchor, 0))
		if err != nil {
			t.Fatal(err)
		}
		// Building includes 37 minutes clipped at midnight, two completed hours, and one ongoing hour.
		want := []data.SubjectTimeView{
			{SubjectID: 1, SubjectName: "Building myself", TimeSpentSeconds: 13020},
			{SubjectID: 3, SubjectName: "Design decisions", TimeSpentSeconds: 7200},
			{SubjectID: 4, SubjectName: "Documentation", TimeSpentSeconds: 7200},
			{SubjectID: 2, SubjectName: "Testing reality", TimeSpentSeconds: 7200},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("daily subject time: got %+v, want %+v", got, want)
		}
	})

	t.Run("stress diary supplies three mindsets per populated day without accumulating active history", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		const anchor = 1789151820
		if _, err := db.Exec(`CREATE TEMP TABLE demo_clock AS SELECT ? AS at`, anchor); err != nil {
			t.Fatal(err)
		}
		seed, err := os.ReadFile("../scripts/demo_stress.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(seed)); err != nil {
			t.Fatal(err)
		}
		rows, err := db.QueryContext(t.Context(), `SELECT DISTINCT
			unixepoch(observed_at, 'unixepoch', 'localtime', 'start of day', 'utc')
			FROM thoughts ORDER BY 1`)
		if err != nil {
			t.Fatal(err)
		}
		var days []int64
		for rows.Next() {
			var start int64
			if err := rows.Scan(&start); err != nil {
				t.Fatal(err)
			}
			days = append(days, start)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if len(days) < 28 {
			t.Fatalf("populated days: got %d, want at least 28", len(days))
		}
		store := data.NewSQLiteThoughtStore(db)
		for _, start := range days {
			from := time.Unix(start, 0).In(time.Local)
			items, err := store.ListMindsetsInRange(t.Context(), "demo", from, from.AddDate(0, 0, 1))
			if err != nil {
				t.Fatal(err)
			}
			if got, want := len(items), 3; got != want {
				t.Fatalf("mindsets on %s: got %d, want %d", from.Format(time.DateOnly), got, want)
			}
			for _, item := range items {
				if got, want := item.IsMindset, from.YearDay() == time.Unix(anchor, 0).In(time.Local).YearDay(); got != want {
					t.Fatalf("active mindset on %s: got %v, want %v", from.Format(time.DateOnly), got, want)
				}
			}
		}
	})

	t.Run("seeds a local user with subjects thoughts and a current event", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seed, err := os.ReadFile("../scripts/demo.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(seed)); err != nil {
			t.Fatal(err)
		}
		var subjects, thoughts, misc, completed, currentThoughts int
		err = db.QueryRow(`SELECT
			(SELECT COUNT(*) FROM subjects WHERE user_id = u.user_id),
			(SELECT COUNT(*) FROM thoughts WHERE user_id = u.user_id),
			(SELECT COUNT(*) FROM thoughts WHERE user_id = u.user_id AND subject_id IS NULL),
			(SELECT COUNT(*) FROM events WHERE user_id = u.user_id AND ended_at IS NOT NULL),
			(SELECT COUNT(*) FROM thoughts t JOIN events e ON e.user_id = t.user_id
			 WHERE t.user_id = u.user_id AND e.ended_at IS NULL AND t.observed_at >= e.started_at)
			FROM users u WHERE handle = 'local'`).Scan(&subjects, &thoughts, &misc, &completed, &currentThoughts)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := [5]int{subjects, thoughts, misc, completed, currentThoughts}, [5]int{2, 12, 2, 2, 8}; got != want {
			t.Fatalf("got subjects/thoughts/Misc/completed/current thoughts %v, want %v", got, want)
		}
	})
}
