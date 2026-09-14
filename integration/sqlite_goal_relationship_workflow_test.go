//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func seedGoalRelationships(t *testing.T, db *sql.DB) {
	t.Helper()
	insertUsers(t, db, "owner", "other")
	_, err := db.Exec(`INSERT INTO goals(goal_id,user_id,goal_name,target_seconds) VALUES
 (1,'owner','Root',60),(2,'owner','Parent',60),(3,'owner','Child',60),(4,'other','Other',60),(5,'owner','Second parent',60);
 INSERT INTO events(event_id,user_id,started_at,ended_at) VALUES (1,'owner',100,200),(2,'other',100,200),(3,'owner',300,NULL);
 INSERT INTO thoughts(thought_id,user_id,event_id,thought) VALUES (1,'owner',3,'Context'),(2,'other',2,'Other context');`)
	if err != nil {
		t.Fatal(err)
	}
}

func progressRecord() *data.GoalProgress {
	return &data.GoalProgress{GoalID: 3, UserID: "owner", TimeSpentSec: 60, OccurredAt: time.Unix(150, 0).UTC(), CreatedAt: time.Unix(200, 0).UTC()}
}

func TestGoalProgressStoreWorkflow_SQLite(t *testing.T) {
	t.Run("persists explicit contributions and independent nullable provenance", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		seedGoalRelationships(t, db)
		store := data.NewSQLiteGoalProgressStore(db)
		id, note := int64(1), "  PRIVATE-NOTE 界  "
		empty := ""
		for _, refs := range []struct {
			event, thought *int64
			note           *string
		}{{nil, nil, nil}, {nil, nil, &empty}, {&id, nil, &note}, {nil, &id, nil}, {&id, &id, &note}} {
			input := progressRecord()
			input.EventID, input.ThoughtID, input.ProgressNote = refs.event, refs.thought, refs.note
			input.OccurredAt = time.Unix(150, 123).In(time.FixedZone("offset", -7*3600))
			snapshot := *input
			created, err := store.CreateGoalProgress(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			if created.ProgressID <= 0 {
				t.Fatalf("progress ID: got %d, want positive", created.ProgressID)
			}
			want := snapshot
			want.ProgressID, want.OccurredAt = created.ProgressID, time.Unix(150, 0).UTC()
			if !reflect.DeepEqual(*created, want) {
				t.Fatalf("created progress: got %+v, want %+v", created, want)
			}
			if !reflect.DeepEqual(*input, snapshot) {
				t.Fatalf("input: got %+v, want %+v", input, snapshot)
			}
			got, err := data.NewSQLiteGoalProgressStore(openSQLite(t, dsn)).GetGoalProgress(t.Context(), "owner", created.ProgressID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, created) {
				t.Fatalf("persisted progress: got %+v, want %+v", got, created)
			}
		}
	})
	t.Run("lists direct contributions in time and ID order within a half open range", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedGoalRelationships(t, db)
		store := data.NewSQLiteGoalProgressStore(db)
		var want []data.GoalProgress
		for _, second := range []int64{200, 150, 100, 150, 99} {
			input := progressRecord()
			input.OccurredAt = time.Unix(second, 0).UTC()
			item, err := store.CreateGoalProgress(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			if second == 100 {
				want = append([]data.GoalProgress{*item}, want...)
			} else if second == 150 {
				want = append(want, *item)
			}
		}
		other := progressRecord()
		other.GoalID = 2
		if _, err := store.CreateGoalProgress(t.Context(), other); err != nil {
			t.Fatal(err)
		}
		if err := data.NewSQLiteGoalParentStore(db).AddGoalParent(t.Context(), "owner", 3, 2); err != nil {
			t.Fatal(err)
		}
		got, err := store.ListGoalProgress(t.Context(), "owner", 3, time.Unix(100, 0), time.Unix(200, 0))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("listed progress: got %+v, want %+v", got, want)
		}
		direct, err := store.ListGoalProgress(t.Context(), "owner", 2, time.Unix(100, 0), time.Unix(200, 0))
		if err != nil {
			t.Fatal(err)
		}
		if len(direct) != 1 {
			t.Fatalf("parent direct entries: got %d, want 1", len(direct))
		}
		for _, bounds := range [][2]int64{{150, 150}, {200, 100}, {300, 400}} {
			got, err := store.ListGoalProgress(t.Context(), "owner", 3, time.Unix(bounds[0], 0), time.Unix(bounds[1], 0))
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || len(got) != 0 {
				t.Fatalf("empty range: got %#v, want non-nil empty", got)
			}
		}
	})
}

func TestGoalParentStoreWorkflow_SQLite(t *testing.T) {
	t.Run("adds reads and removes individual edges without changing goals or progress", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedGoalRelationships(t, db)
		store := data.NewSQLiteGoalParentStore(db)
		if _, err := data.NewSQLiteGoalProgressStore(db).CreateGoalProgress(t.Context(), progressRecord()); err != nil {
			t.Fatal(err)
		}
		for _, edge := range [][2]int64{{3, 5}, {3, 2}, {2, 1}, {5, 1}} {
			if err := store.AddGoalParent(t.Context(), "owner", edge[0], edge[1]); err != nil {
				t.Fatal(err)
			}
		}
		parents, err := store.ListGoalParents(t.Context(), "owner", 3)
		if err != nil {
			t.Fatal(err)
		}
		want := []data.GoalParent{{GoalID: 3, ParentGoalID: 2, UserID: "owner"}, {GoalID: 3, ParentGoalID: 5, UserID: "owner"}}
		if !reflect.DeepEqual(parents, want) {
			t.Fatalf("parents: got %+v, want %+v", parents, want)
		}
		children, err := store.ListGoalChildren(t.Context(), "owner", 1)
		if err != nil {
			t.Fatal(err)
		}
		want = []data.GoalParent{{GoalID: 2, ParentGoalID: 1, UserID: "owner"}, {GoalID: 5, ParentGoalID: 1, UserID: "owner"}}
		if !reflect.DeepEqual(children, want) {
			t.Fatalf("children: got %+v, want %+v", children, want)
		}
		if err := store.RemoveGoalParent(t.Context(), "owner", 3, 2); err != nil {
			t.Fatal(err)
		}
		if err := store.RemoveGoalParent(t.Context(), "owner", 3, 2); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("missing removal: got %v, want not found", err)
		}
		var links, goals, progress int
		if err := db.QueryRow(`SELECT (SELECT count(*) FROM goal_parents),(SELECT count(*) FROM goals),(SELECT count(*) FROM goal_progress)`).Scan(&links, &goals, &progress); err != nil {
			t.Fatal(err)
		}
		if links != 3 || goals != 5 || progress != 1 {
			t.Fatalf("retained records: got %d/%d/%d, want 3/5/1", links, goals, progress)
		}
	})
}

func TestGoalRelationshipVisibilityWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name, change string
		field        string
		id           int64
	}{
		{"missing goal", "", "goal", 99}, {"another owners goal", "", "goal", 4},
		{"deleted goal", "UPDATE goals SET deleted_at=updated_at WHERE goal_id=3", "goal", 3},
		{"missing event", "", "event", 99}, {"another owners event", "", "event", 2},
		{"deleted event", "UPDATE events SET deleted_at=updated_at WHERE event_id=1", "event", 1},
		{"missing thought", "", "thought", 99}, {"another owners thought", "", "thought", 2},
		{"deleted thought", "UPDATE thoughts SET deleted_at=updated_at WHERE thought_id=1", "thought", 1},
		{"deleted owner", "UPDATE users SET deleted_at=updated_at WHERE user_id='owner'", "", 0},
	} {
		t.Run("rejects new progress with "+tc.name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedGoalRelationships(t, db)
			if tc.change != "" {
				if _, err := db.Exec(tc.change); err != nil {
					t.Fatal(err)
				}
			}
			input := progressRecord()
			switch tc.field {
			case "goal":
				input.GoalID = tc.id
			case "event":
				input.EventID = &tc.id
			case "thought":
				input.ThoughtID = &tc.id
			}
			_, err := data.NewSQLiteGoalProgressStore(db).CreateGoalProgress(t.Context(), input)
			if !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("create: got %v, want not found", err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM goal_progress").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("inserted progress: got %d, want 0", count)
			}
		})
	}
	for _, tc := range []struct {
		name, change  string
		child, parent int64
	}{
		{"missing child", "", 99, 1}, {"missing parent", "", 3, 99},
		{"another owners child", "", 4, 1}, {"another owners parent", "", 3, 4},
		{"deleted child", "UPDATE goals SET deleted_at=updated_at WHERE goal_id=3", 3, 1},
		{"deleted parent", "UPDATE goals SET deleted_at=updated_at WHERE goal_id=1", 3, 1},
		{"deleted owner", "UPDATE users SET deleted_at=updated_at WHERE user_id='owner'", 3, 1},
	} {
		t.Run("rejects new parent link with "+tc.name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedGoalRelationships(t, db)
			if tc.change != "" {
				if _, err := db.Exec(tc.change); err != nil {
					t.Fatal(err)
				}
			}
			err := data.NewSQLiteGoalParentStore(db).AddGoalParent(t.Context(), "owner", tc.child, tc.parent)
			if !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("add parent: got %v, want not found", err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM goal_parents").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("inserted links: got %d, want 0", count)
			}
		})
	}
	t.Run("retains history and links after soft deletion while blocking other and deleted owners", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedGoalRelationships(t, db)
		progress, parents := data.NewSQLiteGoalProgressStore(db), data.NewSQLiteGoalParentStore(db)
		id := int64(1)
		input := progressRecord()
		input.EventID, input.ThoughtID = &id, &id
		item, err := progress.CreateGoalProgress(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if err := parents.AddGoalParent(t.Context(), "owner", 3, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE goals SET goal_is_active=0,deleted_at=updated_at WHERE goal_id IN (1,3);
   UPDATE events SET deleted_at=updated_at WHERE event_id=1; UPDATE thoughts SET deleted_at=updated_at WHERE thought_id=1;`); err != nil {
			t.Fatal(err)
		}
		got, err := progress.GetGoalProgress(t.Context(), "owner", item.ProgressID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, item) {
			t.Fatalf("retained progress: got %+v, want %+v", got, item)
		}
		entries, err := progress.ListGoalProgress(t.Context(), "owner", 3, time.Unix(0, 0), time.Unix(300, 0))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(entries, []data.GoalProgress{*item}) {
			t.Fatalf("history: got %+v, want %+v", entries, item)
		}
		links, err := parents.ListGoalParents(t.Context(), "owner", 3)
		if err != nil {
			t.Fatal(err)
		}
		want := []data.GoalParent{{GoalID: 3, ParentGoalID: 1, UserID: "owner"}}
		if !reflect.DeepEqual(links, want) {
			t.Fatalf("retained parents: got %+v, want %+v", links, want)
		}
		children, err := parents.ListGoalChildren(t.Context(), "owner", 1)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(children, want) {
			t.Fatalf("retained children: got %+v, want %+v", children, want)
		}
		for _, owner := range []string{"other", "missing", "owner"} {
			if owner == "owner" {
				if _, err := db.Exec("UPDATE users SET deleted_at=updated_at WHERE user_id='owner'"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := progress.GetGoalProgress(t.Context(), owner, item.ProgressID); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("inaccessible get: got %v, want not found", err)
			}
			entries, err := progress.ListGoalProgress(t.Context(), owner, 3, time.Unix(0, 0), time.Unix(300, 0))
			if err != nil {
				t.Fatal(err)
			}
			if entries == nil || len(entries) != 0 {
				t.Fatalf("inaccessible history: got %#v, want non-nil empty", entries)
			}
			links, err := parents.ListGoalParents(t.Context(), owner, 3)
			if err != nil {
				t.Fatal(err)
			}
			if links == nil || len(links) != 0 {
				t.Fatalf("inaccessible parents: got %#v, want non-nil empty", links)
			}
			links, err = parents.ListGoalChildren(t.Context(), owner, 1)
			if err != nil {
				t.Fatal(err)
			}
			if links == nil || len(links) != 0 {
				t.Fatalf("inaccessible children: got %#v, want non-nil empty", links)
			}
			if err := parents.RemoveGoalParent(t.Context(), owner, 3, 1); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("inaccessible removal: got %v, want not found", err)
			}
		}
		if _, err := db.Exec("UPDATE users SET deleted_at=NULL WHERE user_id='owner'"); err != nil {
			t.Fatal(err)
		}
		if err := parents.RemoveGoalParent(t.Context(), "owner", 3, 1); err != nil {
			t.Fatalf("remove retained link: got %v, want nil", err)
		}
		got, err = progress.GetGoalProgress(t.Context(), "owner", item.ProgressID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, item) {
			t.Fatalf("progress after unlinking: got %+v, want %+v", got, item)
		}
	})
	t.Run("allows inactive goals and explicit progress on ongoing events", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedGoalRelationships(t, db)
		if _, err := db.Exec("UPDATE goals SET goal_is_active=0 WHERE user_id='owner'"); err != nil {
			t.Fatal(err)
		}
		event := int64(3)
		input := progressRecord()
		input.EventID = &event
		got, err := data.NewSQLiteGoalProgressStore(db).CreateGoalProgress(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if got.TimeSpentSec != input.TimeSpentSec {
			t.Fatalf("explicit duration: got %d, want %d", got.TimeSpentSec, input.TimeSpentSec)
		}
		if err := data.NewSQLiteGoalParentStore(db).AddGoalParent(t.Context(), "owner", 3, 1); err != nil {
			t.Fatal(err)
		}
	})
}

func TestGoalRelationshipMissingRecordsWorkflow_SQLite(t *testing.T) {
	t.Run("returns not found for missing records and non-nil empty relationship lists", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedGoalRelationships(t, db)
		progress, parents := data.NewSQLiteGoalProgressStore(db), data.NewSQLiteGoalParentStore(db)
		if _, err := progress.GetGoalProgress(t.Context(), "owner", 99); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("missing progress: got %v, want not found", err)
		}
		entries, err := progress.ListGoalProgress(t.Context(), "owner", 99, time.Unix(0, 0), time.Unix(300, 0))
		if err != nil {
			t.Fatal(err)
		}
		if entries == nil || len(entries) != 0 {
			t.Fatalf("missing goal progress: got %#v, want non-nil empty", entries)
		}
		for _, id := range []int64{3, 99} {
			links, err := parents.ListGoalParents(t.Context(), "owner", id)
			if err != nil {
				t.Fatal(err)
			}
			if links == nil || len(links) != 0 {
				t.Fatalf("empty parents: got %#v, want non-nil empty", links)
			}
			links, err = parents.ListGoalChildren(t.Context(), "owner", id)
			if err != nil {
				t.Fatal(err)
			}
			if links == nil || len(links) != 0 {
				t.Fatalf("empty children: got %#v, want non-nil empty", links)
			}
		}
		input := progressRecord()
		input.UserID = "missing"
		if _, err := progress.CreateGoalProgress(t.Context(), input); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("missing owner progress: got %v, want not found", err)
		}
		if err := parents.AddGoalParent(t.Context(), "missing", 3, 1); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("missing owner link: got %v, want not found", err)
		}
	})
}

func TestGoalRelationshipConstraintsWorkflow_SQLite(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds int64
		note    string
	}{{"zero contribution", 0, ""}, {"negative contribution", -1, ""}, {"overlong note", 60, strings.Repeat("界", 1000001)}} {
		t.Run("preserves database error for "+tc.name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedGoalRelationships(t, db)
			input := progressRecord()
			input.TimeSpentSec, input.ProgressNote = tc.seconds, &tc.note
			_, err := data.NewSQLiteGoalProgressStore(db).CreateGoalProgress(t.Context(), input)
			var cause *sqlite.Error
			if !errors.As(err, &cause) || cause.Code() != sqlite3.SQLITE_CONSTRAINT_CHECK {
				t.Fatalf("constraint: got %v, want SQLite CHECK", err)
			}
		})
	}
	t.Run("preserves a maximum length Unicode note", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedGoalRelationships(t, db)
		note := strings.Repeat("界", 1000000)
		input := progressRecord()
		input.ProgressNote = &note
		got, err := data.NewSQLiteGoalProgressStore(db).CreateGoalProgress(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if got.ProgressNote == nil || *got.ProgressNote != note {
			t.Fatal("note: got changed content, want original Unicode note")
		}
	})
	for _, tc := range []struct {
		name          string
		child, parent int64
		deleted       bool
		code          int
	}{{"duplicate", 3, 2, false, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY}, {"self link", 5, 5, false, sqlite3.SQLITE_CONSTRAINT_CHECK}, {"cycle", 1, 3, false, sqlite3.SQLITE_CONSTRAINT_TRIGGER}, {"cycle through deleted goal", 1, 3, true, sqlite3.SQLITE_CONSTRAINT_TRIGGER}} {
		t.Run("rejects "+tc.name+" without changing existing links", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			seedGoalRelationships(t, db)
			store := data.NewSQLiteGoalParentStore(db)
			for _, edge := range [][2]int64{{3, 2}, {2, 1}} {
				if err := store.AddGoalParent(t.Context(), "owner", edge[0], edge[1]); err != nil {
					t.Fatal(err)
				}
			}
			if tc.deleted {
				if _, err := db.Exec("UPDATE goals SET deleted_at=updated_at,goal_is_active=0 WHERE goal_id=2"); err != nil {
					t.Fatal(err)
				}
			}
			err := store.AddGoalParent(t.Context(), "owner", tc.child, tc.parent)
			var cause *sqlite.Error
			if !errors.As(err, &cause) || cause.Code() != tc.code {
				t.Fatalf("constraint: got %v, want SQLite code %d", err, tc.code)
			}
			if tc.name == "duplicate" && !errors.Is(err, data.ErrDuplicateRecord) {
				t.Fatalf("duplicate: got %v, want ErrDuplicateRecord", err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM goal_parents WHERE (goal_id=3 AND parent_goal_id=2) OR (goal_id=2 AND parent_goal_id=1)").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("retained links: got %d, want 2", count)
			}
			if err := db.QueryRow("SELECT count(*) FROM goal_parents").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("total links: got %d, want 2", count)
			}
		})
	}
}

func TestGoalRelationshipErrorsWorkflow_SQLite(t *testing.T) {
	t.Run("preserves cancellation for all operations", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		seedGoalRelationships(t, db)
		progress, parents := data.NewSQLiteGoalProgressStore(db), data.NewSQLiteGoalParentStore(db)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, createErr := progress.CreateGoalProgress(ctx, progressRecord())
		_, getErr := progress.GetGoalProgress(ctx, "owner", 1)
		_, listErr := progress.ListGoalProgress(ctx, "owner", 3, time.Unix(0, 0), time.Unix(300, 0))
		_, parentErr := parents.ListGoalParents(ctx, "owner", 3)
		_, childErr := parents.ListGoalChildren(ctx, "owner", 1)
		for operation, err := range map[string]error{"create": createErr, "get": getErr, "list": listErr, "parents": parentErr, "children": childErr, "add": parents.AddGoalParent(ctx, "owner", 3, 1), "remove": parents.RemoveGoalParent(ctx, "owner", 3, 1)} {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s: got %v, want context.Canceled", operation, err)
			}
		}
	})
	t.Run("retains busy and read only identities and driver causes", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		seedGoalRelationships(t, db)
		progress, parents := data.NewSQLiteGoalProgressStore(db), data.NewSQLiteGoalParentStore(db)
		if err := parents.AddGoalParent(t.Context(), "owner", 3, 1); err != nil {
			t.Fatal(err)
		}
		writer := openSQLite(t, dsn)
		if _, err := writer.Exec("BEGIN IMMEDIATE"); err != nil {
			t.Fatal(err)
		}
		_, err := progress.CreateGoalProgress(t.Context(), progressRecord())
		assertDatabaseFailure(t, err, data.ErrDatabaseBusy, sqlite3.SQLITE_BUSY)
		assertDatabaseFailure(t, parents.AddGoalParent(t.Context(), "owner", 3, 2), data.ErrDatabaseBusy, sqlite3.SQLITE_BUSY)
		assertDatabaseFailure(t, parents.RemoveGoalParent(t.Context(), "owner", 3, 1), data.ErrDatabaseBusy, sqlite3.SQLITE_BUSY)
		if _, err := writer.Exec("ROLLBACK"); err != nil {
			t.Fatal(err)
		}
		ro := openSQLite(t, dsn+"?mode=ro")
		_, err = data.NewSQLiteGoalProgressStore(ro).CreateGoalProgress(t.Context(), progressRecord())
		assertDatabaseFailure(t, err, data.ErrDatabaseReadOnly, sqlite3.SQLITE_READONLY)
		assertDatabaseFailure(t, data.NewSQLiteGoalParentStore(ro).AddGoalParent(t.Context(), "owner", 3, 2), data.ErrDatabaseReadOnly, sqlite3.SQLITE_READONLY)
		assertDatabaseFailure(t, data.NewSQLiteGoalParentStore(ro).RemoveGoalParent(t.Context(), "owner", 3, 1), data.ErrDatabaseReadOnly, sqlite3.SQLITE_READONLY)
	})
}
