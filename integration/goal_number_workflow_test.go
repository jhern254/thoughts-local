//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/goalassignment"
)

func TestGoalNumberWorkflow_SQLite(t *testing.T) {
	t.Run("relinking restores a reserved number and later goals leave gaps", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		if _, err := db.Exec("INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES(4,'owner','Fourth',60)"); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteSubjectGoalStore(db)
		for _, id := range []int64{1, 2} {
			if err := store.AddSubjectGoal(t.Context(), "owner", 1, id); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.RemoveSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 4); err != nil {
			t.Fatal(err)
		}
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		for id, want := range map[int64]int64{1: 1, 2: 2, 4: 3} {
			refs, err := store.ListLiveNumberedSubjectsForGoal(t.Context(), "owner", id)
			if err != nil || len(refs) != 1 {
				t.Fatalf("goal %d references: got %v, %v, want one", id, refs, err)
			}
			if got := refs[0].GoalNumber; got != want {
				t.Fatalf("goal %d number: got %d, want %d", id, got, want)
			}
		}
	})
}

func TestGoalNumberBoundaries_SQLite(t *testing.T) {
	t.Run("numbers are independent per Subject and survive reopening", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		s := data.NewSQLiteSubjectGoalStore(db)
		for _, pair := range [][2]int64{{1, 2}, {1, 1}, {2, 1}} {
			if err := s.AddSubjectGoal(t.Context(), "owner", pair[0], pair[1]); err != nil {
				t.Fatal(err)
			}
		}
		refs, err := data.NewSQLiteSubjectGoalStore(openSQLite(t, dsn)).ListLiveNumberedSubjectsForGoal(t.Context(), "owner", 1)
		if err != nil || len(refs) != 2 {
			t.Fatalf("references: got %v,%v; want two", refs, err)
		}
		if refs[0].GoalNumber != 2 || refs[1].GoalNumber != 1 {
			t.Fatalf("numbers: got %v, want 2 and 1", refs)
		}
	})
	t.Run("allocation failure rolls back the entire creation", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		if _, err := db.Exec(`CREATE TRIGGER reject_number BEFORE INSERT ON subject_goals BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
			t.Fatal(err)
		}
		s := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), data.NewSQLiteSubjectGoalStore(db))
		if _, err := s.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "Rollback", TargetSeconds: 60}, []int64{1, 2}); err == nil {
			t.Fatal("expected allocation failure")
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM goals WHERE goal_name='Rollback'").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("goals: got %d, want 0", count)
		}
	})
}

func TestGoalNumberMigration_SQLite(t *testing.T) {
	t.Run("backfills retained links without changing Goal versions", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		legacy, err := os.ReadFile("../migrations/000011_create_subject_goals.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("DROP TABLE subject_goals;" + string(legacy)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO subject_goals(subject_id,goal_id,user_id) VALUES (1,2,'owner'),(1,1,'owner'),(2,1,'owner'); UPDATE subjects SET deleted_at=updated_at WHERE subject_id=2`); err != nil {
			t.Fatal(err)
		}
		migration, err := os.ReadFile("../migrations/000015_soft_delete_subject_goals.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(migration)); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct{ subject, goal, number int64 }{{1, 1, 1}, {1, 2, 2}, {2, 1, 1}} {
			var got int64
			if err := db.QueryRow("SELECT goal_number FROM subject_goals WHERE subject_id=? AND goal_id=?", tc.subject, tc.goal).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.number {
				t.Fatalf("number for Subject %d Goal %d: got %d, want %d", tc.subject, tc.goal, got, tc.number)
			}
		}
		var version int64
		if err := db.QueryRow("SELECT version FROM goals WHERE goal_id=1").Scan(&version); err != nil {
			t.Fatal(err)
		}
		if version != 1 {
			t.Fatalf("version: got %d, want 1", version)
		}
		rows, err := db.Query("PRAGMA foreign_key_check")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Fatal("foreign key violation")
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("replacement preserves reservations and no-op versions", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		service := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), store)
		g, err := service.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "Numbers", TargetSeconds: 60}, []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		first, err := store.ListLiveNumberedSubjectsForGoal(t.Context(), "owner", g.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		g, err = service.ReplaceSubjects(t.Context(), "owner", g.GoalID, g.Version, nil)
		if err != nil {
			t.Fatal(err)
		}
		g, err = service.ReplaceSubjects(t.Context(), "owner", g.GoalID, g.Version, []int64{2, 1})
		if err != nil {
			t.Fatal(err)
		}
		again, err := store.ListLiveNumberedSubjectsForGoal(t.Context(), "owner", g.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("restored numbers: got %v, want %v", again, first)
		}
		same, err := service.ReplaceSubjects(t.Context(), "owner", g.GoalID, g.Version, []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		if same.Version != g.Version {
			t.Fatalf("no-op version: got %d, want %d", same.Version, g.Version)
		}
	})
}

func TestGoalNumberVisibility_SQLite(t *testing.T) {
	t.Run("users have independent numbers and deleted Subjects stay historical", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		s := data.NewSQLiteSubjectGoalStore(db)
		for _, tc := range []struct {
			user          string
			subject, goal int64
		}{{"owner", 1, 1}, {"other", 3, 3}} {
			if err := s.AddSubjectGoal(t.Context(), tc.user, tc.subject, tc.goal); err != nil {
				t.Fatal(err)
			}
			refs, err := s.ListLiveNumberedSubjectsForGoal(t.Context(), tc.user, tc.goal)
			if err != nil || len(refs) != 1 || refs[0].GoalNumber != 1 {
				t.Fatalf("references: got %v,%v, want number 1", refs, err)
			}
		}
		if err := data.NewSQLiteSubjectStore(db).DeleteSubject(t.Context(), "owner", 1); err != nil {
			t.Fatal(err)
		}
		refs, err := s.ListLiveNumberedSubjectsForGoal(t.Context(), "owner", 1)
		if err != nil || refs == nil || len(refs) != 0 {
			t.Fatalf("deleted live membership: got %v,%v, want nonnil empty", refs, err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM subject_goals WHERE subject_id=1").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("reservations: got %d, want 1", count)
		}
		if _, err := s.ListLiveNumberedSubjectsForGoal(t.Context(), "other", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("foreign read: got %v, want not found", err)
		}
	})
	t.Run("cancelled creation cannot reserve numbers", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		service := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), data.NewSQLiteSubjectGoalStore(db))
		if _, err := service.Create(ctx, "owner", goal.CreateGoalInput{Name: "Cancelled", TargetSeconds: 60}, []int64{1}); !errors.Is(err, context.Canceled) {
			t.Fatalf("creation: got %v, want cancelled", err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM subject_goals").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("reservations: got %d, want 0", count)
		}
	})
}

func TestGoalNumberConcurrency_SQLite(t *testing.T) {
	t.Run("concurrent link writers cannot reserve the same number", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		other := openSQLite(t, dsn)
		start := make(chan struct{})
		done := make(chan struct {
			id  int64
			err error
		}, 2)
		for i, connection := range []*sql.DB{db, other} {
			go func(id int64, connection *sql.DB) {
				<-start
				err := data.NewSQLiteSubjectGoalStore(connection).AddSubjectGoal(t.Context(), "owner", 1, id)
				done <- struct {
					id  int64
					err error
				}{id, err}
			}(int64(i+1), connection)
		}
		close(start)
		// Join both writers before retrying contention; a completed writer must
		// not start another transaction while its peer is still committing.
		first, second := <-done, <-done
		for _, result := range []struct {
			id  int64
			err error
		}{first, second} {
			if errors.Is(result.err, data.ErrDatabaseBusy) {
				result.err = data.NewSQLiteSubjectGoalStore(db).AddSubjectGoal(t.Context(), "owner", 1, result.id)
			}
			if result.err != nil {
				t.Fatal(result.err)
			}
		}
		var total, distinct int
		if err := db.QueryRow("SELECT count(*),count(DISTINCT goal_number) FROM subject_goals WHERE subject_id=1").Scan(&total, &distinct); err != nil {
			t.Fatal(err)
		}
		if total != 2 || distinct != 2 {
			t.Fatalf("reservations: got %d total, %d distinct; want 2 of each", total, distinct)
		}
	})
}
