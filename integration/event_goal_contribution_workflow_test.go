//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
)

func assertEarnedSeconds(t *testing.T, db *sql.DB, goalID, want int64) {
	t.Helper()
	got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", goalID)
	if err != nil || got.LifetimeEarnedSeconds != want {
		t.Fatalf("Goal %d earned seconds: got %+v,%v, want %d", goalID, got, err, want)
	}
}

// Compare contribution IDs and their Event, Goal and Subject references.
// Checking only the total seconds would miss an edit that unnecessarily deleted
// and recreated these rows.
func readEventGoalContributions(t *testing.T, db *sql.DB) [][5]int64 {
	t.Helper()
	rows, err := db.Query(`SELECT progress_id,event_id,goal_id,recorded_subject_id,COALESCE(subject_id,0) FROM goal_progress ORDER BY progress_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	contributions := make([][5]int64, 0)
	for rows.Next() {
		var row [5]int64
		if err := rows.Scan(&row[0], &row[1], &row[2], &row[3], &row[4]); err != nil {
			t.Fatal(err)
		}
		contributions = append(contributions, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return contributions
}

func TestEventGoalContributionWorkflow_SQLite(t *testing.T) {
	t.Run("label and time corrections use current Event intervals without rewriting contributions", func(t *testing.T) {
		db, service := attributionFixture(t)
		item, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		before := readEventGoalContributions(t, db)
		// Reject contribution writes while editing only an Event's label or time.
		// Those edits should change earned time through the Event record alone,
		// without replacing the connections that explain which Goals earned it.
		if _, err := db.Exec(`
    CREATE TRIGGER reject_contribution_update BEFORE UPDATE ON goal_progress BEGIN SELECT RAISE(ABORT,'unexpected contribution rewrite'); END;
    CREATE TRIGGER reject_contribution_delete BEFORE DELETE ON goal_progress BEGIN SELECT RAISE(ABORT,'unexpected contribution rewrite'); END;
    CREATE TRIGGER reject_contribution_insert BEFORE INSERT ON goal_progress BEGIN SELECT RAISE(ABORT,'unexpected contribution rewrite'); END;`); err != nil {
			t.Fatal(err)
		}
		for _, cut := range []struct{ start, end int64 }{{100, 200}, {100, 150}, {50, 200}, {75, 225}, {75, 75}} {
			item, err = service.Update(t.Context(), "owner", item.EventID, item.Version, event.UpdateInput{Activity: "Changed", StartedAt: time.Unix(cut.start, 0), EndedAt: timePointer(cut.end), SubjectIDs: []int64{2, 1}})
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []int64{1, 2} {
				assertEarnedSeconds(t, db, id, cut.end-cut.start)
			}
			if after := readEventGoalContributions(t, db); !reflect.DeepEqual(after, before) {
				t.Fatalf("contribution after interval edit: got %v, want %v", after, before)
			}
		}
	})
	t.Run("explicit additions discover only new contributions and removals retract only that Subject", func(t *testing.T) {
		db, service := attributionFixture(t)
		item, err := service.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
    INSERT INTO goals (goal_id,user_id,goal_name,target_seconds) VALUES (4,'owner','Later Goal',60);
    INSERT INTO subject_goals (subject_id,goal_id,user_id,goal_number) VALUES (1,4,'owner',3),(2,4,'owner',3);
    UPDATE subject_goals SET deleted_at = 1 WHERE goal_id = 1;`); err != nil {
			t.Fatal(err)
		}
		assertEarnedSeconds(t, db, 4, 0)
		for _, tc := range []struct {
			ids                  []int64
			first, second, later int64
		}{
			{[]int64{1}, 100, 0, 0},
			{[]int64{1, 2}, 100, 100, 100},
			{[]int64{2}, 0, 100, 100},
			{nil, 0, 0, 0},
		} {
			item, err = service.Update(t.Context(), "owner", item.EventID, item.Version, event.UpdateInput{Activity: "Corrected", StartedAt: item.StartedAt, EndedAt: item.EndedAt, SubjectIDs: tc.ids})
			if err != nil {
				t.Fatal(err)
			}
			assertEarnedSeconds(t, db, 1, tc.first)
			assertEarnedSeconds(t, db, 2, tc.second)
			assertEarnedSeconds(t, db, 4, tc.later)
		}
	})
	t.Run("ongoing Events never establish contributions and stale completion cannot duplicate it", func(t *testing.T) {
		db, service := attributionFixture(t)
		item, err := service.Create(t.Context(), "owner", "Work", time.Unix(100, 0), []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		if got := readEventGoalContributions(t, db); len(got) != 0 {
			t.Fatalf("ongoing contribution: got %v, want empty", got)
		}
		if _, err := service.End(t.Context(), "owner", item.EventID, item.Version, time.Unix(200, 0)); err != nil {
			t.Fatal(err)
		}
		before := readEventGoalContributions(t, db)
		if _, err := service.End(t.Context(), "owner", item.EventID, item.Version, time.Unix(200, 0)); !errors.Is(err, data.ErrEventVersionConflict) {
			t.Fatalf("stale completion: got %v, want version conflict", err)
		}
		if after := readEventGoalContributions(t, db); !reflect.DeepEqual(after, before) {
			t.Fatalf("stale contribution: got %v, want %v", after, before)
		}
	})
	t.Run("a canceled summary returns no partial total", func(t *testing.T) {
		db, _ := attributionFixture(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(ctx, "owner", 1)
		if !errors.Is(err, context.Canceled) || got != (data.GoalProgressSummary{}) {
			t.Fatalf("canceled summary: got %+v,%v, want zero and canceled", got, err)
		}
	})
	t.Run("rejects an interval whose elapsed seconds exceed int64", func(t *testing.T) {
		db, _ := attributionFixture(t)
		if _, err := db.Exec(`
    INSERT INTO events (event_id,user_id,started_at,ended_at) VALUES (1,'owner',-9223372036854775808,9223372036854775807);
    INSERT INTO goal_progress (event_id,goal_id,user_id,recorded_subject_id,subject_id) VALUES (1,1,'owner',1,1);`); err != nil {
			t.Fatal(err)
		}
		got, err := data.NewSQLiteGoalProgressStore(db).GetGoalProgressSummary(t.Context(), "owner", 1)
		if !errors.Is(err, data.ErrInvalidEventInterval) || got != (data.GoalProgressSummary{}) {
			t.Fatalf("invalid interval: got %+v,%v, want zero and interval error", got, err)
		}
	})
}
