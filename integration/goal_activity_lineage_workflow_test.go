//go:build integration

package integration_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
)

func TestGoalActivityLineageWorkflow_SQLite(t *testing.T) {
	t.Run("shows earned Events once and only Thoughts from contributing Subjects within the interval", func(t *testing.T) {
		db, events := attributionFixture(t)
		item, err := events.CreatePast(t.Context(), "owner", "Building", time.Unix(100, 0), time.Unix(200, 0), []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(`
            INSERT INTO thoughts (thought_id,user_id,subject_id,thought,observed_at,created_at,updated_at) VALUES
                (1,'owner',1,'At start',100,300,300),
                (2,'owner',2,'During',150,300,300),
                (3,'owner',NULL,'Misc',150,300,300),
                (4,'owner',1,'At end',200,300,300),
                (5,'owner',1,'Before',99,300,300);
            INSERT INTO goals (goal_id,user_id,goal_name,target_seconds) VALUES (4,'owner','Parent',60);
            INSERT INTO goal_parents (goal_id,parent_goal_id,user_id) VALUES (1,4,'owner'),(2,4,'owner');
        `)
		if err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteGoalProgressStore(db)
		for _, goalID := range []int64{1, 4} {
			page, err := store.ListEarnedEvents(t.Context(), "owner", goalID, data.GoalEventPageRequest{})
			if err != nil || len(page.Events) != 1 || page.Events[0].EventID != item.EventID {
				t.Fatalf("earned Events: got %+v,%v, want one Event %d", page, err, item.EventID)
			}
			thoughts, err := store.ListEventLineageThoughts(t.Context(), "owner", goalID, item.EventID, data.ThoughtSummaryViewRequest{})
			if err != nil {
				t.Fatal(err)
			}
			ids := []int64{}
			for _, thought := range thoughts.Thoughts.Items {
				ids = append(ids, thought.ThoughtID)
			}
			if !reflect.DeepEqual(ids, []int64{2, 1}) {
				t.Fatalf("lineage Thoughts: got %v, want [2 1]", ids)
			}
			full, err := store.GetEventLineageThought(t.Context(), "owner", goalID, item.EventID, 1)
			if err != nil || full.Thought != "At start" || full.EventID != nil {
				t.Fatalf("time-associated Thought: got %+v,%v, want At start without EventID", full, err)
			}
			for _, id := range []int64{3, 4, 5} {
				if _, err := store.GetEventLineageThought(t.Context(), "owner", goalID, item.EventID, id); !errors.Is(err, data.ErrRecordNotFound) {
					t.Fatalf("excluded Thought %d: got %v, want not found", id, err)
				}
			}
		}
	})
	t.Run("distinguishes unavailable Goals and Events from valid empty history", func(t *testing.T) {
		db, _ := attributionFixture(t)
		store := data.NewSQLiteGoalProgressStore(db)
		page, err := store.ListEarnedEvents(t.Context(), "owner", 1, data.GoalEventPageRequest{})
		if err != nil || page.Events == nil || len(page.Events) != 0 {
			t.Fatalf("empty history: got %+v,%v, want non-nil empty page", page, err)
		}
		for _, user := range []string{"owner", "other"} {
			if _, err := store.ListEarnedEvents(t.Context(), user, 999, data.GoalEventPageRequest{}); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("missing Goal: got %v, want not found", err)
			}
			if _, err := store.ListEventLineageThoughts(t.Context(), user, 1, 999, data.ThoughtSummaryViewRequest{}); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("missing Event: got %v, want not found", err)
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := store.ListEarnedEvents(ctx, "owner", 1, data.GoalEventPageRequest{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled read: got %v, want canceled", err)
		}
	})
}

func TestGoalActivityLineageRetentionWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name, change             string
		wantEvents, wantThoughts int
	}{
		{"inactive Goals keep earned activity", `UPDATE goals SET goal_is_active=0 WHERE goal_id=1`, 1, 1},
		{"retired Goal membership keeps earned activity", `UPDATE subject_goals SET deleted_at=1 WHERE goal_id=1`, 1, 1},
		{"retired Event membership does not rediscover eligibility", `UPDATE event_subjects SET deleted_at=1`, 1, 1},
		{"soft-deleted Thoughts disappear", `UPDATE thoughts SET deleted_at=updated_at`, 1, 0},
		{"soft-deleted Events disappear", `UPDATE events SET deleted_at=updated_at`, 0, 0},
		{"a deleted Subject retains Event time but cannot match Thoughts", `UPDATE subjects SET deleted_at=updated_at WHERE subject_id=1`, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, events := attributionFixture(t)
			item, err := events.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO thoughts (user_id,subject_id,thought,observed_at) VALUES ('owner',1,'During',150)`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.change); err != nil {
				t.Fatal(err)
			}
			store := data.NewSQLiteGoalProgressStore(db)
			page, err := store.ListEarnedEvents(t.Context(), "owner", 1, data.GoalEventPageRequest{})
			if err != nil || len(page.Events) != tc.wantEvents {
				t.Fatalf("Events: got %+v,%v, want %d", page, err, tc.wantEvents)
			}
			thoughts, err := store.ListEventLineageThoughts(t.Context(), "owner", 1, item.EventID, data.ThoughtSummaryViewRequest{})
			if tc.wantEvents == 0 {
				if !errors.Is(err, data.ErrRecordNotFound) {
					t.Fatalf("deleted Event: got %v, want not found", err)
				}
				return
			}
			if err != nil || len(thoughts.Thoughts.Items) != tc.wantThoughts {
				t.Fatalf("Thoughts: got %+v,%v, want %d", thoughts, err, tc.wantThoughts)
			}
		})
	}
	t.Run("a new Subject reusing a hard-deleted identity cannot match old contributions", func(t *testing.T) {
		db, events := attributionFixture(t)
		item, err := events.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
            DELETE FROM subjects WHERE subject_id=1;
            INSERT INTO subjects (subject_id,user_id,subject_name) VALUES (1,'owner','New identity');
            INSERT INTO thoughts (thought_id,user_id,subject_id,thought,observed_at) VALUES (10,'owner',1,'New Subject',150);
        `); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteGoalProgressStore(db)
		page, err := store.ListEventLineageThoughts(t.Context(), "owner", 1, item.EventID, data.ThoughtSummaryViewRequest{})
		if err != nil || len(page.Thoughts.Items) != 0 {
			t.Fatalf("detached history: got %+v,%v, want no Thoughts", page, err)
		}
		if _, err := store.GetEventLineageThought(t.Context(), "owner", 1, item.EventID, 10); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("reused identity read: got %v, want not found", err)
		}
	})
	t.Run("current parent paths include deleted descendants once and removing the last path removes history", func(t *testing.T) {
		db, events := attributionFixture(t)
		if _, err := db.Exec(`
            INSERT INTO goals (goal_id,user_id,goal_name,target_seconds) VALUES
                (4,'owner','Parent A',60),(5,'owner','Parent B',60),(6,'owner','Root',60);
            INSERT INTO goal_parents (goal_id,parent_goal_id,user_id) VALUES
                (1,4,'owner'),(1,5,'owner'),(4,6,'owner'),(5,6,'owner');
        `); err != nil {
			t.Fatal(err)
		}
		for _, start := range []int64{100, 200} {
			if _, err := events.CreatePast(t.Context(), "owner", "Equal durations", time.Unix(start, 0), time.Unix(start+100, 0), []int64{1, 2}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.Exec(`UPDATE goals SET deleted_at=updated_at,goal_is_active=0 WHERE goal_id=1`); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteGoalProgressStore(db)
		for step, change := range []string{"", `DELETE FROM goal_parents WHERE parent_goal_id=5`, `DELETE FROM goal_parents WHERE parent_goal_id=4`} {
			if change != "" {
				if _, err := db.Exec(change); err != nil {
					t.Fatal(err)
				}
			}
			want := 2
			if step == 2 {
				want = 0
			}
			page, err := store.ListEarnedEvents(t.Context(), "owner", 6, data.GoalEventPageRequest{})
			if err != nil || len(page.Events) != want {
				t.Fatalf("parent history step %d: got %+v,%v, want %d", step, page, err, want)
			}
		}
	})
	t.Run("explicit Subject corrections change matching Thoughts while time corrections use the current interval", func(t *testing.T) {
		db, events := attributionFixture(t)
		item, err := events.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO thoughts (thought_id,user_id,subject_id,thought,observed_at) VALUES
            (1,'owner',1,'Coding',120),(2,'owner',2,'Learning',150)`); err != nil {
			t.Fatal(err)
		}
		item, err = events.Update(t.Context(), "owner", item.EventID, item.Version, event.UpdateInput{Activity: "Changed", StartedAt: item.StartedAt, EndedAt: timePointer(140), SubjectIDs: []int64{1}})
		if err != nil {
			t.Fatal(err)
		}
		page, err := data.NewSQLiteGoalProgressStore(db).ListEventLineageThoughts(t.Context(), "owner", 1, item.EventID, data.ThoughtSummaryViewRequest{})
		if err != nil || page.Event.ElapsedSeconds != 40 || len(page.Thoughts.Items) != 1 || page.Thoughts.Items[0].ThoughtID != 1 {
			t.Fatalf("corrected lineage: got %+v,%v, want 40s and Thought 1", page, err)
		}
	})
}

func TestGoalActivityLineagePaginationWorkflow_SQLite(t *testing.T) {
	t.Run("pages tied Event timestamps in both directions without duplicates", func(t *testing.T) {
		db, events := attributionFixture(t)
		total := data.GoalEventPageSize*2 + 3
		for i := 0; i < total; i++ {
			if _, err := events.CreatePast(t.Context(), "owner", "Zero", time.Unix(100, 0), time.Unix(100, 0), []int64{1}); err != nil {
				t.Fatal(err)
			}
		}
		store := data.NewSQLiteGoalProgressStore(db)
		request := data.GoalEventPageRequest{}
		seen := map[int64]bool{}
		var previousID int64 = 1 << 62
		for {
			page, err := store.ListEarnedEvents(t.Context(), "owner", 1, request)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range page.Events {
				if seen[item.EventID] || item.EventID >= previousID {
					t.Fatalf("Event ordering: got %d after %d", item.EventID, previousID)
				}
				seen[item.EventID] = true
				previousID = item.EventID
			}
			if !page.More {
				break
			}
			cursor := page.Events[len(page.Events)-1].Cursor()
			request.Cursor = &cursor
		}
		if len(seen) != total {
			t.Fatalf("Event count: got %d, want %d", len(seen), total)
		}
		cursor := data.GoalEventCursor{StartedAt: time.Unix(100, 0), EventID: 1}
		page, err := store.ListEarnedEvents(t.Context(), "owner", 1, data.GoalEventPageRequest{Cursor: &cursor, Direction: data.EarnedEventsNewer})
		if err != nil || len(page.Events) != data.GoalEventPageSize || page.Events[len(page.Events)-1].EventID != 2 {
			t.Fatalf("newer Events: got %+v,%v, want nearest newer IDs ending in 2", page, err)
		}
	})
	t.Run("pages tied Thought timestamps once despite several contribution paths", func(t *testing.T) {
		db, events := attributionFixture(t)
		item, err := events.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		total := data.ThoughtSummaryViewBatchSize*2 + 3
		for i := 0; i < total; i++ {
			if _, err := db.Exec(`INSERT INTO thoughts (user_id,subject_id,thought,observed_at,created_at,updated_at) VALUES ('owner',2,'Thought',150,300,300)`); err != nil {
				t.Fatal(err)
			}
		}
		store := data.NewSQLiteGoalProgressStore(db)
		request := data.ThoughtSummaryViewRequest{}
		seen := map[int64]bool{}
		for {
			page, err := store.ListEventLineageThoughts(t.Context(), "owner", 1, item.EventID, request)
			if err != nil {
				t.Fatal(err)
			}
			for _, thought := range page.Thoughts.Items {
				if seen[thought.ThoughtID] {
					t.Fatalf("duplicate Thought: %d", thought.ThoughtID)
				}
				seen[thought.ThoughtID] = true
			}
			if !page.Thoughts.More {
				break
			}
			cursor := page.Thoughts.Items[len(page.Thoughts.Items)-1].Cursor()
			request.Cursor = &cursor
		}
		if len(seen) != total {
			t.Fatalf("Thought count: got %d, want %d", len(seen), total)
		}
		cursor := data.ThoughtCursor{ObservedAt: time.Unix(150, 0), CreatedAt: time.Unix(300, 0), ThoughtID: 1}
		page, err := store.ListEventLineageThoughts(t.Context(), "owner", 1, item.EventID, data.ThoughtSummaryViewRequest{Cursor: &cursor, Direction: data.ThoughtsNewer})
		if err != nil || len(page.Thoughts.Items) != data.ThoughtSummaryViewBatchSize || page.Thoughts.Items[len(page.Thoughts.Items)-1].ThoughtID != 2 {
			t.Fatalf("newer Thoughts: got %+v,%v, want nearest newer IDs ending in 2", page, err)
		}
	})
}

func TestGoalActivityLineageAccessWorkflow_SQLite(t *testing.T) {
	t.Run("rejects foreign or deleted roots Events and Thoughts without exposing records", func(t *testing.T) {
		db, events := attributionFixture(t)
		earned, err := events.CreatePast(t.Context(), "owner", "Earned", time.Unix(100, 0), time.Unix(200, 0), []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		unearned, err := events.CreatePast(t.Context(), "owner", "Misc", time.Unix(200, 0), time.Unix(300, 0), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO thoughts (thought_id,user_id,subject_id,thought,observed_at) VALUES (1,'other',3,'Foreign',150)`); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteGoalProgressStore(db)
		for _, scope := range []struct {
			user string
			goal int64
		}{{"other", 1}, {"owner", 3}} {
			if _, err := store.ListEarnedEvents(t.Context(), scope.user, scope.goal, data.GoalEventPageRequest{}); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("foreign history: got %v, want not found", err)
			}
			if _, err := store.ListEventLineageThoughts(t.Context(), scope.user, scope.goal, earned.EventID, data.ThoughtSummaryViewRequest{}); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("foreign expansion: got %v, want not found", err)
			}
		}
		if _, err := store.ListEventLineageThoughts(t.Context(), "owner", 1, unearned.EventID, data.ThoughtSummaryViewRequest{}); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("unearned Event: got %v, want not found", err)
		}
		if _, err := store.GetEventLineageThought(t.Context(), "owner", 1, earned.EventID, 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("foreign Thought: got %v, want not found", err)
		}
		if _, err := db.Exec(`UPDATE goals SET deleted_at=updated_at WHERE goal_id=1`); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ListEarnedEvents(t.Context(), "owner", 1, data.GoalEventPageRequest{}); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("deleted root: got %v, want not found", err)
		}
		if _, err := store.ListEventLineageThoughts(t.Context(), "owner", 1, earned.EventID, data.ThoughtSummaryViewRequest{}); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("deleted root expansion: got %v, want not found", err)
		}
	})
	t.Run("reopened history includes zero-duration Events with an empty non-nil Thought page", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		seedSubjectGoals(t, db)
		if _, err := db.Exec(`INSERT INTO subject_goals (subject_id,goal_id,user_id,goal_number) VALUES (1,1,'owner',1)`); err != nil {
			t.Fatal(err)
		}
		events := event.NewService(data.NewSQLiteEventStore(db))
		item, err := events.CreatePast(t.Context(), "owner", "Zero", time.Unix(100, 0), time.Unix(100, 0), []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := events.Create(t.Context(), "owner", "Ongoing", time.Unix(200, 0), []int64{1}); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteGoalProgressStore(openSQLite(t, dsn))
		page, err := store.ListEarnedEvents(t.Context(), "owner", 1, data.GoalEventPageRequest{})
		if err != nil || len(page.Events) != 1 || page.Events[0].ElapsedSeconds != 0 {
			t.Fatalf("zero/ongoing history: got %+v,%v, want zero-duration Event only", page, err)
		}
		thoughts, err := store.ListEventLineageThoughts(t.Context(), "owner", 1, item.EventID, data.ThoughtSummaryViewRequest{})
		if err != nil || thoughts.Thoughts.Items == nil || len(thoughts.Thoughts.Items) != 0 {
			t.Fatalf("zero-duration Thoughts: got %+v,%v, want non-nil empty slice", thoughts, err)
		}
	})
	t.Run("rejects elapsed time that overflows an integer instead of displaying a rounded duration", func(t *testing.T) {
		db, events := attributionFixture(t)
		item, err := events.CreatePast(t.Context(), "owner", "Work", time.Unix(100, 0), time.Unix(200, 0), []int64{1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE events SET started_at=-9223372036854775808, ended_at=9223372036854775807 WHERE event_id=?`, item.EventID); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteGoalProgressStore(db)
		if _, err := store.ListEarnedEvents(t.Context(), "owner", 1, data.GoalEventPageRequest{}); err == nil {
			t.Fatal("overflowing interval returned no error")
		}
		if _, err := store.ListEventLineageThoughts(t.Context(), "owner", 1, item.EventID, data.ThoughtSummaryViewRequest{}); err == nil {
			t.Fatal("overflowing expansion returned no error")
		}
	})
}
