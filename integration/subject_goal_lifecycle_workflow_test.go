//go:build integration

package integration_test

import (
	"database/sql"
	"errors"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/goalassignment"
	"math"
	"reflect"
	"testing"
)

func TestSubjectGoalLifecycleWorkflow_SQLite(t *testing.T) {
	t.Run("removal retires the relationship and relinking restores its number", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		if err := store.RemoveSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		retained, err := store.ListGoalSubjects(t.Context(), "owner", 1)
		if err != nil || len(retained) != 1 {
			t.Fatalf("retained relationships: got %v,%v; want one", retained, err)
		}
		if err := store.RemoveSubjectGoal(t.Context(), "owner", 1, 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("repeated removal: got %v, want not found", err)
		}
		live, err := store.ListLiveNumberedSubjectsForGoal(t.Context(), "owner", 1)
		if err != nil || live == nil || len(live) != 0 {
			t.Fatalf("live: got %v,%v, want nonnil empty", live, err)
		}
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		live, err = store.ListLiveNumberedSubjectsForGoal(t.Context(), "owner", 1)
		if err != nil || len(live) != 1 || live[0].GoalNumber != 1 {
			t.Fatalf("restored: got %v,%v; want number 1", live, err)
		}
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("active add: got %v,want duplicate", err)
		}
	})
}

func TestSubjectGoalLifecycleVisibility_SQLite(t *testing.T) {
	for _, endpoint := range []string{"subjects", "goals", "users"} {
		t.Run("removal preserves history for a deleted "+endpoint, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedSubjectGoals(t, db)
			store := data.NewSQLiteSubjectGoalStore(db)
			if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
				t.Fatal(err)
			}
			statement := "UPDATE " + endpoint + " SET deleted_at=updated_at WHERE user_id='owner'"
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			if err := store.RemoveSubjectGoal(t.Context(), "owner", 1, 1); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("remove: got %v,want not found", err)
			}
			var number, version int64
			var retired sql.NullInt64
			if err := db.QueryRow("SELECT goal_number,deleted_at FROM subject_goals WHERE subject_id=1 AND goal_id=1").Scan(&number, &retired); err != nil {
				t.Fatal(err)
			}
			if number != 1 || retired.Valid {
				t.Fatalf("history: got number %d, retirement %v; want #1, unretired", number, retired)
			}
			if err := db.QueryRow("SELECT version FROM goals WHERE goal_id=1").Scan(&version); err != nil {
				t.Fatal(err)
			}
			if version != 2 {
				t.Fatalf("version: got %d,want 2", version)
			}
		})
	}
	t.Run("retired membership is absent from all live reads and discovery", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		if err := store.RemoveSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		subjects, err := store.ListLiveSubjectsForGoal(t.Context(), "owner", 1)
		if err != nil || subjects == nil || len(subjects) != 0 {
			t.Fatalf("live subjects: got %v,%v,want nonnil empty", subjects, err)
		}
		goals, err := store.ListActiveGoalsForSubject(t.Context(), "owner", 1)
		if err != nil || len(goals) != 0 {
			t.Fatalf("discovery goals: got %v,%v,want empty", goals, err)
		}
		retained, err := store.ListSubjectGoals(t.Context(), "owner", 1)
		if err != nil || len(retained) != 1 || retained[0].DeletedAt == nil || retained[0].GoalNumber != 1 {
			t.Fatalf("history: got %v,%v,want retired #1", retained, err)
		}
	})
}

func TestSubjectGoalReplacementLifecycle_SQLite(t *testing.T) {
	t.Run("no-op preserves retirement timestamps and reactivation changes version once", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		service := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), store)
		item, err := service.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "Lifecycle", TargetSeconds: 60}, []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		retired, err := service.ReplaceSubjects(t.Context(), "owner", item.GoalID, item.Version, nil)
		if err != nil {
			t.Fatal(err)
		}
		before, err := store.ListGoalSubjects(t.Context(), "owner", item.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		same, err := service.ReplaceSubjects(t.Context(), "owner", item.GoalID, retired.Version, nil)
		if err != nil {
			t.Fatal(err)
		}
		after, err := store.ListGoalSubjects(t.Context(), "owner", item.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(same, retired) {
			t.Fatalf("no-op changed relationship history or Goal: before=%v after=%v", before, after)
		}
		restored, err := service.ReplaceSubjects(t.Context(), "owner", item.GoalID, same.Version, []int64{2})
		if err != nil {
			t.Fatal(err)
		}
		if restored.Version != 3 {
			t.Fatalf("version: got %d,want 3", restored.Version)
		}
		rows, err := store.ListGoalSubjects(t.Context(), "owner", item.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].DeletedAt == nil || rows[1].DeletedAt != nil || rows[0].GoalNumber != 1 || rows[1].GoalNumber != 1 {
			t.Fatalf("restored links: got %v,want retired Subject 1 and live Subject 2, both #1", rows)
		}
	})
	t.Run("number overflow rejects new pairs but permits reactivation", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		if _, err := db.Exec("INSERT INTO subject_goals(subject_id,goal_id,user_id,goal_number,deleted_at) VALUES(1,1,'owner',9223372036854775807,1)"); err != nil {
			t.Fatal(err)
		}
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 2); err == nil {
			t.Fatal("new pair succeeded past integer ceiling")
		}
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		refs, err := store.ListLiveNumberedSubjectsForGoal(t.Context(), "owner", 1)
		if err != nil || len(refs) != 1 || refs[0].GoalNumber != math.MaxInt64 {
			t.Fatalf("restored ceiling: got %v,%v", refs, err)
		}
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("duplicate at ceiling: got %v,want duplicate", err)
		}
	})
}

func TestSubjectGoalReactivationAtomicity_SQLite(t *testing.T) {
	t.Run("failed second reactivation restores retirement state and Goal version", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		service := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), store)
		item, err := service.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "Atomic reactivation", TargetSeconds: 60}, []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		item, err = service.ReplaceSubjects(t.Context(), "owner", item.GoalID, item.Version, nil)
		if err != nil {
			t.Fatal(err)
		}
		before, err := store.ListGoalSubjects(t.Context(), "owner", item.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TEMP TRIGGER reject_second_reactivation BEFORE UPDATE ON subject_goals
 WHEN NEW.subject_id=2 AND NEW.deleted_at IS NULL BEGIN SELECT RAISE(ABORT,'controlled failure'); END`); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ReplaceSubjects(t.Context(), "owner", item.GoalID, item.Version, []int64{1, 2}); err == nil {
			t.Fatal("replacement: got nil error, want rollback")
		}
		after, err := store.ListGoalSubjects(t.Context(), "owner", item.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		persisted, err := data.NewSQLiteGoalStore(db).GetGoal(t.Context(), "owner", item.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(item, persisted) {
			t.Fatalf("rollback changed history or Goal: before=%v after=%v Goal=%+v", before, after, persisted)
		}
	})
}
