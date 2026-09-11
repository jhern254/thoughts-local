//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"modernc.org/sqlite"
)

func eventTime(second int64) time.Time { return time.Unix(second, 0).UTC() }

func eventRecord(user string, start int64, end *time.Time) *data.Event {
	return &data.Event{UserID: user, StartedAt: eventTime(start), EndedAt: end,
		CreatedAt: eventTime(1000), UpdatedAt: eventTime(1000)}
}

func TestEventWorkflow_SQLite(t *testing.T) {
	t.Run("creation requires the appropriate interval state", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		store := data.NewSQLiteEventStore(db)
		ctx := context.Background()
		if _, err := store.AddPastEvent(ctx, eventRecord("u", 100, nil)); !errors.Is(err, data.ErrInvalidEventInterval) {
			t.Fatalf("got missing historical end error %v, want ErrInvalidEventInterval", err)
		}
		end := eventTime(200)
		if _, err := store.StartEvent(ctx, eventRecord("u", 100, &end)); !errors.Is(err, data.ErrInvalidEventInterval) {
			t.Fatalf("got ended start error %v, want ErrInvalidEventInterval", err)
		}
		if _, err := store.AddPastEvent(ctx, eventRecord("u", 300, &end)); !errors.Is(err, data.ErrInvalidEventInterval) {
			t.Fatalf("got reversed historical interval error %v, want ErrInvalidEventInterval", err)
		}
	})

	t.Run("ongoing corrections preserve explicit instants and immutable metadata", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		store := data.NewSQLiteEventStore(db)
		ctx := context.Background()
		input := eventRecord("u", 100, nil)
		input.StartedAt = input.StartedAt.In(time.FixedZone("offset", -7*3600))
		got, err := store.StartEvent(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if got.StartedAt.Unix() != 100 || got.StartedAt.Location() != time.UTC || got.ActivityType != nil {
			t.Fatalf("got stored event %+v, want instant 100 in UTC with no label", got)
		}
		got.StartedAt, got.CreatedAt, got.UpdatedAt = eventTime(50), eventTime(9999), eventTime(900)
		updated, err := store.UpdateEvent(ctx, got)
		if err != nil {
			t.Fatal(err)
		}
		if updated.StartedAt.Unix() != 50 || updated.CreatedAt.Unix() != 1000 || updated.UpdatedAt.Unix() != 1000 || updated.EndedAt != nil || updated.Version != 2 {
			t.Fatalf("got corrected event %+v, want start 50, retained timestamps 1000, ongoing version 2", updated)
		}
	})

	t.Run("a busy commit rolls back the handoff and leaves the connection reusable", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		store := data.NewSQLiteEventStore(db)
		ctx := context.Background()
		first, err := store.StartEvent(ctx, eventRecord("u", 100, nil))
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
		if err := tx.QueryRow("SELECT count(*) FROM events").Scan(&count); err != nil {
			t.Fatal(err)
		}
		// In rollback-journal mode the read transaction prevents writer COMMIT.
		if _, err := store.StartEvent(ctx, eventRecord("u", 200, nil)); !errors.Is(err, data.ErrDatabaseBusy) {
			t.Fatalf("got blocked commit error %v, want ErrDatabaseBusy", err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		for _, connection := range []*data.SQLiteEventStore{store, data.NewSQLiteEventStore(reader)} {
			got, err := connection.GetEvent(ctx, "u", first.EventID)
			if err != nil {
				t.Fatal(err)
			}
			if got.EndedAt != nil || got.Version != 1 {
				t.Fatalf("got event %+v, want original ongoing event after failed commit", got)
			}
		}
		if err := reader.QueryRow("SELECT count(*) FROM events").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("got event count %d, want 1 after rollback", count)
		}
		if _, err := store.StartEvent(ctx, eventRecord("u", 200, nil)); err != nil {
			t.Fatalf("got next handoff error %v, want reusable connection", err)
		}
	})

	for _, start := range []int64{50, 200} {
		name := "rejects a handoff before the ongoing start"
		if start == 200 {
			name = "rolls back handoff when the successor overlaps a completed event"
		}
		t.Run(name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u")
			if _, err := db.Exec(`INSERT INTO events(event_id,user_id,started_at,ended_at) VALUES
				(1,'u',100,NULL),(2,'u',300,400)`); err != nil {
				t.Fatal(err)
			}
			store := data.NewSQLiteEventStore(db)
			if _, err := store.StartEvent(context.Background(), eventRecord("u", start, nil)); !errors.Is(err, data.ErrEventOverlap) {
				t.Fatalf("got start error %v, want ErrEventOverlap", err)
			}
			item, err := store.GetEvent(context.Background(), "u", 1)
			if err != nil {
				t.Fatal(err)
			}
			if item.EndedAt != nil || item.Version != 1 {
				t.Fatalf("got prior event %+v, want unchanged ongoing version 1", item)
			}
		})
	}

	t.Run("conflicting corrections do not move adjacent events", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec(`INSERT INTO events(event_id,user_id,started_at,ended_at) VALUES
			(1,'u',100,200),(2,'u',300,NULL)`); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteEventStore(db)
		ctx := context.Background()
		completed, err := store.GetEvent(ctx, "u", 1)
		if err != nil {
			t.Fatal(err)
		}
		end := eventTime(350)
		completed.EndedAt = &end
		if _, err := store.UpdateEvent(ctx, completed); !errors.Is(err, data.ErrEventOverlap) {
			t.Fatalf("got correction error %v, want ErrEventOverlap", err)
		}
		ongoing, err := store.GetEvent(ctx, "u", 2)
		if err != nil {
			t.Fatal(err)
		}
		ongoing.StartedAt = eventTime(150)
		if _, err := store.UpdateEvent(ctx, ongoing); !errors.Is(err, data.ErrEventOverlap) {
			t.Fatalf("got ongoing correction error %v, want ErrEventOverlap", err)
		}
		if _, err := store.EndEvent(ctx, "u", 2, 1, eventTime(250), eventTime(1100)); !errors.Is(err, data.ErrInvalidEventInterval) {
			t.Fatalf("got reversed end error %v, want ErrInvalidEventInterval", err)
		}
		end = eventTime(50)
		if _, err := store.UpdateEvent(ctx, completed); !errors.Is(err, data.ErrInvalidEventInterval) {
			t.Fatalf("got reversed correction error %v, want ErrInvalidEventInterval", err)
		}
		for id, want := range map[int64]int64{1: 100, 2: 300} {
			got, err := store.GetEvent(ctx, "u", id)
			if err != nil {
				t.Fatal(err)
			}
			if got.Version != 1 || got.StartedAt.Unix() != want {
				t.Fatalf("got event %+v, want original start %d and version 1", got, want)
			}
		}
	})

	t.Run("same-second handoff and empty intervals retain their boundary semantics", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		store := data.NewSQLiteEventStore(db)
		ctx := context.Background()
		if _, err := store.StartEvent(ctx, eventRecord("u", 100, nil)); err != nil {
			t.Fatal(err)
		}
		if _, err := store.StartEvent(ctx, eventRecord("u", 100, nil)); err != nil {
			t.Fatalf("got same-second handoff error %v, want nil", err)
		}
		end := eventTime(150)
		if _, err := store.AddPastEvent(ctx, eventRecord("u", 150, &end)); err != nil {
			t.Fatalf("got empty interval error %v, want nil", err)
		}
		items, err := store.ListEvents(ctx, "u", eventTime(100), eventTime(200))
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 3 || items[0].EndedAt == nil || items[0].EndedAt.Unix() != 100 || items[1].EndedAt != nil {
			t.Fatalf("got events %+v, want empty predecessor, ongoing successor, empty historical event", items)
		}
	})

	t.Run("new entries cannot bypass overlap checks using an existing ID", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		if _, err := db.Exec("INSERT INTO events(event_id,user_id,started_at,ended_at) VALUES (0,'u',100,200)"); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteEventStore(db)
		end := eventTime(180)
		item := eventRecord("u", 150, &end)
		if _, err := store.AddPastEvent(context.Background(), item); !errors.Is(err, data.ErrEventOverlap) {
			t.Fatalf("got insert error %v, want ErrEventOverlap with ID zero present", err)
		}
	})

	for _, scope := range []string{"other", "missing", "deleted event", "deleted owner"} {
		t.Run("rejects reads and edits for "+scope, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u", "other")
			store := data.NewSQLiteEventStore(db)
			ctx := context.Background()
			item, err := store.StartEvent(ctx, eventRecord("u", 100, nil))
			if err != nil {
				t.Fatal(err)
			}
			user, id := "u", item.EventID
			switch scope {
			case "other":
				user = "other"
			case "missing":
				id++
			case "deleted event":
				if _, err := db.Exec("UPDATE events SET deleted_at=updated_at"); err != nil {
					t.Fatal(err)
				}
			case "deleted owner":
				if _, err := db.Exec("UPDATE users SET deleted_at=updated_at WHERE user_id='u'"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.GetEvent(ctx, user, id); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got get error %v, want ErrRecordNotFound", err)
			}
			if _, err := store.EndEvent(ctx, user, id, 1, eventTime(200), eventTime(1100)); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got end error %v, want ErrRecordNotFound", err)
			}
			item.UserID, item.EventID = user, id
			if _, err := store.UpdateEvent(ctx, item); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got update error %v, want ErrRecordNotFound", err)
			}
			var version int64
			if err := db.QueryRow("SELECT version FROM events").Scan(&version); err != nil {
				t.Fatal(err)
			}
			if version != 1 {
				t.Fatalf("got retained version %d, want 1", version)
			}
		})
	}

	for _, user := range []string{"missing", "deleted"} {
		t.Run("rejects new events for "+user+" owner", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "deleted")
			if _, err := db.Exec("UPDATE users SET deleted_at=updated_at"); err != nil {
				t.Fatal(err)
			}
			store := data.NewSQLiteEventStore(db)
			ctx := context.Background()
			if _, err := store.StartEvent(ctx, eventRecord(user, 100, nil)); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got start error %v, want ErrRecordNotFound", err)
			}
			end := eventTime(200)
			if _, err := store.AddPastEvent(ctx, eventRecord(user, 100, &end)); !errors.Is(err, data.ErrRecordNotFound) {
				t.Fatalf("got historical insert error %v, want ErrRecordNotFound", err)
			}
		})
	}

	t.Run("competing edits do not lose a successful update", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		first := data.NewSQLiteEventStore(db)
		second := data.NewSQLiteEventStore(openSQLite(t, dsn))
		ctx := context.Background()
		item, err := first.StartEvent(ctx, eventRecord("u", 100, nil))
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, store := range []*data.SQLiteEventStore{first, second} {
			go func() {
				<-start
				_, err := store.EndEvent(ctx, "u", item.EventID, item.Version, eventTime(200), eventTime(1100))
				results <- err
			}()
		}
		close(start)
		successes := 0
		for range 2 {
			if err := <-results; err == nil {
				successes++
			} else if !errors.Is(err, data.ErrDatabaseBusy) && !errors.Is(err, data.ErrEventVersionConflict) {
				t.Fatalf("got competing edit error %v, want busy or stale version", err)
			}
		}
		if successes > 1 {
			t.Fatalf("got %d successful edits, want at most 1", successes)
		}
		got, err := first.GetEvent(ctx, "u", item.EventID)
		if err != nil {
			t.Fatal(err)
		}
		// With no busy timeout or automatic retries, both transactions may fail
		// acquiring/committing their write. Neither may leave a partial update.
		if successes == 0 {
			if got.Version != 1 || got.EndedAt != nil {
				t.Fatalf("got event %+v, want unchanged ongoing event after contention", got)
			}
			got, err = first.EndEvent(ctx, "u", item.EventID, item.Version, eventTime(200), eventTime(1100))
			if err != nil {
				t.Fatal(err)
			}
		}
		if got.Version != 2 || got.EndedAt == nil || got.EndedAt.Unix() != 200 {
			t.Fatalf("got event %+v, want one committed end at 200 with version 2", got)
		}
	})

	t.Run("read-only and canceled operations retain recognizable causes", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		readOnly := data.NewSQLiteEventStore(openSQLite(t, dsn+"?mode=ro"))
		var cause *sqlite.Error
		if _, err := readOnly.StartEvent(context.Background(), eventRecord("u", 100, nil)); !errors.Is(err, data.ErrDatabaseReadOnly) || !errors.As(err, &cause) {
			t.Fatalf("got read-only start error %v, want ErrDatabaseReadOnly with driver cause", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := data.NewSQLiteEventStore(db).StartEvent(ctx, eventRecord("u", 100, nil)); !errors.Is(err, context.Canceled) {
			t.Fatalf("got canceled start error %v, want context.Canceled", err)
		}
	})

	t.Run("ends and edits an event with optimistic version protection", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		store := data.NewSQLiteEventStore(db)
		ctx := context.Background()
		item, err := store.StartEvent(ctx, eventRecord("u", 100, nil))
		if err != nil {
			t.Fatal(err)
		}
		ended, err := store.EndEvent(ctx, "u", item.EventID, item.Version, eventTime(200), eventTime(1100))
		if err != nil {
			t.Fatal(err)
		}
		if ended.EndedAt == nil || ended.EndedAt.Unix() != 200 || ended.Version != 2 {
			t.Fatalf("got ended event %+v, want end 200 and version 2", ended)
		}
		if _, err := store.EndEvent(ctx, "u", item.EventID, item.Version, eventTime(300), eventTime(1200)); !errors.Is(err, data.ErrEventVersionConflict) {
			t.Fatalf("got stale end error %v, want ErrEventVersionConflict", err)
		}
		if _, err := store.EndEvent(ctx, "u", ended.EventID, ended.Version, eventTime(300), eventTime(1200)); !errors.Is(err, data.ErrEventStateConflict) {
			t.Fatalf("got repeated end error %v, want ErrEventStateConflict", err)
		}
		label := "activity"
		ended.ActivityType, ended.StartedAt, ended.UpdatedAt = &label, eventTime(50), eventTime(1200)
		updated, err := store.UpdateEvent(ctx, ended)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Version != 3 || updated.StartedAt.Unix() != 50 || updated.ActivityType == nil || *updated.ActivityType != label {
			t.Fatalf("got updated event %+v, want version 3, start 50, activity label", updated)
		}
		if ended.Version != 2 {
			t.Fatalf("got caller version %d, want unchanged 2", ended.Version)
		}
		if _, err := store.UpdateEvent(ctx, ended); !errors.Is(err, data.ErrEventVersionConflict) {
			t.Fatalf("got stale update error %v, want ErrEventVersionConflict", err)
		}
		reopened := *updated
		reopened.EndedAt = nil
		if _, err := store.UpdateEvent(ctx, &reopened); !errors.Is(err, data.ErrEventStateConflict) {
			t.Fatalf("got reopening error %v, want ErrEventStateConflict", err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		persisted, err := data.NewSQLiteEventStore(openSQLite(t, dsn)).GetEvent(ctx, "u", updated.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.Version != 3 || persisted.EndedAt == nil || persisted.EndedAt.Unix() != 200 || persisted.CreatedAt.Unix() != 1000 || persisted.UpdatedAt.Unix() != 1200 {
			t.Fatalf("got persisted event %+v, want version 3, end 200 and timestamps 1000/1200", persisted)
		}
	})

	t.Run("lists intersecting events in order including ongoing and zero duration events", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other", "deleted")
		if _, err := db.Exec(`UPDATE users SET deleted_at=updated_at WHERE user_id='deleted';
			INSERT INTO events(event_id,user_id,started_at,ended_at,deleted_at) VALUES
			(1,'u',50,100,NULL),(2,'u',90,110,NULL),(3,'u',120,130,NULL),
			(4,'u',120,120,NULL),(5,'u',150,NULL,NULL),(6,'u',200,210,NULL),
			(7,'other',110,120,NULL),(8,'u',110,120,unixepoch()),(9,'deleted',110,120,NULL)`); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteEventStore(db)
		for _, tc := range []struct {
			user string
			ids  []int64
		}{{"u", []int64{2, 3, 4, 5}}, {"deleted", nil}, {"missing", nil}} {
			items, err := store.ListEvents(context.Background(), tc.user, eventTime(100), eventTime(200))
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != len(tc.ids) {
				t.Fatalf("got %d events for %s, want %d", len(items), tc.user, len(tc.ids))
			}
			for i, item := range items {
				if item.EventID != tc.ids[i] {
					t.Fatalf("got event ID %d, want %d", item.EventID, tc.ids[i])
				}
			}
		}
		if _, err := store.ListEvents(context.Background(), "u", eventTime(200), eventTime(100)); !errors.Is(err, data.ErrInvalidEventInterval) {
			t.Fatalf("got reversed range error %v, want ErrInvalidEventInterval", err)
		}
	})

	t.Run("starting an event atomically closes the previous event", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		store := data.NewSQLiteEventStore(db)
		ctx := context.Background()
		first, err := store.StartEvent(ctx, eventRecord("u", 100, nil))
		if err != nil {
			t.Fatal(err)
		}
		second, err := store.StartEvent(ctx, eventRecord("u", 200, nil))
		if err != nil {
			t.Fatal(err)
		}
		closed, err := store.GetEvent(ctx, "u", first.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if closed.EndedAt == nil || !closed.EndedAt.Equal(second.StartedAt) || closed.Version != 2 {
			t.Fatalf("got previous event %+v, want end 200 and version 2", closed)
		}
		if second.EndedAt != nil || second.Version != 1 || second.EventID == first.EventID {
			t.Fatalf("got new event %+v, want distinct ongoing event with version 1", second)
		}
	})

	t.Run("failed insertion leaves the previous event unchanged", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		store := data.NewSQLiteEventStore(db)
		ctx := context.Background()
		first, err := store.StartEvent(ctx, eventRecord("u", 100, nil))
		if err != nil {
			t.Fatal(err)
		}
		// Fail after the prior event is closed, exercising transaction rollback.
		if _, err := db.Exec(`CREATE TRIGGER reject_event_insert BEFORE INSERT ON events
			BEGIN SELECT RAISE(ABORT, 'private insertion failure'); END`); err != nil {
			t.Fatal(err)
		}
		var cause *sqlite.Error
		if _, err := store.StartEvent(ctx, eventRecord("u", 200, nil)); !errors.As(err, &cause) {
			t.Fatalf("got insertion error %v, want preserved SQLite trigger failure", err)
		}
		unchanged, err := store.GetEvent(ctx, "u", first.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if unchanged.EndedAt != nil || unchanged.Version != first.Version || !unchanged.UpdatedAt.Equal(first.UpdatedAt) {
			t.Fatalf("got previous event %+v, want unchanged %+v", unchanged, first)
		}
	})

	t.Run("historical entry allows adjacency but rejects overlap without ending the ongoing event", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		store := data.NewSQLiteEventStore(db)
		ctx := context.Background()
		ongoing, err := store.StartEvent(ctx, eventRecord("u", 300, nil))
		if err != nil {
			t.Fatal(err)
		}
		end := eventTime(300)
		if _, err := store.AddPastEvent(ctx, eventRecord("u", 200, &end)); err != nil {
			t.Fatalf("got adjacent insertion error %v, want nil", err)
		}
		for _, start := range []int64{150, 250, 300} {
			end := eventTime(start + 100)
			if _, err := store.AddPastEvent(ctx, eventRecord("u", start, &end)); !errors.Is(err, data.ErrEventOverlap) {
				t.Fatalf("got overlapping insertion error %v, want ErrEventOverlap", err)
			}
		}
		got, err := store.GetEvent(ctx, "u", ongoing.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if got.EndedAt != nil || got.Version != 1 {
			t.Fatalf("got ongoing event %+v, want unchanged version 1 with no end", got)
		}
	})
}
