//go:build integration

package integration_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
)

func TestSpiderDemoWorkflow_SQLite(t *testing.T) {
	seed := func(t *testing.T, db *sql.DB) {
		t.Helper()
		body, err := os.ReadFile("../scripts/demo_spider_man.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(body)); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("seeds a readable diary with daily mindsets and usable aliases", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seed(t, db)
		var subjects, events, thoughts, days, mindsets, aliases, future int
		err := db.QueryRowContext(t.Context(), `SELECT
   (SELECT COUNT(*) FROM subjects), (SELECT COUNT(*) FROM events),
   (SELECT COUNT(*) FROM thoughts),
   (SELECT COUNT(DISTINCT date(observed_at,'unixepoch','localtime')) FROM thoughts),
   (SELECT COUNT(*) FROM thought_mindset_periods WHERE ended_at IS NULL),
   (SELECT COUNT(*) FROM subject_aliases),
   (SELECT COUNT(*) FROM thoughts WHERE observed_at>unixepoch('now'))`).Scan(&subjects, &events, &thoughts, &days, &mindsets, &aliases, &future)
		if err != nil {
			t.Fatal(err)
		}
		if subjects != 8 || events < 60 || thoughts < 120 || days != 15 || mindsets != 3 || aliases < 8 || future != 0 {
			t.Fatalf("got subjects/events/thoughts/days/current mindsets/aliases/future %v, want 8/>=60/>=120/15/3/>=8/0", []int{subjects, events, thoughts, days, mindsets, aliases, future})
		}
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		for _, name := range []string{"Web Slinging", "Swinging"} {
			resolved, err := service.ResolveName(t.Context(), "spider-demo", name)
			if err != nil {
				t.Fatal(err)
			}
			if resolved.SubjectName != "Web Slinging" || resolved.SubjectID != 7 {
				t.Fatalf("resolve %q: got %q/%d, want Web Slinging/7", name, resolved.SubjectName, resolved.SubjectID)
			}
		}
		var invalidDays int
		err = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM (
   SELECT date(started_at,'unixepoch','localtime') FROM thought_mindset_periods
   GROUP BY date(started_at,'unixepoch','localtime') HAVING COUNT(*)<3)`).Scan(&invalidDays)
		if err != nil {
			t.Fatal(err)
		}
		if invalidDays != 0 {
			t.Fatalf("days with fewer than three mindsets: got %d, want 0", invalidDays)
		}
		rows, err := db.QueryContext(t.Context(), `PRAGMA foreign_key_check`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Fatal("seed violates a foreign key")
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("restart and reseeding preserve edits deletions and new entries", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		seed(t, db)
		_, err := db.ExecContext(t.Context(), `UPDATE thoughts SET thought='My edited diary entry' WHERE thought_id=1;
   UPDATE thoughts SET deleted_at=unixepoch('now'),updated_at=unixepoch('now') WHERE thought_id=2;
   INSERT INTO thoughts(user_id,thought) VALUES('spider-demo','My continuation');`)
		if err != nil {
			t.Fatal(err)
		}
		var before int
		var originalObservedAt int64
		if err := db.QueryRowContext(t.Context(), `SELECT (SELECT COUNT(*) FROM thoughts), observed_at FROM thoughts WHERE thought_id=1`).Scan(&before, &originalObservedAt); err != nil {
			t.Fatal(err)
		}
		reopened, err := sql.Open("sqlite", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		seed(t, reopened)
		var count, deleted int
		var observedAt int64
		var first, continuation string
		err = reopened.QueryRowContext(t.Context(), `SELECT
   (SELECT COUNT(*) FROM thoughts),
   (SELECT thought FROM thoughts WHERE thought_id=1),
   (SELECT COUNT(*) FROM thoughts WHERE thought_id=2 AND deleted_at IS NOT NULL),
   (SELECT thought FROM thoughts ORDER BY thought_id DESC LIMIT 1),
   (SELECT observed_at FROM thoughts WHERE thought_id=1)`).Scan(&count, &first, &deleted, &continuation, &observedAt)
		if err != nil {
			t.Fatal(err)
		}
		if observedAt != originalObservedAt {
			t.Fatalf("reseeded observation time: got %d, want %d", observedAt, originalObservedAt)
		}
		if count != before || first != "My edited diary entry" || deleted != 1 || continuation != "My continuation" {
			t.Fatalf("got count/edit/deleted/continuation %d/%q/%d/%q, want %d/preserved/1/preserved", count, first, deleted, continuation, before)
		}
	})
	t.Run("does not seed a database that already belongs to a user", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		if _, err := db.ExecContext(t.Context(), `INSERT INTO users(user_id,handle) VALUES('owner','local')`); err != nil {
			t.Fatal(err)
		}
		seed(t, db)
		var users, thoughts int
		if err := db.QueryRowContext(t.Context(), `SELECT (SELECT COUNT(*) FROM users),(SELECT COUNT(*) FROM thoughts)`).Scan(&users, &thoughts); err != nil {
			t.Fatal(err)
		}
		if users != 1 || thoughts != 0 {
			t.Fatalf("got users/thoughts %d/%d, want 1/0", users, thoughts)
		}
	})
}
