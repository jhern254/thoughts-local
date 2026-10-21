//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
)

func TestSubjectFromAliasWorkflow_SQLite(t *testing.T) {
	t.Run("creates an empty independent Subject and retains the original relationships", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		if _, err := db.Exec(`INSERT INTO events(event_id,user_id,subject_id,started_at,ended_at) VALUES(1,'owner',1,100,200);
INSERT INTO thoughts(thought_id,user_id,subject_id,event_id,thought) VALUES(1,'owner',1,1,'Retain this thought');
INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES(1,'owner','Practice',60);
INSERT INTO subject_goals(subject_id,goal_id,user_id) VALUES(1,1,'owner');
INSERT INTO goal_progress(goal_id,user_id,time_spent_sec,event_id,thought_id) VALUES(1,'owner',60,1,1)`); err != nil {
			t.Fatal(err)
		}
		original, err := service.Get(t.Context(), "owner", 1)
		if err != nil {
			t.Fatal(err)
		}
		alias, err := service.CreateAlias(t.Context(), "owner", 1, "Golang")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateAlias(t.Context(), "owner", 1, "Go development"); err != nil {
			t.Fatal(err)
		}
		_, err = service.Create(t.Context(), "owner", "Golang")
		var conflict *data.SubjectAliasConflictError
		if !errors.As(err, &conflict) || conflict.AliasID != alias.AliasID {
			t.Fatalf("conflict: got %+v, %v; want exact alias identity", conflict, err)
		}
		created, err := service.CreateFromAlias(t.Context(), "owner", 1, alias.AliasID, "Golang")
		if err != nil {
			t.Fatal(err)
		}
		if created.SubjectID == 1 || created.SubjectName != "Golang" || created.UserID != "owner" {
			t.Fatalf("new Subject: got %+v, want independent Golang", created)
		}
		retained, err := service.Get(t.Context(), "owner", 1)
		if err != nil || !reflect.DeepEqual(retained, original) {
			t.Fatalf("original Subject: got %+v,%v, want %+v", retained, err, original)
		}
		aliases, err := service.ListAliases(t.Context(), "owner", 1)
		if err != nil || len(aliases) != 1 || aliases[0].AliasName != "Go development" {
			t.Fatalf("unrelated aliases: got %+v,%v, want Go development", aliases, err)
		}
		for _, query := range []string{
			`SELECT COUNT(*) FROM thoughts WHERE subject_id=1 AND thought_id=1 AND event_id=1 AND thought='Retain this thought' AND version=1 AND created_at=updated_at`,
			`SELECT COUNT(*) FROM events WHERE subject_id=1 AND event_id=1 AND started_at=100 AND ended_at=200 AND version=1 AND created_at=updated_at`,
			`SELECT COUNT(*) FROM subject_goals WHERE subject_id=1 AND goal_id=1`,
			`SELECT COUNT(*) FROM goal_progress WHERE goal_id=1 AND event_id=1 AND thought_id=1 AND time_spent_sec=60`,
			`SELECT COUNT(*) FROM subject_aliases WHERE alias_id=1 AND deleted_at IS NOT NULL`,
		} {
			var count int
			if err := db.QueryRow(query).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("retained relationship (%s): got %d, want 1", query, count)
			}
		}
		var linked int
		if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM thoughts WHERE subject_id=?)+(SELECT COUNT(*) FROM events WHERE subject_id=?)+(SELECT COUNT(*) FROM subject_goals WHERE subject_id=?)`, created.SubjectID, created.SubjectID, created.SubjectID).Scan(&linked); err != nil {
			t.Fatal(err)
		}
		if linked != 0 {
			t.Fatalf("new Subject relationships: got %d, want 0", linked)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		reopened := subject.NewService(data.NewSQLiteSubjectStore(openSQLite(t, dsn)))
		resolved, err := reopened.ResolveName(t.Context(), "owner", "Golang")
		if err != nil || resolved.SubjectID != created.SubjectID {
			t.Fatalf("persisted identity: got %+v,%v, want %d", resolved, err, created.SubjectID)
		}
		if _, err := reopened.CreateFromAlias(t.Context(), "owner", 1, alias.AliasID, "Golang"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("repeated confirmation: got %v, want not found", err)
		}
	})
	t.Run("retains exact case-sensitive identity", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		original, err := service.Create(t.Context(), "owner", "coding")
		if err != nil {
			t.Fatal(err)
		}
		alias, err := service.CreateAlias(t.Context(), "owner", original.SubjectID, "coDing")
		if err != nil {
			t.Fatal(err)
		}
		created, err := service.CreateFromAlias(t.Context(), "owner", original.SubjectID, alias.AliasID, "coDing")
		if err != nil || created.SubjectName != "coDing" || created.SubjectID == original.SubjectID {
			t.Fatalf("case-sensitive creation: got %+v,%v, want independent coDing", created, err)
		}
		resolved, err := service.ResolveName(t.Context(), "owner", "coding")
		if err != nil || resolved.SubjectID != original.SubjectID {
			t.Fatalf("original case: got %+v,%v, want coding", resolved, err)
		}
	})
	for _, scenario := range []struct {
		name, sql, user, nameInput string
		subjectID, aliasID         int64
	}{
		{name: "wrong owner", user: "other", nameInput: "Golang", subjectID: 1, aliasID: 1},
		{name: "wrong Subject", user: "owner", nameInput: "Golang", subjectID: 2, aliasID: 1},
		{name: "wrong alias", user: "owner", nameInput: "Golang", subjectID: 1, aliasID: 99},
		{name: "different case", user: "owner", nameInput: "golang", subjectID: 1, aliasID: 1},
		{name: "renamed alias", sql: `UPDATE subject_aliases SET alias_name='Changed' WHERE alias_id=1`, user: "owner", nameInput: "Golang", subjectID: 1, aliasID: 1},
		{name: "removed and recreated alias", sql: `UPDATE subject_aliases SET deleted_at=updated_at WHERE alias_id=1; INSERT INTO subject_aliases(subject_id,user_id,alias_name) VALUES(1,'owner','Golang')`, user: "owner", nameInput: "Golang", subjectID: 1, aliasID: 1},
		{name: "deleted Subject", sql: `UPDATE subjects SET deleted_at=updated_at WHERE subject_id=1`, user: "owner", nameInput: "Golang", subjectID: 1, aliasID: 1},
		{name: "deleted owner", sql: `UPDATE users SET deleted_at=updated_at WHERE user_id='owner'`, user: "owner", nameInput: "Golang", subjectID: 1, aliasID: 1},
	} {
		t.Run("rejects "+scenario.name+" without changing aliases", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			service := seedAliasSubjects(t, db)
			if _, err := service.CreateAlias(t.Context(), "owner", 1, "Golang"); err != nil {
				t.Fatal(err)
			}
			if scenario.sql != "" {
				if _, err := db.Exec(scenario.sql); err != nil {
					t.Fatal(err)
				}
			}
			_, err := service.CreateFromAlias(t.Context(), scenario.user, scenario.subjectID, scenario.aliasID, scenario.nameInput)
			if !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("create: got %v, want not found", err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM subjects WHERE subject_name='Golang'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("created Subjects: got %d, want 0", count)
			}
		})
	}
	t.Run("failed creation restores the alias", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		alias, err := service.CreateAlias(t.Context(), "owner", 1, "Golang")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TRIGGER reject_new_subject BEFORE INSERT ON subjects BEGIN SELECT RAISE(ABORT,'private write failure'); END`); err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateFromAlias(t.Context(), "owner", 1, alias.AliasID, "Golang"); err == nil {
			t.Fatal("creation succeeded, want injected failure")
		}
		aliases, err := service.ListAliases(t.Context(), "owner", 1)
		if err != nil || len(aliases) != 1 || aliases[0] != *alias {
			t.Fatalf("rolled back aliases: got %+v,%v, want %+v", aliases, err, alias)
		}
	})
	t.Run("cancellation keeps the alias usable", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		alias, err := service.CreateAlias(t.Context(), "owner", 1, "Golang")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := service.CreateFromAlias(ctx, "owner", 1, alias.AliasID, "Golang"); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled creation: got %v, want canceled", err)
		}
		resolved, err := service.ResolveName(t.Context(), "owner", "Golang")
		if err != nil || resolved.SubjectID != 1 {
			t.Fatalf("alias after cancellation: got %+v,%v, want Subject 1", resolved, err)
		}
	})
}

func TestSubjectFromAliasConcurrencyWorkflow_SQLite(t *testing.T) {
	t.Run("only one confirmation can consume the alias", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		alias, err := service.CreateAlias(t.Context(), "owner", 1, "Golang")
		if err != nil {
			t.Fatal(err)
		}
		otherDB := openSQLite(t, dsn)
		for _, connection := range []*sql.DB{db, otherDB} {
			if _, err := connection.Exec(`PRAGMA busy_timeout=2000`); err != nil {
				t.Fatal(err)
			}
		}
		other := subject.NewService(data.NewSQLiteSubjectStore(otherDB))
		start := make(chan struct{})
		results := make(chan error, 2)
		var workers sync.WaitGroup
		for _, writer := range []*subject.Service{service, other} {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				_, err := writer.CreateFromAlias(t.Context(), "owner", 1, alias.AliasID, "Golang")
				results <- err
			}()
		}
		close(start)
		workers.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			} else if !errors.Is(err, data.ErrRecordNotFound) && !errors.Is(err, data.ErrDatabaseBusy) {
				t.Fatalf("competing confirmation: got %v, want unavailable or busy", err)
			}
		}
		if successes != 1 {
			t.Fatalf("successful confirmations: got %d, want 1", successes)
		}
		var names int
		if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM subjects WHERE user_id='owner' AND subject_name='Golang' AND deleted_at IS NULL)+(SELECT COUNT(*) FROM subject_aliases WHERE user_id='owner' AND alias_name='Golang' AND deleted_at IS NULL)`).Scan(&names); err != nil {
			t.Fatal(err)
		}
		if names != 1 {
			t.Fatalf("active name owners: got %d, want 1", names)
		}
	})
}
