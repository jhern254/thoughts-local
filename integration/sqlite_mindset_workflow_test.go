//go:build integration

package integration_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestThoughtMindsetWorkflow_SQLite(t *testing.T) {
	t.Run("retains designation history without changing the thought", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		ctx := context.Background()
		original, err := service.Create(ctx, "owner", "Practice deliberately", nil, time.Unix(10, 0))
		if err != nil {
			t.Fatal(err)
		}
		for _, action := range []struct {
			active bool
			at     int64
		}{{true, 100}, {true, 110}, {false, 120}, {false, 130}, {true, 140}} {
			active, err := service.SetMindset(ctx, "owner", original.ThoughtID, action.active, time.Unix(action.at, 0))
			if err != nil || active != action.active {
				t.Fatalf("SetMindset: got %v, %v, want %v, nil", active, err, action.active)
			}
		}
		got, err := service.Get(ctx, "owner", original.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		want := *original
		want.IsMindset = true
		if !reflect.DeepEqual(*got, want) {
			t.Fatalf("thought: got %+v, want %+v", got, want)
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM thought_mindset_periods WHERE thought_id=?`, original.ThoughtID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("period count: got %d, want 2", count)
		}
		if _, err := service.SetMindset(ctx, "owner", original.ThoughtID, false, time.Unix(139, 0)); !errors.Is(err, data.ErrMindsetChronology) {
			t.Fatalf("chronology error: got %v, want ErrMindsetChronology", err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		service = thought.NewService(data.NewSQLiteThoughtStore(openSQLite(t, dsn)))
		view, err := service.BrowseView(ctx, "owner", data.ThoughtSummaryViewRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if len(view.Items) != 1 || !view.Items[0].IsMindset {
			t.Fatalf("reopened preview: got %+v, want marked thought", view)
		}
	})
	t.Run("lists each thought once ordered by latest overlapping designation", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner", "other")
		_, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'owner','one'),(2,'owner','two'),(3,'owner','three'),(4,'owner','ended at midnight'),(5,'owner','starts tomorrow'),(6,'other','private'),(7,'owner','deleted');
  INSERT INTO thought_mindset_periods(thought_id,started_at,ended_at) VALUES (1,50,120),(1,180,NULL),(2,90,150),(3,110,110),(4,50,100),(5,200,NULL),(6,110,NULL),(7,110,NULL);
  UPDATE thoughts SET deleted_at=updated_at WHERE thought_id=7;`)
		if err != nil {
			t.Fatal(err)
		}
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		rows, err := service.ListMindsetsInRange(t.Context(), "owner", time.Unix(100, 0), time.Unix(200, 0))
		if err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, row := range rows {
			ids = append(ids, row.ThoughtID)
		}
		if want := []int64{2, 3, 1}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("ordered thought IDs: got %v, want %v", ids, want)
		}
		if rows[0].IsMindset || !rows[2].IsMindset {
			t.Fatalf("current status: got %+v, want closed two and open one", rows)
		}
	})
	for _, state := range []string{"missing", "other owner", "deleted thought", "deleted user"} {
		t.Run("rejects "+state, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "owner", "other")
			_, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'owner','one')`)
			if err != nil {
				t.Fatal(err)
			}
			owner, id := "owner", int64(1)
			switch state {
			case "missing":
				id = 2
			case "other owner":
				owner = "other"
			case "deleted thought":
				_, err = db.Exec(`UPDATE thoughts SET deleted_at=updated_at`)
			case "deleted user":
				_, err = db.Exec(`UPDATE users SET deleted_at=updated_at WHERE user_id='owner'`)
			}
			if err != nil {
				t.Fatal(err)
			}
			service := thought.NewService(data.NewSQLiteThoughtStore(db))
			if _, err := service.SetMindset(t.Context(), owner, id, true, time.Unix(100, 0)); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("mark error: got %v, want ErrRecordNotFound", err)
			}
		})
	}
}

func TestThoughtMindsetFailuresWorkflow_SQLite(t *testing.T) {
	for _, failure := range []string{"canceled", "busy", "read only"} {
		t.Run("preserves history after "+failure, func(t *testing.T) {
			db, dsn := openMigratedSQLite(t)
			insertUsers(t, db, "owner")
			if _, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'owner','one'); INSERT INTO thought_mindset_periods(thought_id,started_at) VALUES (1,100)`); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			store := data.NewSQLiteThoughtStore(db)
			wantErr := context.Canceled
			switch failure {
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "busy":
				writer := openSQLite(t, dsn)
				if _, err := writer.Exec("BEGIN IMMEDIATE"); err != nil {
					t.Fatal(err)
				}
				defer writer.Exec("ROLLBACK")
				wantErr = data.ErrDatabaseBusy
			case "read only":
				store = data.NewSQLiteThoughtStore(openSQLite(t, dsn+"?mode=ro"))
				wantErr = data.ErrDatabaseReadOnly
			}
			if _, err := store.SetThoughtMindset(ctx, "owner", 1, false, time.Unix(120, 0)); !errors.Is(err, wantErr) {
				t.Fatalf("unmark error: got %v, want %v", err, wantErr)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM thought_mindset_periods WHERE thought_id=1 AND started_at=100 AND ended_at IS NULL`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("retained periods: got %d, want 1", count)
			}
		})
	}
	t.Run("rejects reactivation before the previous end and allows equal seconds", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		if _, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'owner','one'); INSERT INTO thought_mindset_periods(thought_id,started_at,ended_at) VALUES (1,100,200)`); err != nil {
			t.Fatal(err)
		}
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		if _, err := service.SetMindset(t.Context(), "owner", 1, true, time.Unix(199, 0)); !errors.Is(err, data.ErrMindsetChronology) {
			t.Fatalf("reactivation: got %v, want chronology error", err)
		}
		for _, active := range []bool{true, false, true} {
			if _, err := service.SetMindset(t.Context(), "owner", 1, active, time.Unix(200, 0)); err != nil {
				t.Fatal(err)
			}
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM thought_mindset_periods`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 3 {
			t.Fatalf("periods: got %d, want 3", count)
		}
	})
	t.Run("hidden owner keeps periods but has no historical panel", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		if _, err := db.Exec(`INSERT INTO thoughts(thought_id,user_id,thought) VALUES (1,'owner','one'); INSERT INTO thought_mindset_periods(thought_id,started_at) VALUES (1,100); UPDATE users SET deleted_at=updated_at WHERE user_id='owner'`); err != nil {
			t.Fatal(err)
		}
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		rows, err := service.ListMindsetsInRange(t.Context(), "owner", time.Unix(100, 0), time.Unix(200, 0))
		if err != nil || len(rows) != 0 {
			t.Fatalf("hidden owner panel: got %v, %v, want empty", rows, err)
		}
	})
}
