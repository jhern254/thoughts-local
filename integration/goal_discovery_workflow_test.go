//go:build integration

package integration_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/goaldiscovery"
)

func TestGoalDiscoveryWorkflow_SQLite(t *testing.T) {
	t.Run("candidates are active visible linked goals in priority and ID order", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		_, err := db.Exec(`INSERT INTO goals(goal_id,user_id,goal_name,target_seconds,priority) VALUES
 (4,'owner','High first',60,'high'),(5,'owner','Low',60,'low'),(6,'owner','High second',60,'high'),
 (7,'owner','Deleted',60,'high'),(8,'owner','Unlinked',60,'high');
 UPDATE goals SET deleted_at=updated_at WHERE goal_id=7;
 UPDATE goals SET goal_start_date='2099-01-01',goal_end_date='2099-12-31' WHERE goal_id=4;
 INSERT INTO subject_goals(subject_id,goal_id,user_id) VALUES
 (1,6,'owner'),(1,5,'owner'),(1,4,'owner'),(1,2,'owner'),(1,1,'owner'),(1,7,'owner'),(3,3,'other');`)
		if err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteSubjectGoalStore(db)
		got, err := store.ListActiveGoalsForSubject(t.Context(), "owner", 1)
		if err != nil {
			t.Fatal(err)
		}
		want := []data.Goal{}
		for _, id := range []int64{4, 6, 1, 5} {
			item, err := data.NewSQLiteGoalStore(db).GetGoal(t.Context(), "owner", id)
			if err != nil {
				t.Fatal(err)
			}
			want = append(want, *item)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("candidates: got %+v, want %+v", got, want)
		}
		retained, err := store.ListSubjectGoals(t.Context(), "owner", 1)
		if err != nil || len(retained) != 6 {
			t.Fatalf("retained links: got %+v, %v; want six including inactive and deleted goals", retained, err)
		}
	})

	for _, tc := range []struct {
		name, setup string
		subject     int64
	}{
		{"foreign subject", "", 3},
		{"missing subject", "", 99},
		{"unlinked subject", "", 2},
		{"deleted subject", "UPDATE subjects SET deleted_at=updated_at WHERE subject_id=1", 1},
		{"deleted owner", "UPDATE users SET deleted_at=updated_at WHERE user_id='owner'", 1},
	} {
		t.Run("no candidates for "+tc.name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedSubjectGoals(t, db)
			if _, err := db.Exec("INSERT INTO subject_goals VALUES (1,1,'owner'),(3,3,'other')"); err != nil {
				t.Fatal(err)
			}
			if tc.setup != "" {
				if _, err := db.Exec(tc.setup); err != nil {
					t.Fatal(err)
				}
			}
			got, err := data.NewSQLiteSubjectGoalStore(db).ListActiveGoalsForSubject(t.Context(), "owner", tc.subject)
			if err != nil || got == nil || len(got) != 0 {
				t.Fatalf("candidates: got %+v, %v; want non-nil empty, nil", got, err)
			}
		})
	}

	t.Run("runtime manages links and discovers current goals for a completed event without recording progress", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := runtime.Close(); err != nil {
				t.Error(err)
			}
		})
		owner := runtime.LocalUser().UserID
		subject, err := runtime.Subjects().Create(t.Context(), owner, "Coding")
		if err != nil {
			t.Fatal(err)
		}
		high, err := runtime.Goals().Create(t.Context(), owner, goal.CreateGoalInput{Name: "Practice", TargetSeconds: 60, Priority: "high"})
		if err != nil {
			t.Fatal(err)
		}
		normal, err := runtime.Goals().Create(t.Context(), owner, goal.CreateGoalInput{Name: "Project", TargetSeconds: 60})
		if err != nil {
			t.Fatal(err)
		}
		links := runtime.SubjectGoals()
		for _, id := range []int64{high.GoalID, normal.GoalID} {
			if err := links.Add(t.Context(), owner, subject.SubjectID, id); err != nil {
				t.Fatal(err)
			}
		}
		if err := links.Add(t.Context(), owner, subject.SubjectID, high.GoalID); !errors.Is(err, data.ErrDuplicateRecord) {
			t.Fatalf("duplicate link: got %v, want ErrDuplicateRecord", err)
		}
		bySubject, err := links.ListForSubject(t.Context(), owner, subject.SubjectID)
		wantLinks := []data.SubjectGoal{{SubjectID: subject.SubjectID, GoalID: high.GoalID, UserID: owner}, {SubjectID: subject.SubjectID, GoalID: normal.GoalID, UserID: owner}}
		if err != nil || !reflect.DeepEqual(bySubject, wantLinks) {
			t.Fatalf("subject links: got %+v, %v; want %+v, nil", bySubject, err, wantLinks)
		}
		byGoal, err := links.ListForGoal(t.Context(), owner, high.GoalID)
		if err != nil || !reflect.DeepEqual(byGoal, wantLinks[:1]) {
			t.Fatalf("goal links: got %+v, %v; want %+v, nil", byGoal, err, wantLinks[:1])
		}
		now := time.Now().UTC().Truncate(time.Second)
		item, err := runtime.Events().CreatePast(t.Context(), owner, "Coding", now.Add(-2*time.Hour), now.Add(-time.Hour), &subject.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		for _, enabled := range []bool{false, true} {
			got, err := runtime.GoalDiscovery().DiscoverForEvent(t.Context(), owner, item.EventID, enabled)
			want := goaldiscovery.Result{PriorityGoals: []data.Goal{*high}, RecommendedGoals: []data.Goal{}}
			if enabled {
				want.RecommendedGoals = []data.Goal{*normal}
			}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("discovery enabled=%t: got %+v, %v; want %+v, nil", enabled, got, err, want)
			}
		}
		if err := links.Remove(t.Context(), owner, subject.SubjectID, high.GoalID); err != nil {
			t.Fatal(err)
		}
		inactive := false
		if _, err := runtime.Goals().Update(t.Context(), owner, normal.GoalID, normal.Version, goal.UpdateGoalInput{
			Name: normal.GoalName, TargetSeconds: normal.TargetSeconds, IsActive: &inactive,
			Cadence: normal.Cadence, TZ: normal.TZ, WeekStart: normal.WeekStart,
		}); err != nil {
			t.Fatal(err)
		}
		got, err := runtime.GoalDiscovery().DiscoverForEvent(t.Context(), owner, item.EventID, true)
		want := goaldiscovery.Result{PriorityGoals: []data.Goal{}, RecommendedGoals: []data.Goal{}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("discovery after link and flag changes: got %+v, %v; want %+v, nil", got, err, want)
		}
		unchanged, err := runtime.Events().Get(t.Context(), owner, item.EventID)
		if err != nil || !reflect.DeepEqual(unchanged, item) {
			t.Fatalf("event after discovery: got %+v, %v; want %+v, nil", unchanged, err, item)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM goal_progress").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("progress rows: got %d, want 0", count)
		}
		for _, request := range []struct {
			user  string
			event int64
		}{{"missing", item.EventID}, {owner, item.EventID + 1}} {
			if _, err := runtime.GoalDiscovery().DiscoverForEvent(t.Context(), request.user, request.event, true); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("unavailable event: got %v, want ErrRecordNotFound", err)
			}
		}
	})
}
