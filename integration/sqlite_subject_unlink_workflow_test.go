//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestSubjectUnlinkWorkflow_SQLite(t *testing.T) {
	t.Run("unlinks all assigned thoughts without changing content or history", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other")
		_, err := db.Exec(`
			INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','remove'),(2,'u','keep'),(3,'other','keep');
			INSERT INTO events(event_id,user_id) VALUES (1,'u');
			INSERT INTO thoughts(thought_id,user_id,subject_id,event_id,thought,version,created_at,updated_at,observed_at,deleted_at) VALUES
			 (1,'u',1,1,'keep',4,1,2,3,NULL),
			 (2,'u',1,1,'keep',4,1,4102444800,3,2),
			 (3,'u',2,1,'keep',4,1,2,3,NULL),
			 (4,'u',NULL,1,'keep',4,1,2,3,NULL),
			 (5,'other',3,1,'keep',4,1,2,3,NULL);
			INSERT INTO tags(tag_id,user_id,tag_name) VALUES (1,'u','tag');
			INSERT INTO thought_tags(thought_id,tag_id) VALUES (1,1),(2,1);
			INSERT INTO thought_mindset_periods(thought_id,started_at) VALUES (1,1),(2,1);
			INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES (1,'u','goal',60);
			INSERT INTO goal_progress(goal_id,user_id,time_spent_sec,thought_id,event_id) VALUES (1,'u',60,1,1),(1,'u',30,2,1);`)
		if err != nil {
			t.Fatal(err)
		}
		before := time.Now().Unix()
		store := data.NewSQLiteSubjectStore(db)
		if err := store.DeleteSubject(context.Background(), "u", 1); err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteSubject(context.Background(), "u", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got repeated deletion %v, want not found", err)
		}
		for id := int64(1); id <= 5; id++ {
			var subjectID, deletedAt sql.NullInt64
			var owner, body string
			var version, created, updated, observed, eventID int64
			if err := db.QueryRow(`SELECT subject_id,user_id,thought,version,created_at,updated_at,observed_at,deleted_at,event_id FROM thoughts WHERE thought_id=?`, id).
				Scan(&subjectID, &owner, &body, &version, &created, &updated, &observed, &deletedAt, &eventID); err != nil {
				t.Fatal(err)
			}
			wantSubject := map[int64]sql.NullInt64{3: {Int64: 2, Valid: true}, 5: {Int64: 3, Valid: true}}[id]
			wantOwner, wantVersion, wantUpdated := "u", int64(4), int64(2)
			if id == 5 {
				wantOwner = "other"
			}
			if id <= 2 {
				wantVersion = 5
				wantUpdated = updated
				if updated < before {
					t.Fatalf("thought %d: got updated_at %d, want >= %d", id, updated, before)
				}
			}
			if id == 2 {
				wantUpdated = 4102444800 // A future timestamp must not move backward.
			}
			if subjectID != wantSubject || owner != wantOwner || version != wantVersion || updated != wantUpdated {
				t.Fatalf("thought %d: got subject/owner/version/updated %v/%s/%d/%d, want %v/%s/%d/%d", id, subjectID, owner, version, updated, wantSubject, wantOwner, wantVersion, wantUpdated)
			}
			wantDeleted := sql.NullInt64{Int64: 2, Valid: true}
			if id != 2 {
				wantDeleted = sql.NullInt64{}
			}
			if body != "keep" || created != 1 || observed != 3 || eventID != 1 || deletedAt != wantDeleted {
				t.Fatalf("thought %d: got content/timestamps/event/deletion %q/%d/%d/%d/%v, want keep/1/3/1/%v", id, body, created, observed, eventID, deletedAt, wantDeleted)
			}
		}
		for _, check := range []struct {
			query string
			want  int
		}{
			{"SELECT count(*) FROM subjects WHERE subject_id=1 AND deleted_at IS NOT NULL", 1},
			{"SELECT count(*) FROM subjects WHERE subject_id IN (2,3) AND deleted_at IS NULL", 2},
			{"SELECT count(*) FROM thought_tags WHERE thought_id IN (1,2) AND tag_id=1", 2},
			{"SELECT count(*) FROM thought_mindset_periods WHERE thought_id IN (1,2) AND started_at=1 AND ended_at IS NULL", 2},
			{"SELECT sum(time_spent_sec) FROM goal_progress WHERE goal_id=1 AND thought_id IN (1,2) AND event_id=1", 90},
		} {
			var got int
			if err := db.QueryRow(check.query).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != check.want {
				t.Fatalf("%s: got %d, want %d", check.query, got, check.want)
			}
		}
	})
	for _, scenario := range []string{"missing subject", "another owner", "deleted owner", "unlink failure"} {
		t.Run(scenario+" leaves subject and thoughts unchanged", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u", "other")
			if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','keep'); INSERT INTO thoughts(user_id,subject_id,thought) VALUES ('u',1,'keep');`); err != nil {
				t.Fatal(err)
			}
			owner, id := "u", int64(1)
			switch scenario {
			case "missing subject":
				id = 99
			case "another owner":
				owner = "other"
			case "deleted owner":
				if _, err := db.Exec("UPDATE users SET deleted_at=updated_at WHERE user_id='u'"); err != nil {
					t.Fatal(err)
				}
			case "unlink failure":
				if _, err := db.Exec(`CREATE TRIGGER reject_unlink BEFORE UPDATE OF subject_id ON thoughts BEGIN SELECT RAISE(ABORT,'test unlink failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			err := data.NewSQLiteSubjectStore(db).DeleteSubject(context.Background(), owner, id)
			if scenario == "unlink failure" {
				var cause interface{ Code() int }
				if !errors.As(err, &cause) {
					t.Fatalf("got failure %v, want wrapped driver cause", err)
				}
			} else if !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got failure %v, want not found", err)
			}
			var unchanged int
			if err := db.QueryRow(`SELECT count(*) FROM subjects s JOIN thoughts t ON t.subject_id=s.subject_id WHERE s.subject_id=1 AND s.deleted_at IS NULL AND s.created_at=s.updated_at AND t.version=1 AND t.created_at=t.updated_at`).Scan(&unchanged); err != nil {
				t.Fatal(err)
			}
			if unchanged != 1 {
				t.Fatalf("got unchanged subject/thought pairs %d, want 1", unchanged)
			}
		})
	}
}
