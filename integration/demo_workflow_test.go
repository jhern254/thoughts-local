//go:build integration

package integration_test

import (
	"os"
	"testing"
)

func TestDemoWorkflow_SQLite(t *testing.T) {
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
