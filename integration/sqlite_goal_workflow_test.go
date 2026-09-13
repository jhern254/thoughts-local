//go:build integration

package integration_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func goalRecord(owner, name string) *data.Goal {
	return &data.Goal{UserID: owner, GoalName: name, TargetSeconds: 600,
		IsActive: true, Cadence: "weekly", TZ: "UTC", WeekStart: "mon",
		CreatedAt: time.Unix(100, 0).UTC(), UpdatedAt: time.Unix(200, 0).UTC()}
}

func TestGoalWorkflow_SQLite(t *testing.T) {
	for _, scope := range []string{"another owner", "missing goal", "deleted user", "missing user"} {
		t.Run("hides records from "+scope, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "owner", "other")
			store, ctx := data.NewSQLiteGoalStore(db), context.Background()
			original, err := store.CreateGoal(ctx, goalRecord("owner", "Goal"))
			if err != nil {
				t.Fatal(err)
			}
			input := *original
			switch scope {
			case "another owner":
				input.UserID = "other"
			case "missing goal":
				input.GoalID = 99
			case "deleted user":
				if _, err := db.Exec("UPDATE users SET deleted_at=updated_at WHERE user_id='owner'"); err != nil {
					t.Fatal(err)
				}
			case "missing user":
				input.UserID = "missing"
			}
			input.Version = 99 // Not-found takes precedence over stale version.
			if _, err := store.GetGoal(ctx, input.UserID, input.GoalID); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("inaccessible get: got %v, want ErrRecordNotFound", err)
			}
			if _, err := store.UpdateGoal(ctx, &input); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("inaccessible update: got %v, want ErrRecordNotFound", err)
			}
			if err := store.DeleteGoal(ctx, input.UserID, input.GoalID, input.Version); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("inaccessible delete: got %v, want ErrRecordNotFound", err)
			}
			if scope == "deleted user" || scope == "missing user" {
				if _, err := store.CreateGoal(ctx, goalRecord(input.UserID, "New")); !errors.Is(err, data.ErrRecordNotFound) {
					t.Fatalf("inaccessible create: got %v, want ErrRecordNotFound", err)
				}
			}
			if scope != "missing goal" {
				for _, list := range []struct {
					name string
					call func(context.Context, string) ([]data.Goal, error)
				}{{"all", store.ListGoals}, {"active", store.ListActiveGoals}, {"inactive", store.ListInactiveGoals}} {
					items, err := list.call(ctx, input.UserID)
					if err != nil {
						t.Fatal(err)
					}
					if items == nil || len(items) != 0 {
						t.Fatalf("%s inaccessible list: got %#v, want non-nil empty slice", list.name, items)
					}
				}
			}
			var preserved bool
			if err := db.QueryRow("SELECT user_id='owner' AND goal_name='Goal' AND version=1 AND deleted_at IS NULL FROM goals WHERE goal_id=1").Scan(&preserved); err != nil {
				t.Fatal(err)
			}
			if !preserved {
				t.Fatal("goal after inaccessible writes: got changed, want unchanged")
			}
		})
	}

	t.Run("enforces per user names until soft deletion and preserves duplicate causes", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner", "other")
		store, ctx := data.NewSQLiteGoalStore(db), context.Background()
		first, err := store.CreateGoal(ctx, goalRecord("owner", "Same"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateGoal(ctx, goalRecord("other", "Same")); err != nil {
			t.Fatalf("other owner name: got %v, want nil", err)
		}
		second, err := store.CreateGoal(ctx, goalRecord("owner", "Different"))
		if err != nil {
			t.Fatal(err)
		}
		input := *second
		input.GoalName = first.GoalName
		_, createErr := store.CreateGoal(ctx, goalRecord("owner", "Same"))
		_, updateErr := store.UpdateGoal(ctx, &input)
		for _, failure := range []struct {
			name string
			err  error
		}{{"create", createErr}, {"update", updateErr}} {
			var cause *sqlite.Error
			if !errors.Is(failure.err, data.ErrDuplicateRecord) || !errors.As(failure.err, &cause) {
				t.Fatalf("%s duplicate: got %v, want ErrDuplicateRecord and retained SQLite cause", failure.name, failure.err)
			}
			if got, want := cause.Code(), sqlite3.SQLITE_CONSTRAINT_UNIQUE; got != want {
				t.Fatalf("%s duplicate code: got %d, want %d", failure.name, got, want)
			}
		}
		got, err := store.GetGoal(ctx, "owner", second.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, second) {
			t.Fatalf("goal after duplicate update: got %+v, want %+v", got, second)
		}
		if err := store.DeleteGoal(ctx, "owner", first.GoalID, first.Version); err != nil {
			t.Fatal(err)
		}
		reused, err := store.CreateGoal(ctx, goalRecord("owner", "Same"))
		if err != nil {
			t.Fatalf("reuse deleted name: got %v, want nil", err)
		}
		if reused.GoalID == first.GoalID || reused.Version != 1 {
			t.Fatalf("reused name: got ID %d/version %d, want new ID/version 1", reused.GoalID, reused.Version)
		}
	})

	t.Run("honors Unicode and schema permitted date and timezone text", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		input := goalRecord("owner", strings.Repeat("界", 512))
		// The schema checks date shape and timezone length, not calendar validity
		// or IANA membership. Persistence must not silently add stricter rules.
		date := "2026-19-39"
		input.StartDate, input.EndDate, input.TZ = &date, &date, "Custom zone"
		got, err := data.NewSQLiteGoalStore(db).CreateGoal(context.Background(), input)
		if err != nil {
			t.Fatalf("schema permitted settings: got %v, want nil", err)
		}
		if got.GoalName != input.GoalName || *got.StartDate != date || *got.EndDate != date || got.TZ != input.TZ {
			t.Fatalf("stored settings: got %+v, want unchanged name, dates, and timezone from %+v", got, input)
		}
	})
	for _, tc := range []struct {
		name   string
		change func(*data.Goal)
	}{
		{"blank name", func(g *data.Goal) { g.GoalName = " " }},
		{"overlong Unicode name", func(g *data.Goal) { g.GoalName = strings.Repeat("界", 513) }},
		{"nonpositive target", func(g *data.Goal) { g.TargetSeconds = 0 }},
		{"unknown cadence", func(g *data.Goal) { g.Cadence = "unknown" }},
		{"unknown default cadence", func(g *data.Goal) { v := "unknown"; g.DefaultCadence = &v }},
		{"unknown week start", func(g *data.Goal) { g.WeekStart = "unknown" }},
		{"blank timezone", func(g *data.Goal) { g.TZ = " " }},
		{"overlong timezone", func(g *data.Goal) { g.TZ = strings.Repeat("x", 65) }},
		{"malformed date", func(g *data.Goal) { v := "09/01/2026"; g.StartDate = &v }},
		{"reversed dates", func(g *data.Goal) { a, b := "2026-09-02", "2026-09-01"; g.StartDate, g.EndDate = &a, &b }},
		{"reversed timestamps", func(g *data.Goal) { g.UpdatedAt = g.CreatedAt.Add(-time.Second) }},
	} {
		t.Run("returns the database constraint for "+tc.name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "owner")
			input := goalRecord("owner", "Goal")
			tc.change(input)
			_, err := data.NewSQLiteGoalStore(db).CreateGoal(context.Background(), input)
			var cause *sqlite.Error
			if !errors.As(err, &cause) || cause.Code() != sqlite3.SQLITE_CONSTRAINT_CHECK || errors.Is(err, data.ErrDuplicateRecord) {
				t.Fatalf("invalid settings: got %v, want retained SQLite CHECK error", err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM goals").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if got, want := count, 0; got != want {
				t.Fatalf("rows after rejected creation: got %d, want %d", got, want)
			}
		})
	}

	t.Run("preserves cancellation for reads and writes", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		store := data.NewSQLiteGoalStore(db)
		item, err := store.CreateGoal(context.Background(), goalRecord("owner", "Goal"))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.GetGoal(ctx, "owner", item.GoalID); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled get: got %v, want context.Canceled", err)
		}
		if _, err := store.ListGoals(ctx, "owner"); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled list: got %v, want context.Canceled", err)
		}
		if _, err := store.UpdateGoal(ctx, item); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled update: got %v, want context.Canceled", err)
		}
	})

	t.Run("retains busy and read only driver errors", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		store, ctx := data.NewSQLiteGoalStore(db), context.Background()
		item, err := store.CreateGoal(ctx, goalRecord("owner", "Goal"))
		if err != nil {
			t.Fatal(err)
		}
		writer := openSQLite(t, dsn)
		if _, err := writer.Exec("BEGIN IMMEDIATE"); err != nil {
			t.Fatal(err)
		}
		_, err = store.UpdateGoal(ctx, item)
		assertDatabaseFailure(t, err, data.ErrDatabaseBusy, sqlite3.SQLITE_BUSY)
		if _, err := writer.Exec("ROLLBACK"); err != nil {
			t.Fatal(err)
		}
		_, err = data.NewSQLiteGoalStore(openSQLite(t, dsn+"?mode=ro")).CreateGoal(ctx, goalRecord("owner", "New"))
		assertDatabaseFailure(t, err, data.ErrDatabaseReadOnly, sqlite3.SQLITE_READONLY)
	})

	t.Run("rolls back deletion on a busy commit and leaves the connection reusable", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		store, ctx := data.NewSQLiteGoalStore(db), context.Background()
		item, err := store.CreateGoal(ctx, goalRecord("owner", "Goal"))
		if err != nil {
			t.Fatal(err)
		}
		reader := openSQLite(t, dsn)
		tx, err := reader.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM goals").Scan(&count); err != nil {
			t.Fatal(err)
		}
		err = store.DeleteGoal(ctx, "owner", item.GoalID, item.Version)
		assertDatabaseFailure(t, err, data.ErrDatabaseBusy, sqlite3.SQLITE_BUSY)
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		got, err := data.NewSQLiteGoalStore(reader).GetGoal(ctx, "owner", item.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, item) {
			t.Fatalf("goal after failed commit: got %+v, want %+v", got, item)
		}
		if err := store.DeleteGoal(ctx, "owner", item.GoalID, item.Version); err != nil {
			t.Fatalf("retry after releasing reader: got %v, want nil", err)
		}
	})

	t.Run("updates settings and activation while preserving immutable metadata", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		store, ctx := data.NewSQLiteGoalStore(db), context.Background()
		original, err := store.CreateGoal(ctx, goalRecord("owner", "Original"))
		if err != nil {
			t.Fatal(err)
		}
		input := *original
		start, end, cadence := "2026-09-01", "2026-12-31", "daily"
		input.GoalName, input.TargetSeconds, input.IsActive = "PRIVATE-Updated", 1200, false
		input.StartDate, input.EndDate, input.DefaultCadence = &start, &end, &cadence
		input.Cadence, input.TZ, input.WeekStart = "yearly", "America/Los_Angeles", "sun"
		input.CreatedAt, input.UpdatedAt = time.Unix(999, 0), time.Unix(150, 0)
		snapshot := input
		updated, err := store.UpdateGoal(ctx, &input)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(input, snapshot) {
			t.Fatalf("update input: got %+v, want %+v", input, snapshot)
		}
		want := input
		want.CreatedAt, want.UpdatedAt, want.Version = original.CreatedAt, original.UpdatedAt, 2
		if !reflect.DeepEqual(*updated, want) {
			t.Fatalf("updated goal: got %+v, want %+v", *updated, want)
		}
		// Clear the nullable settings and reactivate the same record.
		input = *updated
		input.StartDate, input.EndDate, input.DefaultCadence, input.IsActive = nil, nil, nil, true
		input.UpdatedAt = time.Unix(300, 0).UTC()
		updated, err = store.UpdateGoal(ctx, &input)
		if err != nil {
			t.Fatal(err)
		}
		want = input
		want.Version = 3
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		got, err := data.NewSQLiteGoalStore(openSQLite(t, dsn)).GetGoal(ctx, "owner", original.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(*got, want) || !reflect.DeepEqual(*updated, want) {
			t.Fatalf("reactivated goal: got stored %+v and returned %+v, want %+v", *got, *updated, want)
		}
	})

	t.Run("rejects stale update and delete after another connection edits", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		store, ctx := data.NewSQLiteGoalStore(db), context.Background()
		original, err := store.CreateGoal(ctx, goalRecord("owner", "Goal"))
		if err != nil {
			t.Fatal(err)
		}
		// Even identical settings constitute a new revision.
		current, err := data.NewSQLiteGoalStore(openSQLite(t, dsn)).UpdateGoal(ctx, original)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := current.Version, original.Version+1; got != want {
			t.Fatalf("new version: got %d, want %d", got, want)
		}
		original.GoalName = "PRIVATE-Stale"
		if _, err := store.UpdateGoal(ctx, original); !errors.Is(err, data.ErrVersionConflict) {
			t.Fatalf("stale update: got %v, want ErrVersionConflict", err)
		}
		if err := store.DeleteGoal(ctx, "owner", original.GoalID, original.Version); !errors.Is(err, data.ErrVersionConflict) {
			t.Fatalf("stale deletion: got %v, want ErrVersionConflict", err)
		}
		got, err := store.GetGoal(ctx, "owner", original.GoalID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, current) {
			t.Fatalf("goal after rejected writes: got %+v, want %+v", got, current)
		}
	})

	t.Run("updates and soft deletes without changing progress or parent links", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		store, ctx := data.NewSQLiteGoalStore(db), context.Background()
		original, err := store.CreateGoal(ctx, goalRecord("owner", "Goal"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(`INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES
            (2,'owner','Parent',60),(3,'owner','Child',60);
            INSERT INTO goal_parents(goal_id,parent_goal_id,user_id) VALUES (1,2,'owner'),(3,1,'owner');
            INSERT INTO events(event_id,user_id) VALUES (1,'owner');
            INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'owner','Thought');
            INSERT INTO goal_progress(goal_id,user_id,event_id,thought_id,time_spent_sec,progress_note,occurred_at,created_at)
            VALUES (1,'owner',1,1,60,'PRIVATE-Provenance',100,200);`)
		if err != nil {
			t.Fatal(err)
		}
		original.GoalName = "Changed"
		updated, err := store.UpdateGoal(ctx, original)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteGoal(ctx, "owner", updated.GoalID, updated.Version); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openSQLite(t, dsn)
		store = data.NewSQLiteGoalStore(db)
		if _, err := store.GetGoal(ctx, "owner", 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("deleted get: got %v, want ErrRecordNotFound", err)
		}
		if _, err := store.UpdateGoal(ctx, updated); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("deleted update: got %v, want ErrRecordNotFound", err)
		}
		if err := store.DeleteGoal(ctx, "owner", 1, updated.Version+1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("repeat deletion: got %v, want ErrRecordNotFound", err)
		}
		var preserved bool
		err = db.QueryRow(`SELECT goal_name='Changed' AND user_id='owner' AND goal_is_active=1
            AND target_seconds=600 AND cadence='weekly' AND tz='UTC' AND week_start='mon'
            AND created_at=100 AND version=3 AND deleted_at=updated_at AND deleted_at>=200
            FROM goals WHERE goal_id=1`).Scan(&preserved)
		if err != nil {
			t.Fatal(err)
		}
		if !preserved {
			t.Fatal("retained goal and deletion metadata: got false, want true")
		}
		for _, tc := range []struct {
			name, query string
			want        int
		}{
			{"progress provenance", `SELECT count(*) FROM goal_progress WHERE goal_id=1 AND user_id='owner' AND event_id=1 AND thought_id=1 AND time_spent_sec=60 AND progress_note='PRIVATE-Provenance' AND occurred_at=100 AND created_at=200`, 1},
			{"incident links", `SELECT count(*) FROM goal_parents WHERE user_id='owner' AND (goal_id,parent_goal_id) IN ((1,2),(3,1))`, 2},
			{"unchanged neighbors", `SELECT count(*) FROM goals WHERE goal_id IN (2,3) AND deleted_at IS NULL AND goal_is_active=1 AND version=1`, 2},
			{"foreign-key violations", `SELECT count(*) FROM pragma_foreign_key_check`, 0},
		} {
			var got int
			if err := db.QueryRow(tc.query).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("%s: got %d, want %d", tc.name, got, tc.want)
			}
		}
	})

	for _, optional := range []bool{false, true} {
		name := "round trips nullable settings"
		if optional {
			name = "persists complete settings across connections without changing input"
		}
		t.Run(name, func(t *testing.T) {
			db, dsn := openMigratedSQLite(t)
			insertUsers(t, db, "owner")
			input := goalRecord("owner", "  PRIVATE-Goal 界  ")
			input.GoalID, input.Version = 999, 99 // Creation generates both.
			if optional {
				start, end, cadence := "2026-09-01", "2026-12-31", "daily"
				input.StartDate, input.EndDate, input.DefaultCadence = &start, &end, &cadence
				input.IsActive, input.Cadence, input.TZ, input.WeekStart = false, "monthly", "America/Los_Angeles", "sun"
			}
			input.CreatedAt = input.CreatedAt.In(time.FixedZone("offset", -7*3600))
			original := *input
			store := data.NewSQLiteGoalStore(db)
			created, err := store.CreateGoal(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*input, original) {
				t.Fatalf("create input: got %+v, want %+v", *input, original)
			}
			if created.GoalID == 0 || created.GoalID == input.GoalID {
				t.Fatalf("created ID: got %d, want generated nonzero ID", created.GoalID)
			}
			want := original
			want.GoalID, want.Version = created.GoalID, 1
			want.CreatedAt = want.CreatedAt.UTC()
			if !reflect.DeepEqual(*created, want) {
				t.Fatalf("created goal: got %+v, want %+v", *created, want)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			got, err := data.NewSQLiteGoalStore(openSQLite(t, dsn)).GetGoal(context.Background(), "owner", created.GoalID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, want) {
				t.Fatalf("persisted goal: got %+v, want %+v", *got, want)
			}
		})
	}

	t.Run("lists all active and inactive goals in ID order for their owner", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner", "other")
		_, err := db.Exec(`INSERT INTO goals(goal_id,user_id,goal_name,target_seconds,goal_is_active) VALUES
            (30,'owner','A',60,1),(10,'owner','Z',60,0),(20,'owner','B',60,1),
            (40,'owner','Deleted active',60,1),(41,'owner','Deleted inactive',60,0),(50,'other','Other',60,1);
            UPDATE goals SET deleted_at=updated_at WHERE goal_id IN (40,41);`)
		if err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteGoalStore(db)
		for _, tc := range []struct {
			name string
			list func(context.Context, string) ([]data.Goal, error)
			want []int64
		}{
			{"all", store.ListGoals, []int64{10, 20, 30}},
			{"active", store.ListActiveGoals, []int64{20, 30}},
			{"inactive", store.ListInactiveGoals, []int64{10}},
		} {
			items, err := tc.list(context.Background(), "owner")
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int64, len(items))
			for i, item := range items {
				ids[i] = item.GoalID
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("%s list IDs: got %v, want %v", tc.name, ids, tc.want)
			}
		}
	})
}
