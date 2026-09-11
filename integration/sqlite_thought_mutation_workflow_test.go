//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestThoughtMutationsWorkflow_SQLite(t *testing.T) {
	t.Run("updates only text and persists the returned revision", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		_, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'owner','subject');
			INSERT INTO events(event_id,user_id) VALUES (1,'owner');
			INSERT INTO thoughts(thought_id,user_id,subject_id,event_id,thought,observed_at,created_at,updated_at) VALUES (1,'owner',1,1,'original',10,20,30);`)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		original, err := service.Get(ctx, "owner", 1)
		if err != nil {
			t.Fatal(err)
		}
		updated, err := service.Update(ctx, "owner", 1, "  PRIVATE-EDIT\n界  ", original.Version)
		if err != nil {
			t.Fatal(err)
		}
		want := *original
		want.Thought = "  PRIVATE-EDIT\n界  "
		want.Version++
		want.UpdatedAt = updated.UpdatedAt
		assertThoughtEqual(t, updated, &want)
		if updated.UpdatedAt.Before(original.UpdatedAt) || updated.UpdatedAt.Location() != time.UTC {
			t.Fatalf("got updated time %v, want nondecreasing UTC time", updated.UpdatedAt)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		reopened := openSQLite(t, dsn)
		got, err := thought.NewService(data.NewSQLiteThoughtStore(reopened)).Get(ctx, "owner", 1)
		if err != nil {
			t.Fatal(err)
		}
		assertThoughtEqual(t, got, updated)
	})
	t.Run("returns the active-link projection without erasing retained event provenance", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		_, err := db.Exec(`INSERT INTO events(event_id,user_id) VALUES (1,'owner');
			INSERT INTO thoughts(thought_id,user_id,event_id,thought) VALUES (1,'owner',1,'original');
			UPDATE events SET deleted_at=updated_at WHERE event_id=1;`)
		if err != nil {
			t.Fatal(err)
		}
		updated, err := thought.NewService(data.NewSQLiteThoughtStore(db)).Update(context.Background(), "owner", 1, "changed", 1)
		if err != nil {
			t.Fatal(err)
		}
		if updated.EventID != nil {
			t.Fatalf("got projected event ID %v, want nil for deleted event", updated.EventID)
		}
		var eventID int64
		if err := db.QueryRow(`SELECT event_id FROM thoughts WHERE thought_id=1`).Scan(&eventID); err != nil {
			t.Fatal(err)
		}
		if eventID != 1 {
			t.Fatalf("got retained event ID %d, want 1", eventID)
		}
	})

	t.Run("identical text still advances the version", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		ctx := context.Background()
		item, err := service.Create(ctx, "owner", "same", nil, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		got, err := service.Update(ctx, "owner", item.ThoughtID, item.Thought, item.Version)
		if err != nil {
			t.Fatal(err)
		}
		if got.Version != item.Version+1 {
			t.Fatalf("got version %d, want %d", got.Version, item.Version+1)
		}
	})
	for _, change := range []string{"another edit", "subject deletion"} {
		t.Run("rejects stale update and delete after "+change, func(t *testing.T) {
			db, dsn := openMigratedSQLite(t)
			insertUsers(t, db, "owner")
			ctx := context.Background()
			subjects := subject.NewService(data.NewSQLiteSubjectStore(db))
			parent, err := subjects.Create(ctx, "owner", "subject")
			if err != nil {
				t.Fatal(err)
			}
			service := thought.NewService(data.NewSQLiteThoughtStore(db))
			original, err := service.Create(ctx, "owner", "original", &parent.SubjectID, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			// A separate connection represents the intervening writer.
			writer := openSQLite(t, dsn)
			if change == "another edit" {
				_, err = thought.NewService(data.NewSQLiteThoughtStore(writer)).Update(ctx, "owner", original.ThoughtID, "new revision", original.Version)
			} else {
				err = data.NewSQLiteSubjectStore(writer).DeleteSubject(ctx, "owner", parent.SubjectID)
			}
			if err != nil {
				t.Fatal(err)
			}
			current, err := service.Get(ctx, "owner", original.ThoughtID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Update(ctx, "owner", original.ThoughtID, "PRIVATE-STALE", original.Version); !errors.Is(err, data.ErrVersionConflict) {
				t.Fatalf("got update error %v, want %v", err, data.ErrVersionConflict)
			}
			if err := service.Delete(ctx, "owner", original.ThoughtID, original.Version); !errors.Is(err, data.ErrVersionConflict) {
				t.Fatalf("got delete error %v, want %v", err, data.ErrVersionConflict)
			}
			got, err := service.Get(ctx, "owner", original.ThoughtID)
			if err != nil {
				t.Fatal(err)
			}
			assertThoughtEqual(t, got, current)
			if change == "subject deletion" && got.SubjectID != nil {
				t.Fatalf("got subject %v, want nil", got.SubjectID)
			}
		})
	}
	for _, scope := range []string{"assigned", "unassigned"} {
		t.Run("soft deletes "+scope+" thought while retaining content and history", func(t *testing.T) {
			db, dsn := openMigratedSQLite(t)
			insertUsers(t, db, "owner")
			_, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'owner','subject');
				INSERT INTO events(event_id,user_id) VALUES (1,'owner');
				INSERT INTO tags(tag_id,user_id,tag_name) VALUES (1,'owner','tag');
				INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES (1,'owner','goal',60);`)
			if err != nil {
				t.Fatal(err)
			}
			var subjectID *int64
			if scope == "assigned" {
				id := int64(1)
				subjectID = &id
			}
			_, err = db.Exec(`INSERT INTO thoughts(thought_id,user_id,subject_id,event_id,thought,observed_at,created_at,updated_at) VALUES (1,'owner',?,1,'PRIVATE-RETAINED',10,20,30);
				INSERT INTO thought_tags(thought_id,tag_id,created_at) VALUES (1,1,40);
				INSERT INTO thought_mindset_periods(thought_id,started_at,ended_at) VALUES (1,50,60),(1,70,NULL);
				INSERT INTO goal_progress(goal_id,user_id,time_spent_sec,thought_id,event_id,progress_note) VALUES (1,'owner',60,1,1,'provenance');`, subjectID)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			service := thought.NewService(data.NewSQLiteThoughtStore(db))
			if err := service.Delete(ctx, "owner", 1, 1); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db = openSQLite(t, dsn)
			service = thought.NewService(data.NewSQLiteThoughtStore(db))
			if _, err := service.Get(ctx, "owner", 1); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got get error %v, want %v", err, data.ErrRecordNotFound)
			}
			if _, err := service.Update(ctx, "owner", 1, "restore attempt", 2); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got update error %v, want %v", err, data.ErrRecordNotFound)
			}
			if err := service.Delete(ctx, "owner", 1, 2); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got repeat delete error %v, want %v", err, data.ErrRecordNotFound)
			}
			assigned, err := service.List(ctx, "owner", 1)
			if err != nil {
				t.Fatal(err)
			}
			unassigned, err := service.ListUnassigned(ctx, "owner")
			if err != nil {
				t.Fatal(err)
			}
			if len(assigned) != 0 || len(unassigned) != 0 {
				t.Fatalf("got list lengths %d/%d, want 0/0", len(assigned), len(unassigned))
			}
			metrics := data.NewSQLiteMetricsStore(db)
			count, err := metrics.CountUnassignedThoughts(ctx, "owner")
			if err != nil {
				t.Fatal(err)
			}
			counts, err := metrics.ThoughtCountsBySubject(ctx, "owner")
			if err != nil {
				t.Fatal(err)
			}
			if count != 0 || len(counts) != 1 || counts[0].Count != 0 {
				t.Fatalf("got metrics %d/%v, want unassigned 0 and subject count 0", count, counts)
			}
			var preserved bool
			err = db.QueryRow(`SELECT thought='PRIVATE-RETAINED' AND user_id='owner' AND subject_id IS ? AND event_id=1
				AND observed_at=10 AND created_at=20 AND version=2 AND deleted_at=updated_at AND deleted_at>=30 FROM thoughts WHERE thought_id=1`, subjectID).Scan(&preserved)
			if err != nil {
				t.Fatal(err)
			}
			if !preserved {
				t.Fatal("got changed or missing retained thought fields, want original data with deletion timestamp and version 2")
			}
			for _, query := range []string{
				`SELECT COUNT(*) FROM thought_tags WHERE thought_id=1 AND tag_id=1 AND created_at=40`,
				`SELECT COUNT(*) FROM thought_mindset_periods WHERE thought_id=1 AND started_at=50 AND ended_at=60`,
				`SELECT COUNT(*) FROM thought_mindset_periods WHERE thought_id=1 AND started_at=70 AND ended_at IS NULL`,
				`SELECT COUNT(*) FROM goal_progress WHERE thought_id=1 AND event_id=1 AND time_spent_sec=60 AND progress_note='provenance'`,
			} {
				var got int
				if err := db.QueryRow(query).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if got != 1 {
					t.Fatalf("got retained row count %d for %s, want 1", got, query)
				}
			}
		})
	}
	for _, state := range []string{"missing", "foreign owner", "deleted owner"} {
		t.Run("returns not found for "+state+" without changing data", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "owner", "other")
			service := thought.NewService(data.NewSQLiteThoughtStore(db))
			ctx := context.Background()
			item, err := service.Create(ctx, "owner", "original", nil, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			user, id := "owner", item.ThoughtID
			switch state {
			case "missing":
				id++
			case "foreign owner":
				user = "other"
			case "deleted owner":
				if _, err := db.Exec(`UPDATE users SET deleted_at=updated_at WHERE user_id='owner'`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.Update(ctx, user, id, "PRIVATE-REJECTED", 99); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got update error %v, want %v", err, data.ErrRecordNotFound)
			}
			if err := service.Delete(ctx, user, id, 99); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got delete error %v, want %v", err, data.ErrRecordNotFound)
			}
			var intact bool
			if err := db.QueryRow(`SELECT thought='original' AND version=1 AND deleted_at IS NULL FROM thoughts WHERE thought_id=?`, item.ThoughtID).Scan(&intact); err != nil {
				t.Fatal(err)
			}
			if !intact {
				t.Fatal("got changed thought, want unchanged text, version, and deletion state")
			}
		})
	}
	t.Run("mutation timestamps do not move backward", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		future := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
		_, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought,created_at,updated_at) VALUES (1,'owner','original',?,?)`, future.Unix(), future.Unix())
		if err != nil {
			t.Fatal(err)
		}
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		ctx := context.Background()
		updated, err := service.Update(ctx, "owner", 1, "changed", 1)
		if err != nil {
			t.Fatal(err)
		}
		if !updated.UpdatedAt.Equal(future) {
			t.Fatalf("got updated time %v, want %v", updated.UpdatedAt, future)
		}
		if err := service.Delete(ctx, "owner", 1, updated.Version); err != nil {
			t.Fatal(err)
		}
		var deleted, updatedSec int64
		if err := db.QueryRow(`SELECT deleted_at,updated_at FROM thoughts WHERE thought_id=1`).Scan(&deleted, &updatedSec); err != nil {
			t.Fatal(err)
		}
		if deleted != future.Unix() || updatedSec != future.Unix() {
			t.Fatalf("got deletion/update %d/%d, want %d/%d", deleted, updatedSec, future.Unix(), future.Unix())
		}
	})
	for _, failure := range []string{"canceled", "busy", "read only"} {
		t.Run("preserves "+failure+" mutation errors", func(t *testing.T) {
			db, dsn := openMigratedSQLite(t)
			insertUsers(t, db, "owner")
			_, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'owner','original')`)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			store := data.NewSQLiteThoughtStore(db)
			switch failure {
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "busy":
				writer := openSQLite(t, dsn)
				if _, err := writer.Exec("BEGIN IMMEDIATE"); err != nil {
					t.Fatal(err)
				}
				defer writer.Exec("ROLLBACK")
			case "read only":
				store = data.NewSQLiteThoughtStore(openSQLite(t, dsn+"?mode=ro"))
			}
			_, updateErr := store.UpdateThought(ctx, "owner", 1, "PRIVATE-FAILED", 1, time.Now().UTC())
			deleteErr := store.DeleteThought(ctx, "owner", 1, 1)
			for _, err := range []error{updateErr, deleteErr} {
				switch failure {
				case "canceled":
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("got error %v, want cancellation", err)
					}
				case "busy":
					assertDatabaseFailure(t, err, data.ErrDatabaseBusy, sqlite3.SQLITE_BUSY)
				case "read only":
					assertDatabaseFailure(t, err, data.ErrDatabaseReadOnly, sqlite3.SQLITE_READONLY)
				}
			}
			var intact bool
			if err := db.QueryRow(`SELECT thought='original' AND version=1 AND deleted_at IS NULL FROM thoughts WHERE thought_id=1`).Scan(&intact); err != nil {
				t.Fatal(err)
			}
			if !intact {
				t.Fatal("got changed thought after failure, want unchanged text, version, and deletion state")
			}
		})
	}
}
