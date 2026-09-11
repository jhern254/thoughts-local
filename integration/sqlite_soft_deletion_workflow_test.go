//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/user"
)

func TestSoftDeletionWorkflow_SQLite(t *testing.T) {
	t.Run("retains deleted subject and persists thought unlinking after reopening", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		ctx := context.Background()
		subjects := subject.NewService(data.NewSQLiteSubjectStore(db))
		item, err := subjects.Create(ctx, "u", "name")
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		thoughts := thought.NewService(data.NewSQLiteThoughtStore(db))
		linked, err := thoughts.Create(ctx, "u", "keep", &item.SubjectID, time.Time{})
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if err := subjects.Delete(ctx, "u", item.SubjectID); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if err := subjects.Delete(ctx, "u", item.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got repeated delete error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := subjects.Update(ctx, "u", item.SubjectID, "changed"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got update deleted subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		replacement, err := subjects.Create(ctx, "u", "name")
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if replacement.SubjectID == item.SubjectID {
			t.Fatalf("got replacement subject ID %d, want an ID different from retained ID %d", replacement.SubjectID, item.SubjectID)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		reopened := openSQLite(t, dsn)
		var storedID sql.NullInt64
		var deletedAt, updatedAt int64
		if err := reopened.QueryRow(`SELECT t.subject_id,s.deleted_at,s.updated_at FROM thoughts t CROSS JOIN subjects s WHERE t.thought_id=? AND s.subject_id=?`, linked.ThoughtID, item.SubjectID).Scan(&storedID, &deletedAt, &updatedAt); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if storedID.Valid {
			t.Fatalf("got stored subject ID %d, want NULL", storedID.Int64)
		}
		if got, want := deletedAt, updatedAt; got != want {
			t.Fatalf("got deletion timestamp %d, want updated timestamp %d", got, want)
		}
		if got, want := deletedAt, item.CreatedAt.Unix(); got < want {
			t.Fatalf("got deletion timestamp %d, want at least created timestamp %d", got, want)
		}
		got, err := data.NewSQLiteThoughtStore(reopened).GetThought(ctx, "u", linked.ThoughtID)
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if got.SubjectID != nil {
			t.Fatalf("got subject ID %d, want nil", *got.SubjectID)
		}
		listed, err := data.NewSQLiteSubjectStore(reopened).ListSubjects(ctx, "u")
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if got, want := len(listed), 1; got != want {
			t.Fatalf("got listed subject count %d, want %d", got, want)
		}
		if got, want := listed[0].SubjectID, replacement.SubjectID; got != want {
			t.Fatalf("got listed subject ID %d, want %d", got, want)
		}
	})
	for _, table := range []string{"users", "subjects", "events"} {
		t.Run("rejects new thoughts referencing deleted "+table, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u")
			if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','name'); INSERT INTO events(event_id,user_id) VALUES (1,'u');`); err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			if _, err := db.Exec("UPDATE " + table + " SET deleted_at=updated_at"); err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
			id := int64(1)
			_, err := data.NewSQLiteThoughtStore(db).CreateThought(context.Background(), &data.Thought{UserID: "u", SubjectID: &id, EventID: &id, Thought: "rejected", Version: 1})
			if !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got create thought error %v, want %v", err, data.ErrRecordNotFound)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM thoughts").Scan(&count); err != nil {
				t.Fatalf("got query error %v, want nil", err)
			}
			if got, want := count, 0; got != want {
				t.Fatalf("got row count %d, want %d", got, want)
			}
		})
	}
	t.Run("hides deleted event without losing provenance", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`INSERT INTO events(event_id,user_id) VALUES (1,'u'); INSERT INTO thoughts(thought_id,user_id,event_id,thought) VALUES (1,'u',1,'keep'); UPDATE events SET deleted_at=updated_at;`); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		got, err := data.NewSQLiteThoughtStore(db).GetThought(context.Background(), "u", 1)
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if got.EventID != nil {
			t.Fatalf("got event ID %d, want nil", *got.EventID)
		}
		var id int64
		if err := db.QueryRow("SELECT event_id FROM thoughts WHERE thought_id=1").Scan(&id); err != nil {
			t.Fatalf("got query error %v, want nil", err)
		}
		if got, want := id, int64(1); got != want {
			t.Fatalf("got stored event ID %d, want %d", got, want)
		}
	})
	t.Run("hides provenance owned by a deleted user and rejects new references", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "event-owner")
		if _, err := db.Exec(`INSERT INTO events(event_id,user_id) VALUES (1,'event-owner');
			INSERT INTO thoughts(thought_id,user_id,event_id,thought) VALUES (1,'u',1,'keep');
			UPDATE users SET deleted_at=updated_at WHERE user_id='event-owner';`); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		store := data.NewSQLiteThoughtStore(db)
		got, err := store.GetThought(context.Background(), "u", 1)
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if got.EventID != nil {
			t.Fatalf("got event ID %d, want nil for deleted owner", *got.EventID)
		}
		id := int64(1)
		if _, err := store.CreateThought(context.Background(), &data.Thought{UserID: "u", EventID: &id, Thought: "rejected", Version: 1}); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got create thought error %v, want %v", err, data.ErrRecordNotFound)
		}
	})
	t.Run("hides deleted thought", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'u','keep'); UPDATE thoughts SET deleted_at=updated_at;`); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if _, err := data.NewSQLiteThoughtStore(db).GetThought(context.Background(), "u", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got get thought error %v, want %v", err, data.ErrRecordNotFound)
		}
	})
	t.Run("hides deleted owners content and rejects subject mutations", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','name'); INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'u','keep'); UPDATE users SET deleted_at=updated_at;`); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		ctx := context.Background()
		store := data.NewSQLiteSubjectStore(db)
		if _, err := store.GetSubject(ctx, "u", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got get subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		listed, err := store.ListSubjects(ctx, "u")
		if err != nil {
			t.Fatalf("got list subjects error %v, want nil", err)
		}
		if got, want := len(listed), 0; got != want {
			t.Fatalf("got listed subject count %d, want %d", got, want)
		}
		if _, err := store.CreateSubject(ctx, &data.Subject{UserID: "u", SubjectName: "new"}); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got create subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := store.UpdateSubject(ctx, "u", 1, "changed", time.Now()); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got update subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		if err := store.DeleteSubject(ctx, "u", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got delete subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := data.NewSQLiteThoughtStore(db).GetThought(ctx, "u", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got get thought error %v, want %v", err, data.ErrRecordNotFound)
		}
		var name string
		if err := db.QueryRow("SELECT subject_name FROM subjects WHERE subject_id=1 AND deleted_at IS NULL").Scan(&name); err != nil {
			t.Fatalf("got query error %v, want nil", err)
		}
		if got, want := name, "name"; got != want {
			t.Fatalf("got subject name %q, want %q", got, want)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM subjects").Scan(&count); err != nil {
			t.Fatalf("got query error %v, want nil", err)
		}
		if got, want := count, 1; got != want {
			t.Fatalf("got row count %d, want %d", got, want)
		}
	})
	t.Run("does not revive or replace deleted local user", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		ctx := context.Background()
		store := data.NewSQLiteUserStore(db)
		service := user.NewService(store)
		if _, err := service.EnsureLocalUser(ctx); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if _, err := db.Exec("UPDATE users SET deleted_at=updated_at"); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if _, err := store.GetUserByHandle(ctx, "local"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got get deleted user error %v, want %v", err, data.ErrRecordNotFound)
		}
		if _, err := service.EnsureLocalUser(ctx); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("got bootstrap error %v, want %v", err, data.ErrDuplicateRecord)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM users WHERE deleted_at IS NOT NULL").Scan(&count); err != nil {
			t.Fatalf("got query error %v, want nil", err)
		}
		if got, want := count, 1; got != want {
			t.Fatalf("got row count %d, want %d", got, want)
		}
	})
}
