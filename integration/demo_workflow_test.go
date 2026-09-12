//go:build integration

package integration_test

import (
	"os"
	"testing"
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
