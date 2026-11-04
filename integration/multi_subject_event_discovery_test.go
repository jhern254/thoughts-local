//go:build integration

package integration_test

import (
	"reflect"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/goaldiscovery"
	"github.com/jhern254/go-thoughts/internal/subjectgoal"
)

func TestMultiSubjectEventDiscovery_SQLite(t *testing.T) {
	t.Run("discovers the union once per Goal and never records progress for Misc", func(t *testing.T) {
		db, events := multiSubjectEventFixture(t)
		goals := goal.NewService(data.NewSQLiteGoalStore(db))
		linkStore := data.NewSQLiteSubjectGoalStore(db)
		links := subjectgoal.NewService(linkStore)
		high, err := goals.Create(t.Context(), "u", goal.CreateGoalInput{Name: "Shared", TargetSeconds: 3600, Priority: "high"})
		if err != nil {
			t.Fatal(err)
		}
		normal, err := goals.Create(t.Context(), "u", goal.CreateGoalInput{Name: "Research", TargetSeconds: 3600})
		if err != nil {
			t.Fatal(err)
		}
		inactive := false
		hidden, err := goals.Create(t.Context(), "u", goal.CreateGoalInput{Name: "Inactive", TargetSeconds: 3600, Priority: "high", IsActive: &inactive})
		if err != nil {
			t.Fatal(err)
		}
		for _, link := range []struct{ subject, goal int64 }{{1, high.GoalID}, {2, high.GoalID}, {2, normal.GoalID}, {1, hidden.GoalID}} {
			if err := links.Add(t.Context(), "u", link.subject, link.goal); err != nil {
				t.Fatal(err)
			}
		}
		item, err := events.CreatePast(t.Context(), "u", "Work", eventTime(100), eventTime(200), []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		earnedSupport := eventSupportRows(t, db)
		if len(earnedSupport) != 3 {
			t.Fatalf("completed Event support: got %v, want three support rows", earnedSupport)
		}
		discovery := goaldiscovery.NewService(events, linkStore, goaldiscovery.AllCandidatesRecommender{})
		result, err := discovery.DiscoverForEvent(t.Context(), "u", item.EventID, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.PriorityGoals) != 1 || result.PriorityGoals[0].GoalID != high.GoalID || len(result.RecommendedGoals) != 1 || result.RecommendedGoals[0].GoalID != normal.GoalID {
			t.Fatalf("discovery got %+v; want shared Goal once and research Goal", result)
		}
		misc, err := events.CreatePast(t.Context(), "u", "Unassigned", eventTime(300), eventTime(400), nil)
		if err != nil {
			t.Fatal(err)
		}
		result, err = discovery.DiscoverForEvent(t.Context(), "u", misc.EventID, true)
		if err != nil || len(result.PriorityGoals) != 0 || len(result.RecommendedGoals) != 0 {
			t.Fatalf("Misc candidates got %+v,%v; want empty", result, err)
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM goal_progress WHERE event_id = ?`, misc.EventID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("Misc Event support: got %d, want zero", count)
		}
		if after := eventSupportRows(t, db); !reflect.DeepEqual(after, earnedSupport) {
			t.Fatalf("earned support after discovery and Misc creation: got %v, want %v", after, earnedSupport)
		}
	})
}
