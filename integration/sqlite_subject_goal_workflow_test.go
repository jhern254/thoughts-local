//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func seedSubjectGoals(t *testing.T, db *sql.DB) {
	t.Helper()
	insertUsers(t, db, "owner", "other")
	_, err := db.Exec(`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES
 (1,'owner','Coding'),(2,'owner','Learning'),(3,'other','Other');
 INSERT INTO goals(goal_id,user_id,goal_name,target_seconds,goal_is_active) VALUES
 (1,'owner','Practice',60,1),(2,'owner','Project',60,0),(3,'other','Other',60,1);`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSubjectGoalStoreWorkflow_SQLite(t *testing.T) {
	t.Run("persists many to many links including inactive goals and removes only the requested link", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		// Insertion order deliberately differs from both list orders.
		for _, pair := range [][2]int64{{2, 1}, {1, 2}, {1, 1}} {
			if err := store.AddSubjectGoal(t.Context(), "owner", pair[0], pair[1]); err != nil {
				t.Fatal(err)
			}
		}
		reader := data.NewSQLiteSubjectGoalStore(openSQLite(t, dsn))
		got, err := reader.ListSubjectGoals(t.Context(), "owner", 1)
		want := []data.SubjectGoal{{SubjectID: 1, GoalID: 1, UserID: "owner"}, {SubjectID: 1, GoalID: 2, UserID: "owner"}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("subject links: got %+v, %v; want %+v, nil", got, err, want)
		}
		got, err = reader.ListGoalSubjects(t.Context(), "owner", 1)
		want = []data.SubjectGoal{{SubjectID: 1, GoalID: 1, UserID: "owner"}, {SubjectID: 2, GoalID: 1, UserID: "owner"}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("goal links: got %+v, %v; want %+v, nil", got, err, want)
		}
		if err := store.RemoveSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		got, err = reader.ListGoalSubjects(t.Context(), "owner", 1)
		if err != nil || !reflect.DeepEqual(got, want[1:]) {
			t.Fatalf("remaining goal links: got %+v, %v; want %+v, nil", got, err, want[1:])
		}
		got, err = reader.ListSubjectGoals(t.Context(), "owner", 1)
		want = []data.SubjectGoal{{SubjectID: 1, GoalID: 2, UserID: "owner"}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("remaining subject links: got %+v, %v; want %+v, nil", got, err, want)
		}
	})

	t.Run("rejects duplicate links while preserving the SQLite cause", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		err := store.AddSubjectGoal(t.Context(), "owner", 1, 1)
		var cause *sqlite.Error
		if !errors.Is(err, data.ErrDuplicateRecord) || !errors.As(err, &cause) || cause.Code() != sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
			t.Fatalf("duplicate error: got %v, want ErrDuplicateRecord wrapping SQLite primary key error", err)
		}
	})

	for _, tc := range []struct {
		name, setup, user string
		subject, goal     int64
	}{
		{"missing subject", "", "owner", 99, 1},
		{"missing goal", "", "owner", 1, 99},
		{"foreign subject", "", "owner", 3, 1},
		{"foreign goal", "", "owner", 1, 3},
		{"foreign pair", "", "other", 1, 1},
		{"deleted subject", "UPDATE subjects SET deleted_at=updated_at WHERE subject_id=1", "owner", 1, 1},
		{"deleted goal", "UPDATE goals SET deleted_at=updated_at WHERE goal_id=1", "owner", 1, 1},
		{"deleted owner", "UPDATE users SET deleted_at=updated_at WHERE user_id='owner'", "owner", 1, 1},
		{"missing owner", "", "missing", 1, 1},
	} {
		t.Run("rejects adding with "+tc.name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedSubjectGoals(t, db)
			if tc.setup != "" {
				if _, err := db.Exec(tc.setup); err != nil {
					t.Fatal(err)
				}
			}
			err := data.NewSQLiteSubjectGoalStore(db).AddSubjectGoal(t.Context(), tc.user, tc.subject, tc.goal)
			if !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("add error: got %v, want ErrRecordNotFound", err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM subject_goals").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("links after rejected add: got %d, want 0", count)
			}
		})
	}

	t.Run("scopes reads and removal to an undeleted owner", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			user string
			id   int64
		}{{"other", 1}, {"missing", 1}, {"owner", 99}, {"owner", 2}} {
			for _, list := range []func(context.Context, string, int64) ([]data.SubjectGoal, error){store.ListSubjectGoals, store.ListGoalSubjects} {
				got, err := list(t.Context(), tc.user, tc.id)
				if err != nil || got == nil || len(got) != 0 {
					t.Fatalf("inaccessible or empty list: got %+v, %v; want non-nil empty, nil", got, err)
				}
			}
			if err := store.RemoveSubjectGoal(t.Context(), tc.user, tc.id, tc.id); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("remove error: got %v, want ErrRecordNotFound", err)
			}
		}
		if _, err := db.Exec("UPDATE users SET deleted_at=updated_at WHERE user_id='owner'"); err != nil {
			t.Fatal(err)
		}
		for _, list := range []func(context.Context, string, int64) ([]data.SubjectGoal, error){store.ListSubjectGoals, store.ListGoalSubjects} {
			got, err := list(t.Context(), "owner", 1)
			if err != nil || got == nil || len(got) != 0 {
				t.Fatalf("deleted owner list: got %+v, %v; want non-nil empty, nil", got, err)
			}
		}
		if err := store.RemoveSubjectGoal(t.Context(), "owner", 1, 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("deleted owner removal: got %v, want ErrRecordNotFound", err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM subject_goals").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("retained links: got %d, want 1", count)
		}
	})

	t.Run("retains links after endpoint soft deletion and permits explicit unlinking", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		if err := data.NewSQLiteSubjectStore(db).DeleteSubject(t.Context(), "owner", 1); err != nil {
			t.Fatal(err)
		}
		if err := data.NewSQLiteGoalStore(db).DeleteGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		want := []data.SubjectGoal{{SubjectID: 1, GoalID: 1, UserID: "owner"}}
		for _, list := range []func(context.Context, string, int64) ([]data.SubjectGoal, error){store.ListSubjectGoals, store.ListGoalSubjects} {
			got, err := list(t.Context(), "owner", 1)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("retained links: got %+v, %v; want %+v, nil", got, err, want)
			}
		}
		if err := store.RemoveSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		got, err := store.ListSubjectGoals(t.Context(), "owner", 1)
		if err != nil || len(got) != 0 {
			t.Fatalf("unlinked subject: got %+v, %v; want empty, nil", got, err)
		}
	})

	t.Run("linking and unlinking preserve endpoints and progress provenance", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		_, err := db.Exec(`INSERT INTO events(event_id,user_id,subject_id,started_at) VALUES (1,'owner',1,100);
 INSERT INTO thoughts(thought_id,user_id,subject_id,event_id,thought) VALUES (1,'owner',1,1,'Context');
 INSERT INTO goal_progress(goal_id,user_id,event_id,thought_id,time_spent_sec) VALUES (1,'owner',1,1,60);`)
		if err != nil {
			t.Fatal(err)
		}
		const snapshot = `SELECT s.updated_at,g.updated_at,g.version,e.version,t.version,e.subject_id,t.subject_id,p.event_id,p.thought_id,p.time_spent_sec
 FROM subjects s,goals g,events e,thoughts t,goal_progress p WHERE s.subject_id=1 AND g.goal_id=1`
		var before, after [10]int64
		if err := db.QueryRow(snapshot).Scan(&before[0], &before[1], &before[2], &before[3], &before[4], &before[5], &before[6], &before[7], &before[8], &before[9]); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteSubjectGoalStore(db)
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		if err := store.RemoveSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(snapshot).Scan(&after[0], &after[1], &after[2], &after[3], &after[4], &after[5], &after[6], &after[7], &after[8], &after[9]); err != nil {
			t.Fatal(err)
		}
		if after != before {
			t.Fatalf("endpoint and provenance state: got %v, want %v", after, before)
		}
	})

	t.Run("preserves cancellation errors for every operation", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		store := data.NewSQLiteSubjectGoalStore(db)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, subjectsErr := store.ListGoalSubjects(ctx, "owner", 1)
		_, goalsErr := store.ListSubjectGoals(ctx, "owner", 1)
		for _, err := range []error{store.AddSubjectGoal(ctx, "owner", 1, 1), store.RemoveSubjectGoal(ctx, "owner", 1, 1), subjectsErr, goalsErr} {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("operation error: got %v, want context.Canceled", err)
			}
		}
	})
}
