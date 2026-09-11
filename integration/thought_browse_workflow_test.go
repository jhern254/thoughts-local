//go:build integration

package integration_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestThoughtBrowseWorkflow_SQLite(t *testing.T) {
	t.Run("uses the ordered active-owner index for cursor traversal", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		rows, err := db.Query(`EXPLAIN QUERY PLAN
			SELECT t.thought_id, substr(t.thought,1,81), s.subject_name, t.observed_at, t.created_at
			FROM thoughts t LEFT JOIN subjects s
			ON s.subject_id=t.subject_id AND s.user_id=t.user_id AND s.deleted_at IS NULL
			WHERE t.user_id=? AND t.deleted_at IS NULL
			AND EXISTS (SELECT 1 FROM users u WHERE u.user_id=t.user_id AND u.deleted_at IS NULL)
			AND (t.observed_at,t.created_at,t.thought_id)<(?,?,?)
			ORDER BY t.observed_at DESC,t.created_at DESC,t.thought_id DESC LIMIT 51`, "u", 300, 100, 7)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		plan := ""
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan += detail + "\n"
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(plan, "USING INDEX idx_thoughts_active_user_observed_created_id") || strings.Contains(plan, "TEMP B-TREE") {
			t.Fatalf("got plan %s, want indexed cursor traversal without temporary sorting", plan)
		}
	})
	t.Run("orders observations before creation time and joins only active owned subjects", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other")
		if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES(1,'u','Writing'),(2,'u','Deleted')`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE subjects SET deleted_at=updated_at WHERE subject_id=2`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,subject_id,thought,observed_at,created_at,updated_at) VALUES
			(1,'u',1,'one',300,100,100),(2,'u',NULL,'two',200,400,400),
			(3,'u',2,'three',300,200,200),(4,'u',NULL,'four',300,200,200),
			(5,'other',NULL,'other',500,500,500),(6,'u',NULL,'deleted',600,600,600)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE thoughts SET deleted_at=updated_at WHERE thought_id=6`); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteThoughtStore(db)
		page, err := store.BrowseThoughts(t.Context(), "u", data.ThoughtPageRequest{})
		if err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, item := range page.Items {
			ids = append(ids, item.ThoughtID)
		}
		if want := []int64{4, 3, 1, 2}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("got IDs %v, want %v", ids, want)
		}
		if page.Items[1].SubjectName != nil || page.Items[2].SubjectName == nil || *page.Items[2].SubjectName != "Writing" || page.More {
			t.Fatalf("got page %+v, want only active subject label and no more rows", page)
		}
		if _, err := db.Exec(`UPDATE users SET deleted_at=updated_at WHERE user_id='u'`); err != nil {
			t.Fatal(err)
		}
		page, err = store.BrowseThoughts(t.Context(), "u", data.ThoughtPageRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 0 {
			t.Fatalf("got %d inactive-owner rows, want 0", len(page.Items))
		}
	})
	t.Run("walks tied timestamps in both directions without offset shifts", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		for id := 1; id <= 120; id++ {
			if _, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought,observed_at,created_at,updated_at) VALUES(?,'u','text',100,100,100)`, id); err != nil {
				t.Fatal(err)
			}
		}
		store := data.NewSQLiteThoughtStore(db)
		first, err := store.BrowseThoughts(t.Context(), "u", data.ThoughtPageRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if len(first.Items) != 50 || first.Items[0].ThoughtID != 120 || first.Items[49].ThoughtID != 71 || !first.More {
			t.Fatalf("got first page %+v, want IDs 120 through 71 and more", first)
		}
		if _, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought,observed_at,created_at,updated_at) VALUES(121,'u','new',101,100,100); DELETE FROM thoughts WHERE thought_id=100`); err != nil {
			t.Fatal(err)
		}
		cursor := first.Items[49].Cursor()
		second, err := store.BrowseThoughts(t.Context(), "u", data.ThoughtPageRequest{Cursor: &cursor})
		if err != nil {
			t.Fatal(err)
		}
		if len(second.Items) != 50 || second.Items[0].ThoughtID != 70 || second.Items[49].ThoughtID != 21 || !second.More {
			t.Fatalf("got second page %+v, want IDs 70 through 21", second)
		}
		cursor = second.Items[0].Cursor()
		back, err := store.BrowseThoughts(t.Context(), "u", data.ThoughtPageRequest{Cursor: &cursor, Direction: data.ThoughtsNewer})
		if err != nil {
			t.Fatal(err)
		}
		if len(back.Items) != 50 || back.Items[0].ThoughtID != 121 || back.Items[49].ThoughtID != 71 || back.More {
			t.Fatalf("got reverse page %+v, want remaining 50 newer rows", back)
		}
		cursor = second.Items[49].Cursor()
		last, err := store.BrowseThoughts(t.Context(), "u", data.ThoughtPageRequest{Cursor: &cursor})
		if err != nil {
			t.Fatal(err)
		}
		if len(last.Items) != 20 || last.More {
			t.Fatalf("got %d rows, more %v, want 20 and false", len(last.Items), last.More)
		}
	})
	t.Run("bounds Unicode previews but retrieves the complete original thought", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		body := strings.Repeat("界", 100000)
		store := data.NewSQLiteThoughtStore(db)
		item, err := store.CreateThought(t.Context(), &data.Thought{UserID: "u", Thought: body, Version: 1, ObservedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		page, err := store.BrowseThoughts(t.Context(), "u", data.ThoughtPageRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 || len([]rune(page.Items[0].Preview)) != 81 {
			t.Fatalf("got preview page %+v, want one 81-character preview", page)
		}
		full, err := store.GetThought(t.Context(), "u", item.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		if full.Thought != body {
			t.Fatal("got altered full body, want original")
		}
	})
}
