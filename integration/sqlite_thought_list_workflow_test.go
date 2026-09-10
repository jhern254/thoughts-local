//go:build integration

package integration_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/metrics"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestThoughtListWorkflow_SQLite(t *testing.T) {
	t.Run("lists visible owned thoughts in observed order and counts each subject", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other")
		_, err := db.Exec(`INSERT INTO subjects (subject_id, user_id, subject_name) VALUES (1,'u','one'),(2,'u','empty'),(3,'other','other');
   INSERT INTO thoughts (thought_id,user_id,subject_id,thought,observed_at) VALUES
   (1,'u',1,'older',1),(2,'u',1,'newer',2),(3,'u',1,'tie',2),(4,'other',3,'private',3),(5,'u',NULL,'inbox',3);
   INSERT INTO thoughts (user_id,subject_id,thought,deleted_at) VALUES ('u',1,'deleted',unixepoch());`)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openSQLite(t, dsn)
		ctx := context.Background()
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		rows, err := service.List(ctx, "u", 1)
		if err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, row := range rows {
			ids = append(ids, row.ThoughtID)
			found, err := service.Get(ctx, "u", row.ThoughtID)
			if err != nil {
				t.Fatal(err)
			}
			assertThoughtEqual(t, &row, found)
		}
		if want := []int64{3, 2, 1}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("got IDs %v, want %v", ids, want)
		}
		for _, id := range []int64{2, 3, 99} {
			rows, err := service.List(ctx, "u", id)
			if err != nil || rows == nil || len(rows) != 0 {
				t.Fatalf("got subject %d thoughts %v, error %v; want nonnil empty list and nil error", id, rows, err)
			}
		}
		counts, err := metrics.NewService(data.NewSQLiteMetricsStore(db)).ThoughtCountsBySubject(ctx, "u")
		want := []data.SubjectThoughtCount{{SubjectID: 1, Count: 3}, {SubjectID: 2, Count: 0}}
		if err != nil || !reflect.DeepEqual(counts, want) {
			t.Fatalf("got counts %v, error %v; want %v, nil", counts, err, want)
		}
	})
	for _, deleted := range []string{"subject", "user"} {
		t.Run("excludes deleted "+deleted, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u")
			_, err := db.Exec(`INSERT INTO subjects (subject_id,user_id,subject_name) VALUES (1,'u','one'); INSERT INTO thoughts (user_id,subject_id,thought) VALUES ('u',1,'retained')`)
			if err != nil {
				t.Fatal(err)
			}
			query := "UPDATE subjects SET deleted_at=unixepoch(),updated_at=unixepoch()"
			if deleted == "user" {
				query = "UPDATE users SET deleted_at=unixepoch(),updated_at=unixepoch()"
			}
			if _, err := db.Exec(query); err != nil {
				t.Fatal(err)
			}
			rows, err := data.NewSQLiteThoughtStore(db).ListThoughts(context.Background(), "u", 1)
			if err != nil || rows == nil || len(rows) != 0 {
				t.Fatalf("got thoughts %v, error %v; want nonnil empty list, nil", rows, err)
			}
			counts, err := data.NewSQLiteMetricsStore(db).ThoughtCountsBySubject(context.Background(), "u")
			if err != nil || counts == nil || len(counts) != 0 {
				t.Fatalf("got counts %v, error %v; want nonnil empty list, nil", counts, err)
			}
		})
	}
}
