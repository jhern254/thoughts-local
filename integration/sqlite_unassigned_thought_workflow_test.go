//go:build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/metrics"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestUnassignedThoughtWorkflow_SQLite(t *testing.T) {
	t.Run("lists and counts only active unassigned thoughts for the active owner", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other", "deleted")
		if _, err := db.Exec(`
			UPDATE users SET deleted_at=updated_at WHERE user_id='deleted';
			INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','assigned');
			INSERT INTO thoughts(thought_id,user_id,subject_id,thought,observed_at,deleted_at) VALUES
			 (1,'u',NULL,'older',1,NULL),(2,'u',NULL,'tie',2,NULL),(3,'u',NULL,'newest',2,NULL),
			 (4,'u',1,'assigned',3,NULL),(5,'other',NULL,'other',3,NULL),
			 (6,'deleted',NULL,'deleted owner',3,NULL),(7,'u',NULL,'deleted thought',3,unixepoch());`); err != nil {
			t.Fatal(err)
		}
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		counts := metrics.NewService(data.NewSQLiteMetricsStore(db))
		for _, tt := range []struct {
			user string
			ids  []int64
		}{{"u", []int64{3, 2, 1}}, {"deleted", nil}, {"missing", nil}} {
			rows, err := service.ListUnassigned(context.Background(), tt.user)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != len(tt.ids) {
				t.Fatalf("got %d rows for %s, want %d", len(rows), tt.user, len(tt.ids))
			}
			for i, row := range rows {
				if row.ThoughtID != tt.ids[i] || row.SubjectID != nil {
					t.Fatalf("got row %+v, want unassigned ID %d", row, tt.ids[i])
				}
			}
			count, err := counts.CountUnassignedThoughts(context.Background(), tt.user)
			if err != nil {
				t.Fatal(err)
			}
			if count != int64(len(tt.ids)) {
				t.Fatalf("got count %d, want %d", count, len(tt.ids))
			}
		}
	})
	t.Run("subject deletion makes retained thoughts available as unassigned", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','remove'); INSERT INTO thoughts(user_id,subject_id,thought) VALUES ('u',1,'keep');`); err != nil {
			t.Fatal(err)
		}
		if err := data.NewSQLiteSubjectStore(db).DeleteSubject(context.Background(), "u", 1); err != nil {
			t.Fatal(err)
		}
		rows, err := data.NewSQLiteThoughtStore(db).ListUnassignedThoughts(context.Background(), "u")
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].SubjectID != nil || rows[0].Thought != "keep" {
			t.Fatalf("got %v, want retained unassigned thought", rows)
		}
		count, err := data.NewSQLiteMetricsStore(db).CountUnassignedThoughts(context.Background(), "u")
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("got count %d, want 1", count)
		}
	})
}
