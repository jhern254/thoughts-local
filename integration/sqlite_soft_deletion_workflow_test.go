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
	"github.com/jhern254/go-thoughts/internal/user"
)

func TestSoftDeletionWorkflow_SQLite(t *testing.T) {
	t.Run("retains deleted subject and links after reopening", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		ctx := context.Background()
		subjects := subject.NewService(data.NewSQLiteSubjectStore(db))
		item, err := subjects.Create(ctx, "u", "name")
		if err != nil {
			t.Fatal(err)
		}
		thoughts := thought.NewService(data.NewSQLiteThoughtStore(db))
		linked, err := thoughts.Create(ctx, "u", "keep", &item.SubjectID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if err := subjects.Delete(ctx, "u", item.SubjectID); err != nil {
			t.Fatal(err)
		}
		if err := subjects.Delete(ctx, "u", item.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("repeated delete: %v", err)
		}
		if _, err := subjects.Update(ctx, "u", item.SubjectID, "changed"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("update deleted: %v", err)
		}
		replacement, err := subjects.Create(ctx, "u", "name")
		if err != nil {
			t.Fatal(err)
		}
		if replacement.SubjectID == item.SubjectID {
			t.Fatal("reused retained ID")
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		reopened := openSQLite(t, dsn)
		var storedID, deletedAt, updatedAt int64
		if err := reopened.QueryRow(`SELECT t.subject_id,s.deleted_at,s.updated_at FROM thoughts t JOIN subjects s ON s.subject_id=t.subject_id WHERE t.thought_id=?`, linked.ThoughtID).Scan(&storedID, &deletedAt, &updatedAt); err != nil {
			t.Fatal(err)
		}
		if storedID != item.SubjectID || deletedAt != updatedAt || deletedAt < item.CreatedAt.Unix() {
			t.Fatalf("retained subject: %d %d %d", storedID, deletedAt, updatedAt)
		}
		got, err := data.NewSQLiteThoughtStore(reopened).GetThought(ctx, "u", linked.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		if got.SubjectID != nil {
			t.Fatal("exposed deleted subject")
		}
		listed, err := data.NewSQLiteSubjectStore(reopened).ListSubjects(ctx, "u")
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 1 || listed[0].SubjectID != replacement.SubjectID {
			t.Fatalf("listed: %#v", listed)
		}
	})
	for _, table := range []string{"users", "subjects", "events"} {
		t.Run("rejects new thoughts referencing deleted "+table, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u")
			if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','name'); INSERT INTO events(event_id,user_id) VALUES (1,'u');`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("UPDATE " + table + " SET deleted_at=updated_at"); err != nil {
				t.Fatal(err)
			}
			id := int64(1)
			_, err := data.NewSQLiteThoughtStore(db).CreateThought(context.Background(), &data.Thought{UserID: "u", SubjectID: &id, EventID: &id, Thought: "rejected", Version: 1})
			if !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("create error: %v", err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM thoughts").Scan(&count); err != nil || count != 0 {
				t.Fatalf("persisted rejected thought: %d %v", count, err)
			}
		})
	}
	t.Run("hides deleted event without losing provenance", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`INSERT INTO events(event_id,user_id) VALUES (1,'u'); INSERT INTO thoughts(thought_id,user_id,event_id,thought) VALUES (1,'u',1,'keep'); UPDATE events SET deleted_at=updated_at;`); err != nil {
			t.Fatal(err)
		}
		got, err := data.NewSQLiteThoughtStore(db).GetThought(context.Background(), "u", 1)
		if err != nil {
			t.Fatal(err)
		}
		if got.EventID != nil {
			t.Fatal("exposed deleted event")
		}
		var id int64
		if err := db.QueryRow("SELECT event_id FROM thoughts WHERE thought_id=1").Scan(&id); err != nil || id != 1 {
			t.Fatalf("lost event: %d %v", id, err)
		}
	})
	t.Run("hides provenance owned by a deleted user and rejects new references", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "event-owner")
		if _, err := db.Exec(`INSERT INTO events(event_id,user_id) VALUES (1,'event-owner');
			INSERT INTO thoughts(thought_id,user_id,event_id,thought) VALUES (1,'u',1,'keep');
			UPDATE users SET deleted_at=updated_at WHERE user_id='event-owner';`); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteThoughtStore(db)
		got, err := store.GetThought(context.Background(), "u", 1)
		if err != nil {
			t.Fatal(err)
		}
		if got.EventID != nil {
			t.Fatal("exposed deleted owner's event")
		}
		id := int64(1)
		if _, err := store.CreateThought(context.Background(), &data.Thought{UserID: "u", EventID: &id, Thought: "rejected", Version: 1}); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("create error: %v", err)
		}
	})
	t.Run("hides deleted thought", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'u','keep'); UPDATE thoughts SET deleted_at=updated_at;`); err != nil {
			t.Fatal(err)
		}
		if _, err := data.NewSQLiteThoughtStore(db).GetThought(context.Background(), "u", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("get error: %v", err)
		}
	})
	t.Run("hides deleted owners content and rejects subject mutations", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','name'); INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'u','keep'); UPDATE users SET deleted_at=updated_at;`); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		store := data.NewSQLiteSubjectStore(db)
		if _, err := store.GetSubject(ctx, "u", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("get subject: %v", err)
		}
		listed, err := store.ListSubjects(ctx, "u")
		if err != nil || len(listed) != 0 {
			t.Fatalf("list subjects: %#v %v", listed, err)
		}
		if _, err := store.CreateSubject(ctx, &data.Subject{UserID: "u", SubjectName: "new"}); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("create subject: %v", err)
		}
		if _, err := store.UpdateSubject(ctx, "u", 1, "changed", time.Now()); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("update subject: %v", err)
		}
		if err := store.DeleteSubject(ctx, "u", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("delete subject: %v", err)
		}
		if _, err := data.NewSQLiteThoughtStore(db).GetThought(ctx, "u", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("get thought: %v", err)
		}
		var name string
		if err := db.QueryRow("SELECT subject_name FROM subjects WHERE subject_id=1 AND deleted_at IS NULL").Scan(&name); err != nil || name != "name" {
			t.Fatalf("changed hidden subject: %q %v", name, err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM subjects").Scan(&count); err != nil || count != 1 {
			t.Fatalf("created hidden subject: %d %v", count, err)
		}
	})
	t.Run("does not revive or replace deleted local user", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		ctx := context.Background()
		store := data.NewSQLiteUserStore(db)
		service := user.NewService(store)
		if _, err := service.EnsureLocalUser(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("UPDATE users SET deleted_at=updated_at"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.GetUserByHandle(ctx, "local"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("get deleted user: %v", err)
		}
		if _, err := service.EnsureLocalUser(ctx); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("bootstrap deleted user: %v", err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM users WHERE deleted_at IS NOT NULL").Scan(&count); err != nil || count != 1 {
			t.Fatalf("changed deleted user: %d %v", count, err)
		}
	})
}
