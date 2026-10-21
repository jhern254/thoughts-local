//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"modernc.org/sqlite"
)

func seedAliasSubjects(t *testing.T, db *sql.DB) *subject.Service {
	t.Helper()
	insertUsers(t, db, "owner", "other")
	if _, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES
 (1,'owner','Go Programming'),(2,'owner','Writing'),(3,'other','Go Programming')`); err != nil {
		t.Fatal(err)
	}
	return subject.NewService(data.NewSQLiteSubjectStore(db))
}

func TestSubjectAliasWorkflow_SQLite(t *testing.T) {
	t.Run("canonical case variants remain independent from differently cased aliases", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		first, err := service.Create(t.Context(), "owner", "coding")
		if err != nil {
			t.Fatal(err)
		}
		second, err := service.Create(t.Context(), "owner", "coDing")
		if err != nil {
			t.Fatal(err)
		}
		if first.SubjectID == second.SubjectID {
			t.Fatal("case variants resolved to one Subject")
		}
		if _, err := service.CreateAlias(t.Context(), "owner", first.SubjectID, "Coding"); err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateAlias(t.Context(), "owner", first.SubjectID, "coding"); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("own canonical collision: got %v, want duplicate", err)
		}
	})
	t.Run("unrelated trigger failures are not mislabeled as namespace conflicts", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		if _, err := db.Exec(`CREATE TRIGGER reject_alias_insert BEFORE INSERT ON subject_aliases BEGIN SELECT RAISE(ABORT,'synthetic subject name conflict failure'); END`); err != nil {
			t.Fatal(err)
		}
		_, err := service.CreateAlias(t.Context(), "owner", 1, "Golang")
		var cause *sqlite.Error
		if !errors.As(err, &cause) || errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("unrelated trigger: got %v, want original unexpected failure", err)
		}
	})
	t.Run("persists exact aliases and resolves to the canonical identity after reopening", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		var want []data.SubjectAlias
		for _, name := range []string{"Programming in Go", "Golang", "coding", "coDing", "  名称  "} {
			alias, err := service.CreateAlias(t.Context(), "owner", 1, name)
			if err != nil {
				t.Fatal(err)
			}
			want = append(want, *alias)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		reopened := subject.NewService(data.NewSQLiteSubjectStore(openSQLite(t, dsn)))
		got, err := reopened.ListAliases(t.Context(), "owner", 1)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("persisted aliases: got %+v, %v; want %+v, nil", got, err, want)
		}
		for _, name := range []string{"Go Programming", "Programming in Go", "Golang", "coding", "coDing", "  名称  "} {
			resolved, err := reopened.ResolveName(t.Context(), "owner", name)
			if err != nil || resolved.SubjectID != 1 || resolved.SubjectName != "Go Programming" {
				t.Fatalf("resolve %q: got %+v, %v; want canonical Subject 1", name, resolved, err)
			}
		}
		for _, name := range []string{"Coding", "golang", "名称", "unknown"} {
			if _, err := reopened.ResolveName(t.Context(), "owner", name); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("resolve %q: got %v, want not found", name, err)
			}
		}
	})

	t.Run("rejects conflicts in both write directions without changing the existing alias", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		alias, err := service.CreateAlias(t.Context(), "owner", 1, "Golang")
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"Go Programming", "Writing", "Golang"} {
			if _, err := service.CreateAlias(t.Context(), "owner", 2, name); !errors.Is(err, data.ErrDuplicateRecord) {
				t.Fatalf("create alias %q: got %v, want duplicate", name, err)
			}
		}
		for _, name := range []string{"Go Programming", "Writing"} {
			if _, err := service.UpdateAlias(t.Context(), "owner", 1, alias.AliasID, name); !errors.Is(err, data.ErrDuplicateRecord) {
				t.Fatalf("edit alias %q: got %v, want duplicate", name, err)
			}
		}
		_, err = service.Create(t.Context(), "owner", "Golang")
		var conflict *data.SubjectAliasConflictError
		var cause *sqlite.Error
		if !errors.Is(err, data.ErrDuplicateRecord) || !errors.As(err, &conflict) || !errors.As(err, &cause) {
			t.Fatalf("create canonical: got %v, want alias conflict retaining duplicate identity and SQLite cause", err)
		}
		if conflict.AliasName != "Golang" || conflict.SubjectID != 1 || conflict.SubjectName != "Go Programming" {
			t.Fatalf("alias conflict: got alias %q, Subject %d/%q; want Golang, 1/Go Programming", conflict.AliasName, conflict.SubjectID, conflict.SubjectName)
		}
		if strings.Contains(err.Error(), "Golang") || strings.Contains(err.Error(), "Go Programming") {
			t.Fatalf("conflict error text: got %q, want no authored names", err.Error())
		}
		_, err = service.Create(t.Context(), "owner", "Go Programming")
		conflict = nil
		if !errors.Is(err, data.ErrDuplicateRecord) || errors.As(err, &conflict) {
			t.Fatalf("canonical duplicate: got %v, want ordinary duplicate without alias details", err)
		}
		if _, err := service.Update(t.Context(), "owner", 2, "Golang"); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("rename canonical: got %v, want duplicate", err)
		}
		if _, err := service.UpdateAlias(t.Context(), "owner", 1, alias.AliasID, "Golang"); err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateAlias(t.Context(), "other", 3, "Golang"); err != nil {
			t.Fatalf("other owner's name: %v", err)
		}
		got, err := service.ListAliases(t.Context(), "owner", 1)
		if err != nil || len(got) != 1 || got[0].AliasName != "Golang" {
			t.Fatalf("retained alias: got %+v, %v; want Golang", got, err)
		}
	})

	t.Run("another user's alias does not block creation or disclose its Subject", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		if _, err := service.CreateAlias(t.Context(), "owner", 1, "Golang"); err != nil {
			t.Fatal(err)
		}
		created, err := service.Create(t.Context(), "other", "Golang")
		if err != nil || created.UserID != "other" || created.SubjectName != "Golang" {
			t.Fatalf("other user's Subject: got %+v,%v; want own new Subject without alias disclosure", created, err)
		}
	})

	t.Run("edits and removes only the alias leaving all related records unchanged", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		if _, err := db.Exec(`INSERT INTO events(event_id,user_id,subject_id,started_at,ended_at) VALUES(1,'owner',1,100,200);
 INSERT INTO thoughts(thought_id,user_id,subject_id,event_id,thought) VALUES(1,'owner',1,1,'Retain text');
 INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES(1,'owner','Practice',60);
 INSERT INTO subject_goals(subject_id,goal_id,user_id) VALUES(1,1,'owner');
 INSERT INTO goal_progress(goal_id,user_id,time_spent_sec,thought_id,event_id) VALUES(1,'owner',60,1,1)`); err != nil {
			t.Fatal(err)
		}
		alias, err := service.CreateAlias(t.Context(), "owner", 1, "Golang")
		if err != nil {
			t.Fatal(err)
		}
		updated, err := service.UpdateAlias(t.Context(), "owner", 1, alias.AliasID, "Go development")
		if err != nil || updated.AliasID != alias.AliasID || updated.CreatedAt != alias.CreatedAt || updated.UpdatedAt.Before(alias.UpdatedAt) {
			t.Fatalf("updated alias: got %+v, %v; want retained identity and nondecreasing timestamps", updated, err)
		}
		if err := service.RemoveAlias(t.Context(), "owner", 1, alias.AliasID); err != nil {
			t.Fatal(err)
		}
		if err := service.RemoveAlias(t.Context(), "owner", 1, alias.AliasID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("repeated removal: got %v, want not found", err)
		}
		if _, err := service.ResolveName(t.Context(), "owner", "Go development"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("removed lookup: got %v, want not found", err)
		}
		if _, err := service.CreateAlias(t.Context(), "owner", 2, "Go development"); err != nil {
			t.Fatal(err)
		}
		for _, query := range []string{
			`SELECT COUNT(*) FROM subjects WHERE subject_id=1 AND subject_name='Go Programming' AND created_at=updated_at AND deleted_at IS NULL`,
			`SELECT COUNT(*) FROM thoughts WHERE thought_id=1 AND subject_id=1 AND event_id=1 AND thought='Retain text' AND version=1 AND updated_at=created_at`,
			`SELECT COUNT(*) FROM events WHERE event_id=1 AND subject_id=1 AND started_at=100 AND ended_at=200 AND version=1 AND updated_at=created_at`,
			`SELECT COUNT(*) FROM goals WHERE goal_id=1 AND goal_name='Practice' AND version=1 AND updated_at=created_at`,
			`SELECT COUNT(*) FROM subject_goals WHERE subject_id=1 AND goal_id=1`,
			`SELECT COUNT(*) FROM goal_progress WHERE goal_id=1 AND time_spent_sec=60 AND thought_id=1 AND event_id=1`,
			`SELECT COUNT(*) FROM subject_aliases WHERE alias_id=1 AND deleted_at IS NOT NULL`,
		} {
			var got int
			if err := db.QueryRow(query).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != 1 {
				t.Fatalf("%s: got %d, want 1", query, got)
			}
		}
	})

	for _, scenario := range []struct {
		name, setup, user string
		subject           int64
	}{
		{"foreign owner", "", "other", 1},
		{"wrong canonical subject", "", "owner", 2},
		{"missing subject", "", "owner", 99},
		{"deleted subject", `UPDATE subjects SET deleted_at=updated_at WHERE subject_id=1`, "owner", 1},
		{"deleted user", `UPDATE users SET deleted_at=updated_at WHERE user_id='owner'`, "owner", 1},
	} {
		t.Run("rejects alias operations for "+scenario.name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			service := seedAliasSubjects(t, db)
			alias, err := service.CreateAlias(t.Context(), "owner", 1, "Golang")
			if err != nil {
				t.Fatal(err)
			}
			if scenario.setup != "" {
				if _, err := db.Exec(scenario.setup); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.UpdateAlias(t.Context(), scenario.user, scenario.subject, alias.AliasID, "Changed"); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("edit: got %v, want not found", err)
			}
			if err := service.RemoveAlias(t.Context(), scenario.user, scenario.subject, alias.AliasID); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("remove: got %v, want not found", err)
			}
			// A different active Subject may own new aliases, but may not edit this one.
			if scenario.name != "wrong canonical subject" {
				if _, err := service.CreateAlias(t.Context(), scenario.user, scenario.subject, "New"); !errors.Is(err, data.ErrRecordNotFound) {
					t.Fatalf("create: got %v, want not found", err)
				}
				if _, err := service.ListAliases(t.Context(), scenario.user, scenario.subject); !errors.Is(err, data.ErrRecordNotFound) {
					t.Fatalf("list: got %v, want not found", err)
				}
			}
		})
	}

	t.Run("subject deletion retires aliases and releases their names", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		if _, err := service.CreateAlias(t.Context(), "owner", 1, "Golang"); err != nil {
			t.Fatal(err)
		}
		if err := service.Delete(t.Context(), "owner", 1); err != nil {
			t.Fatal(err)
		}
		var active, retained int
		if err := db.QueryRow(`SELECT COUNT(*),COUNT(deleted_at) FROM subject_aliases WHERE subject_id=1`).Scan(&retained, &active); err != nil {
			t.Fatal(err)
		}
		if retained != 1 || active != 1 {
			t.Fatalf("retained/deleted aliases: got %d/%d, want 1/1", retained, active)
		}
		if _, err := service.ResolveName(t.Context(), "owner", "Golang"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("retired lookup: got %v, want not found", err)
		}
		if _, err := service.Create(t.Context(), "owner", "Golang"); err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateAlias(t.Context(), "owner", 2, "Go Programming"); err != nil {
			t.Fatal(err)
		}
	})

	for _, table := range []string{"subject_aliases", "thoughts"} {
		t.Run("rolls back subject and aliases after a "+table+" failure", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			service := seedAliasSubjects(t, db)
			if _, err := service.CreateAlias(t.Context(), "owner", 1, "Golang"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO thoughts(user_id,subject_id,thought) VALUES('owner',1,'Unchanged');
 CREATE TRIGGER reject_alias_workflow BEFORE UPDATE ON ` + table + ` BEGIN SELECT RAISE(ABORT,'synthetic write failure'); END`); err != nil {
				t.Fatal(err)
			}
			err := service.Delete(t.Context(), "owner", 1)
			var cause *sqlite.Error
			if !errors.As(err, &cause) || errors.Is(err, data.ErrDuplicateRecord) {
				t.Fatalf("delete failure: got %v, want original SQLite failure", err)
			}
			var retained int
			if err := db.QueryRow(`SELECT COUNT(*) FROM subjects s JOIN subject_aliases a ON a.subject_id=s.subject_id JOIN thoughts t ON t.subject_id=s.subject_id WHERE s.subject_id=1 AND s.deleted_at IS NULL AND a.deleted_at IS NULL AND t.version=1`).Scan(&retained); err != nil {
				t.Fatal(err)
			}
			if retained != 1 {
				t.Fatalf("unchanged relationship: got %d, want 1", retained)
			}
		})
	}

	t.Run("lists active aliases for only the requesting user and honors cancellation", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := seedAliasSubjects(t, db)
		for _, input := range []struct {
			user    string
			subject int64
			name    string
		}{{"owner", 1, "Zed"}, {"owner", 2, "Alpha"}, {"other", 3, "Golang"}} {
			if _, err := service.CreateAlias(t.Context(), input.user, input.subject, input.name); err != nil {
				t.Fatal(err)
			}
		}
		got, err := service.ListAliasesForUser(t.Context(), "owner")
		if err != nil || len(got) != 2 || got[0].SubjectID != 2 || got[1].SubjectID != 1 {
			t.Fatalf("user aliases: got %+v,%v; want Alpha->2, Zed->1", got, err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := service.ListAliases(ctx, "owner", 1); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled read: got %v, want canceled", err)
		}
		if _, err := service.CreateAlias(ctx, "owner", 1, "Canceled"); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled create: got %v, want canceled", err)
		}
		if _, err := db.Exec(`UPDATE users SET deleted_at=updated_at WHERE user_id='owner'`); err != nil {
			t.Fatal(err)
		}
		got, err = service.ListAliasesForUser(t.Context(), "owner")
		if err != nil || len(got) != 0 {
			t.Fatalf("deleted owner's aliases: got %+v,%v; want none", got, err)
		}
		if _, err := service.ResolveName(t.Context(), "owner", "Zed"); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("deleted owner lookup: got %v, want not found", err)
		}
	})
}

func TestSubjectAliasConcurrencyWorkflow_SQLite(t *testing.T) {
	for _, canonicalAction := range []string{"create", "rename"} {
		for _, aliasAction := range []string{"create", "rename"} {
			t.Run(canonicalAction+" canonical competes with "+aliasAction+" alias", func(t *testing.T) {
				db, dsn := openMigratedSQLite(t)
				service := seedAliasSubjects(t, db)
				alias, err := service.CreateAlias(t.Context(), "owner", 1, "Old alias")
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
				workers.Add(2)
				go func() {
					defer workers.Done()
					<-start
					var err error
					if canonicalAction == "create" {
						_, err = service.Create(t.Context(), "owner", "Shared")
					} else {
						_, err = service.Update(t.Context(), "owner", 2, "Shared")
					}
					results <- err
				}()
				go func() {
					defer workers.Done()
					<-start
					var err error
					if aliasAction == "create" {
						_, err = other.CreateAlias(t.Context(), "owner", 1, "Shared")
					} else {
						_, err = other.UpdateAlias(t.Context(), "owner", 1, alias.AliasID, "Shared")
					}
					results <- err
				}()
				close(start)
				workers.Wait()
				close(results)
				succeeded, conflicted := 0, 0
				for err := range results {
					if err == nil {
						succeeded++
					} else if errors.Is(err, data.ErrDuplicateRecord) {
						conflicted++
					} else {
						t.Fatalf("competing write: got %v, want success or duplicate", err)
					}
				}
				if succeeded != 1 || conflicted != 1 {
					t.Fatalf("success/conflict: got %d/%d, want 1/1", succeeded, conflicted)
				}
				var count int
				if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM subjects WHERE user_id='owner' AND subject_name='Shared' AND deleted_at IS NULL)+(SELECT COUNT(*) FROM subject_aliases WHERE user_id='owner' AND alias_name='Shared' AND deleted_at IS NULL)`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Fatalf("committed exact names: got %d, want 1", count)
				}
			})
		}
	}
}
