//go:build integration

package integration_test

import (
	"os"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestGoalStressDemoWorkflow_SQLite(t *testing.T) {
	t.Run("supplies varied Goals and enough live Subjects for scrolling", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		if _, err := db.Exec(`CREATE TEMP TABLE demo_clock AS SELECT 1789151820 AS at`); err != nil {
			t.Fatal(err)
		}
		seed, err := os.ReadFile("../scripts/demo_stress.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(seed)); err != nil {
			t.Fatal(err)
		}
		var goals, active, inactive, subjects, cadences, priorities int
		if err := db.QueryRow(`SELECT (SELECT count(*) FROM goals),(SELECT count(*) FROM goals WHERE goal_is_active=1),
 (SELECT count(*) FROM goals WHERE goal_is_active=0),(SELECT count(*) FROM subjects WHERE deleted_at IS NULL),
 (SELECT count(DISTINCT cadence) FROM goals),(SELECT count(DISTINCT priority) FROM goals)`).Scan(&goals, &active, &inactive, &subjects, &cadences, &priorities); err != nil {
			t.Fatal(err)
		}
		if got, want := [6]int{goals, active, inactive, subjects, cadences, priorities}, [6]int{48, 36, 12, 32, 6, 3}; got != want {
			t.Fatalf("Goals/active/inactive/Subjects/cadences/priorities got %v, want %v", got, want)
		}
		store := data.NewSQLiteSubjectGoalStore(db)
		for _, tc := range []struct {
			id   int64
			want int
		}{{1, 32}, {47, 0}, {48, 0}} {
			live, err := store.ListLiveSubjectsForGoal(t.Context(), "demo", tc.id)
			if err != nil || len(live) != tc.want {
				t.Fatalf("Goal %d live Subjects got %d, %v; want %d", tc.id, len(live), err, tc.want)
			}
		}
		raw, err := store.ListGoalSubjects(t.Context(), "demo", 47)
		if err != nil || len(raw) != 1 {
			t.Fatalf("retained historical links got %+v, %v; want one", raw, err)
		}
		rows, err := db.Query("PRAGMA foreign_key_check")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Fatal("foreign_key_check returned a violation")
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	})
}
