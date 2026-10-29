//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/goalassignment"
)

func TestGoalAssignmentWorkflow_SQLite(t *testing.T) {
	for _, ids := range [][]int64{nil, {1}, {2, 1}} {
		t.Run("creates and persists selected subjects "+subjectSelectionName(ids), func(t *testing.T) {
			db, dsn := openMigratedSQLite(t)
			seedSubjectGoals(t, db)
			store := data.NewSQLiteSubjectGoalStore(db)
			service := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), store)
			created, err := service.Create(t.Context(), "owner", goal.CreateGoalInput{Name: " New goal ", TargetSeconds: 60}, ids)
			if err != nil {
				t.Fatal(err)
			}
			if created.GoalName != "New goal" || created.Version != 1 || created.Priority != "normal" || !created.IsActive || created.CreatedAt.IsZero() || created.CreatedAt != created.UpdatedAt {
				t.Fatalf("created goal: got %+v, want normalized creation defaults", created)
			}
			live, err := data.NewSQLiteSubjectGoalStore(openSQLite(t, dsn)).ListLiveSubjectsForGoal(t.Context(), "owner", created.GoalID)
			if err != nil || live == nil || len(live) != len(ids) {
				t.Fatalf("persisted live subjects: got %+v, %v; want %d subjects", live, err, len(ids))
			}
		})
	}
	for _, tc := range []struct {
		name string
		ids  []int64
	}{{"duplicates", []int64{1, 1}}, {"missing", []int64{1, 99}}, {"foreign", []int64{1, 3}}, {"deleted", []int64{1, 2}}} {
		t.Run("rejects "+tc.name+" without creating a goal", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedSubjectGoals(t, db)
			if tc.name == "deleted" {
				if _, err := db.Exec("UPDATE subjects SET deleted_at=updated_at WHERE subject_id=2"); err != nil {
					t.Fatal(err)
				}
			}
			service := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), data.NewSQLiteSubjectGoalStore(db))
			if _, err := service.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "New", TargetSeconds: 60}, tc.ids); err == nil {
				t.Fatal("create error: got nil, want rejected selection")
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM goals").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 3 {
				t.Fatalf("goal count: got %d, want 3", count)
			}
		})
	}
	t.Run("replaces live links and preserves deleted history with deliberate versions", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		service := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), store)
		item, err := service.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "New", TargetSeconds: 60}, []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		unchanged, err := service.ReplaceSubjects(t.Context(), "owner", item.GoalID, 1, []int64{2, 1})
		if err != nil || !reflect.DeepEqual(unchanged, item) {
			t.Fatalf("no-op: got %+v, %v; want %+v", unchanged, err, item)
		}
		if err := data.NewSQLiteSubjectStore(db).DeleteSubject(t.Context(), "owner", 1); err != nil {
			t.Fatal(err)
		}
		updated, err := service.ReplaceSubjects(t.Context(), "owner", item.GoalID, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Version != 2 {
			t.Fatalf("changed version: got %d, want 2", updated.Version)
		}
		item.Version = updated.Version
		item.UpdatedAt = updated.UpdatedAt
		if !reflect.DeepEqual(item, updated) {
			t.Fatalf("unrelated settings: got %+v, want %+v", updated, item)
		}
		live, err := store.ListLiveSubjectsForGoal(t.Context(), "owner", item.GoalID)
		if err != nil || live == nil || len(live) != 0 {
			t.Fatalf("unassigned: got %+v, %v; want nonnil empty", live, err)
		}
		raw, err := store.ListGoalSubjects(t.Context(), "owner", item.GoalID)
		if err != nil || len(raw) != 1 || raw[0].SubjectID != 1 {
			t.Fatalf("retained links: got %+v, %v; want subject 1", raw, err)
		}
		if _, err := service.ReplaceSubjects(t.Context(), "owner", item.GoalID, 1, nil); !errors.Is(err, data.ErrVersionConflict) {
			t.Fatalf("stale no-op: got %v, want version conflict", err)
		}
	})
	t.Run("standalone changes invalidate stale replacement", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		if err := store.AddSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		service := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), store)
		if _, err := service.ReplaceSubjects(t.Context(), "owner", 1, 1, nil); !errors.Is(err, data.ErrVersionConflict) {
			t.Fatalf("stale replacement: got %v, want version conflict", err)
		}
		if err := store.RemoveSubjectGoal(t.Context(), "owner", 1, 1); err != nil {
			t.Fatal(err)
		}
		item, err := data.NewSQLiteGoalStore(db).GetGoal(t.Context(), "owner", 1)
		if err != nil {
			t.Fatal(err)
		}
		if item.Version != 3 {
			t.Fatalf("standalone version: got %d, want 3", item.Version)
		}
	})
}

func subjectSelectionName(ids []int64) string {
	switch len(ids) {
	case 0:
		return "zero"
	case 1:
		return "one"
	default:
		return "multiple"
	}
}

func TestGoalAssignmentTransactionsWorkflow_SQLite(t *testing.T) {
	for _, operation := range []string{"create", "replace"} {
		t.Run(operation+" rolls back earlier writes when a later association fails", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedSubjectGoals(t, db)
			store := data.NewSQLiteSubjectGoalStore(db)
			service := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), store)
			before, err := data.NewSQLiteGoalStore(db).GetGoal(t.Context(), "owner", 1)
			if err != nil {
				t.Fatal(err)
			}
			// The first insert succeeds; the second fails after genuine transaction work.
			if _, err := db.Exec(`CREATE TEMP TRIGGER fail_second_assignment BEFORE INSERT ON subject_goals
    WHEN NEW.subject_id=2 BEGIN SELECT RAISE(ABORT,'controlled association failure'); END;`); err != nil {
				t.Fatal(err)
			}
			if operation == "create" {
				_, err = service.Create(t.Context(), "owner", goal.CreateGoalInput{Name: "New", TargetSeconds: 60}, []int64{1, 2})
			} else {
				_, err = service.ReplaceSubjects(t.Context(), "owner", 1, 1, []int64{1, 2})
			}
			if err == nil {
				t.Fatal("write: got nil error, want forced failure")
			}
			var goals, links int
			if err := db.QueryRow("SELECT count(*) FROM goals").Scan(&goals); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("SELECT count(*) FROM subject_goals").Scan(&links); err != nil {
				t.Fatal(err)
			}
			if goals != 3 || links != 0 {
				t.Fatalf("rolled back counts: got %d goals, %d links; want 3, 0", goals, links)
			}
			after, err := data.NewSQLiteGoalStore(db).GetGoal(t.Context(), "owner", 1)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("rolled back Goal: got %+v, %v; want %+v", after, err, before)
			}
		})
	}
	t.Run("replacement rolls back removals when an insertion fails", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		if _, err := db.Exec(`INSERT INTO subject_goals VALUES(1,1,'owner');
   CREATE TEMP TRIGGER fail_assignment BEFORE INSERT ON subject_goals BEGIN SELECT RAISE(ABORT,'controlled failure'); END;`); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteSubjectGoalStore(db)
		_, err := store.ReplaceGoalSubjects(t.Context(), "owner", 1, 1, []int64{2})
		if err == nil {
			t.Fatal("replace error: got nil, want forced failure")
		}
		live, err := store.ListLiveSubjectsForGoal(t.Context(), "owner", 1)
		if err != nil || len(live) != 1 || live[0].SubjectID != 1 {
			t.Fatalf("membership after rollback: got %+v, %v; want subject 1", live, err)
		}
	})
	for _, ids := range [][]int64{{1, 99}, {1, 3}, {1, 2}} {
		t.Run("rejects invalid replacement "+fmt.Sprint(ids)+" before removing links", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedSubjectGoals(t, db)
			if _, err := db.Exec(`INSERT INTO subject_goals VALUES(1,1,'owner'); UPDATE subjects SET deleted_at=updated_at WHERE subject_id=2;`); err != nil {
				t.Fatal(err)
			}
			store := data.NewSQLiteSubjectGoalStore(db)
			_, err := store.ReplaceGoalSubjects(t.Context(), "owner", 1, 1, ids)
			if !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("replace error: got %v, want not found", err)
			}
			live, err := store.ListLiveSubjectsForGoal(t.Context(), "owner", 1)
			if err != nil || len(live) != 1 || live[0].SubjectID != 1 {
				t.Fatalf("unchanged membership: got %+v, %v; want subject 1", live, err)
			}
			item, err := data.NewSQLiteGoalStore(db).GetGoal(t.Context(), "owner", 1)
			if err != nil || item.Version != 1 {
				t.Fatalf("unchanged version: got %+v, %v; want version 1", item, err)
			}
		})
	}
	for _, tc := range []struct {
		name string
		ids  []int64
		want []int64
	}{
		{"swaps subjects", []int64{2}, []int64{2}}, {"adds while retaining selected subjects", []int64{1, 2}, []int64{1, 2}}, {"clears subjects", nil, []int64{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, dsn := openMigratedSQLite(t)
			seedSubjectGoals(t, db)
			if _, err := db.Exec(`INSERT INTO subject_goals VALUES(1,1,'owner'); UPDATE goals SET updated_at=updated_at+1000 WHERE goal_id=1;`); err != nil {
				t.Fatal(err)
			}
			store := data.NewSQLiteSubjectGoalStore(db)
			before, err := data.NewSQLiteGoalStore(db).GetGoal(t.Context(), "owner", 1)
			if err != nil {
				t.Fatal(err)
			}
			after, err := store.ReplaceGoalSubjects(t.Context(), "owner", 1, 1, tc.ids)
			if err != nil {
				t.Fatal(err)
			}
			if after.Version != 2 || after.UpdatedAt.Before(before.UpdatedAt) {
				t.Fatalf("mutation version/time: got %+v, want version 2 and nondecreasing time", after)
			}
			live, err := data.NewSQLiteSubjectGoalStore(openSQLite(t, dsn)).ListLiveSubjectsForGoal(t.Context(), "owner", 1)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]int64, 0, len(live))
			for _, s := range live {
				got = append(got, s.SubjectID)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("live membership: got %v, want %v", got, tc.want)
			}
		})
	}
	t.Run("Goal settings updates invalidate association replacement and vice versa", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		goals := goal.NewService(data.NewSQLiteGoalStore(db))
		assignments := data.NewSQLiteSubjectGoalStore(db)
		input := goal.UpdateGoalInput{Name: "Practice", TargetSeconds: 120, Cadence: "weekly", TZ: "UTC", WeekStart: "mon"}
		if _, err := goals.Update(t.Context(), "owner", 1, 1, input); err != nil {
			t.Fatal(err)
		}
		if _, err := assignments.ReplaceGoalSubjects(t.Context(), "owner", 1, 1, []int64{1}); !errors.Is(err, data.ErrVersionConflict) {
			t.Fatalf("stale assignment: got %v, want conflict", err)
		}
		if _, err := assignments.ReplaceGoalSubjects(t.Context(), "owner", 1, 2, []int64{1}); err != nil {
			t.Fatal(err)
		}
		if _, err := goals.Update(t.Context(), "owner", 1, 2, input); !errors.Is(err, data.ErrVersionConflict) {
			t.Fatalf("stale settings: got %v, want conflict", err)
		}
	})
	t.Run("cancellation preserves membership and Goal state", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		service := goalassignment.NewService(goal.NewService(data.NewSQLiteGoalStore(db)), store)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, createErr := service.Create(ctx, "owner", goal.CreateGoalInput{Name: "Cancelled", TargetSeconds: 60}, []int64{1})
		_, replaceErr := service.ReplaceSubjects(ctx, "owner", 1, 1, []int64{1})
		_, readErr := store.ListLiveSubjectsForGoal(ctx, "owner", 1)
		for _, err := range []error{createErr, replaceErr, readErr} {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel error: got %v, want context.Canceled", err)
			}
		}
		live, err := store.ListLiveSubjectsForGoal(t.Context(), "owner", 1)
		if err != nil || len(live) != 0 {
			t.Fatalf("cancelled membership: got %+v, %v; want empty", live, err)
		}
	})
}

func TestLiveGoalSubjectsWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name, user, setup string
		id                int64
		wantErr           bool
	}{
		{"unassigned", "owner", "", 1, false}, {"inactive", "owner", "", 2, false},
		{"missing", "owner", "", 99, true}, {"foreign", "owner", "", 3, true},
		{"missing owner", "missing", "", 1, true},
		{"deleted owner", "owner", "UPDATE users SET deleted_at=updated_at WHERE user_id='owner'", 1, true},
		{"deleted Goal", "owner", "UPDATE goals SET deleted_at=updated_at WHERE goal_id=1", 1, true},
	} {
		t.Run("reads "+tc.name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedSubjectGoals(t, db)
			if tc.setup != "" {
				if _, err := db.Exec(tc.setup); err != nil {
					t.Fatal(err)
				}
			}
			store := data.NewSQLiteSubjectGoalStore(db)
			live, err := store.ListLiveSubjectsForGoal(t.Context(), tc.user, tc.id)
			if tc.wantErr {
				if !errors.Is(err, data.ErrRecordNotFound) {
					t.Fatalf("read error: got %v, want not found", err)
				}
			} else if err != nil || live == nil || len(live) != 0 {
				t.Fatalf("unassigned read: got %+v, %v; want nonnil empty", live, err)
			}
			_, err = store.ReplaceGoalSubjects(t.Context(), tc.user, tc.id, 1, nil)
			if tc.wantErr && !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("replace error: got %v, want not found", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("valid empty replacement: got %v, want nil", err)
			}
		})
	}
}

func TestGoalAssignmentConcurrencyWorkflow_SQLite(t *testing.T) {
	t.Run("only one replacement can commit against the same Goal version", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		store := data.NewSQLiteSubjectGoalStore(db)
		type replacement struct {
			goal *data.Goal
			err  error
		}
		replies := make(chan replacement, 2)
		start := make(chan struct{})
		for _, subjectID := range []int64{1, 2} {
			go func() {
				<-start
				item, err := store.ReplaceGoalSubjects(t.Context(), "owner", 1, 1, []int64{subjectID})
				replies <- replacement{item, err}
			}()
		}
		close(start)
		successes, conflicts := 0, 0
		for range 2 {
			reply := <-replies
			if reply.err == nil {
				successes++
				if reply.goal.Version != 2 {
					t.Errorf("committed version: got %d, want 2", reply.goal.Version)
				}
			} else if errors.Is(reply.err, data.ErrVersionConflict) {
				conflicts++
			} else {
				t.Errorf("concurrent replace: got %v, want success or version conflict", reply.err)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("concurrent outcomes: got %d success, %d conflict; want 1 each", successes, conflicts)
		}
		live, err := store.ListLiveSubjectsForGoal(t.Context(), "owner", 1)
		if err != nil || len(live) != 1 {
			t.Fatalf("concurrent membership: got %+v, %v; want one complete selection", live, err)
		}
	})
	for _, operation := range []string{"add", "remove"} {
		t.Run("standalone "+operation+" rolls back its link when version advancement fails", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedSubjectGoals(t, db)
			if operation == "remove" {
				if _, err := db.Exec("INSERT INTO subject_goals VALUES(1,1,'owner')"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(`CREATE TEMP TRIGGER fail_goal_version BEFORE UPDATE OF version ON goals
    BEGIN SELECT RAISE(ABORT,'controlled version failure'); END;`); err != nil {
				t.Fatal(err)
			}
			store := data.NewSQLiteSubjectGoalStore(db)
			var err error
			if operation == "add" {
				err = store.AddSubjectGoal(t.Context(), "owner", 1, 1)
			} else {
				err = store.RemoveSubjectGoal(t.Context(), "owner", 1, 1)
			}
			if err == nil {
				t.Fatal("standalone mutation: got nil error, want forced failure")
			}
			live, err := store.ListLiveSubjectsForGoal(t.Context(), "owner", 1)
			wantCount := 0
			if operation == "remove" {
				wantCount = 1
			}
			if err != nil || len(live) != wantCount {
				t.Fatalf("rolled back membership: got %+v, %v; want %d subjects", live, err, wantCount)
			}
		})
	}
}
