//go:build integration

package integration_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/goalassignment"
	"github.com/jhern254/go-thoughts/internal/goaldiscovery"
)

func TestGoalAssignmentDeletionWorkflow_SQLite(t *testing.T) {
	t.Run("deleting a Subject preserves seven Goals and their histories", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		links := data.NewSQLiteSubjectGoalStore(db)
		goals := data.NewSQLiteGoalStore(db)
		service := goalassignment.NewService(goal.NewService(goals), links)
		before := make([]*data.Goal, 0, 7)
		priorities := []string{"low", "normal", "high"}
		for i := range 7 {
			active := i%2 == 0
			ids := []int64{1}
			if i == 6 {
				ids = append(ids, 2)
			}
			item, err := service.Create(t.Context(), "owner", goal.CreateGoalInput{
				Name: fmt.Sprintf("Goal %d", i), TargetSeconds: 300 + int64(i), Priority: priorities[i%3], IsActive: &active,
				StartDate: "2026-01-01", EndDate: "2026-12-31", Cadence: "monthly", TZ: "America/Los_Angeles", WeekStart: "sun", DefaultCadence: "weekly",
			}, ids)
			if err != nil {
				t.Fatal(err)
			}
			before = append(before, item)
		}
		if _, err := db.Exec(`INSERT INTO events(event_id,user_id,subject_id,started_at,ended_at) VALUES(1,'owner',1,100,200);
   INSERT INTO thoughts(thought_id,user_id,subject_id,event_id,thought) VALUES(1,'owner',1,1,'Synthetic');`); err != nil {
			t.Fatal(err)
		}
		progress := data.NewSQLiteGoalProgressStore(db)
		contributions := make([]*data.GoalProgress, 0, 7)
		for _, item := range before {
			contribution := progressRecord()
			contribution.GoalID = item.GoalID
			id := int64(1)
			contribution.EventID = &id
			contribution.ThoughtID = &id
			saved, err := progress.CreateGoalProgress(t.Context(), contribution)
			if err != nil {
				t.Fatal(err)
			}
			contributions = append(contributions, saved)
		}
		parents := data.NewSQLiteGoalParentStore(db)
		if err := parents.AddGoalParent(t.Context(), "owner", before[1].GoalID, before[0].GoalID); err != nil {
			t.Fatal(err)
		}
		parentLinks, err := parents.ListGoalParents(t.Context(), "owner", before[1].GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if err := data.NewSQLiteSubjectStore(db).DeleteSubject(t.Context(), "owner", 1); err != nil {
			t.Fatal(err)
		}
		for i, item := range before {
			after, err := goals.GetGoal(t.Context(), "owner", item.GoalID)
			if err != nil || !reflect.DeepEqual(after, item) {
				t.Fatalf("Goal %d after deletion: got %+v, %v; want %+v", item.GoalID, after, err, item)
			}
			live, err := links.ListLiveSubjectsForGoal(t.Context(), "owner", item.GoalID)
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 0
			if i == 6 {
				wantCount = 1
			}
			if live == nil || len(live) != wantCount || (i == 6 && live[0].SubjectID != 2) {
				t.Fatalf("Goal %d live subjects: got %+v, want %d remaining live subjects", item.GoalID, live, wantCount)
			}
			raw, err := links.ListGoalSubjects(t.Context(), "owner", item.GoalID)
			if err != nil || len(raw) != wantCount+1 {
				t.Fatalf("Goal %d history: got %+v, %v; want retained links", item.GoalID, raw, err)
			}
			saved, err := progress.GetGoalProgress(t.Context(), "owner", contributions[i].ProgressID)
			if err != nil || !reflect.DeepEqual(saved, contributions[i]) {
				t.Fatalf("progress after deletion: got %+v, %v; want %+v", saved, err, contributions[i])
			}
		}
		afterParents, err := parents.ListGoalParents(t.Context(), "owner", before[1].GoalID)
		if err != nil || !reflect.DeepEqual(afterParents, parentLinks) {
			t.Fatalf("parents after deletion: got %+v, %v; want %+v", afterParents, err, parentLinks)
		}
		var eventSubject, thoughtSubject *int64
		if err := db.QueryRow("SELECT e.subject_id,t.subject_id FROM events e JOIN thoughts t ON t.event_id=e.event_id WHERE e.event_id=1").Scan(&eventSubject, &thoughtSubject); err != nil {
			t.Fatal(err)
		}
		if eventSubject != nil || thoughtSubject != nil {
			t.Fatalf("unlinked subjects: got %v, %v; want NULL, NULL", eventSubject, thoughtSubject)
		}
		rows, err := db.Query("PRAGMA foreign_key_check")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Fatal("foreign_key_check: got violation, want none")
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("Misc Events never discover Unassigned Goals while literal names remain ordinary", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		if _, err := db.Exec(`UPDATE subjects SET subject_name='Misc' WHERE subject_id=1;
   UPDATE subjects SET subject_name='Unassigned' WHERE subject_id=2;
   INSERT INTO subject_goals VALUES(1,1,'owner'),(2,2,'owner');
   INSERT INTO events(event_id,user_id,subject_id,started_at,ended_at) VALUES(1,'owner',NULL,100,200),(2,'owner',1,300,400);`); err != nil {
			t.Fatal(err)
		}
		links := data.NewSQLiteSubjectGoalStore(db)
		discovery := goaldiscovery.NewService(event.NewService(data.NewSQLiteEventStore(db)), links, goaldiscovery.AllCandidatesRecommender{})
		got, err := discovery.DiscoverForEvent(t.Context(), "owner", 1, true)
		if err != nil || len(got.PriorityGoals) != 0 || len(got.RecommendedGoals) != 0 {
			t.Fatalf("Misc discovery: got %+v, %v; want no Goals", got, err)
		}
		got, err = discovery.DiscoverForEvent(t.Context(), "owner", 2, true)
		if err != nil || len(got.RecommendedGoals) != 1 || got.RecommendedGoals[0].GoalID != 1 {
			t.Fatalf("literal Misc discovery: got %+v, %v; want Goal 1", got, err)
		}
		for _, id := range []int64{1, 2} {
			live, err := links.ListLiveSubjectsForGoal(t.Context(), "owner", id)
			if err != nil || len(live) != 1 {
				t.Fatalf("literal Subject membership: got %+v, %v; want one ordinary Subject", live, err)
			}
		}
	})
	t.Run("runtime exposes atomic assignment and live membership", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer runtime.Close()
		owner := runtime.LocalUser().UserID
		item, err := runtime.GoalAssignments().Create(t.Context(), owner, goal.CreateGoalInput{Name: "Unassigned", TargetSeconds: 60}, nil)
		if err != nil {
			t.Fatal(err)
		}
		live, err := runtime.SubjectGoals().ListLiveSubjectsForGoal(t.Context(), owner, item.GoalID)
		if err != nil || live == nil || len(live) != 0 {
			t.Fatalf("runtime Unassigned: got %+v, %v; want nonnil empty", live, err)
		}
	})
}
