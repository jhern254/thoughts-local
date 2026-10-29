//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestDayActivityWorkflow_SQLite(t *testing.T) {
	t.Run("counts event days and all observed thoughts without multiplying joins", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u", "other")
		for _, q := range []string{
			`INSERT INTO events(user_id,started_at,ended_at) VALUES ('u',0,86400),('u',43200,172800),('u',86400,86400),('u',172800,NULL),('other',0,200000)`,
			`INSERT INTO events(user_id,started_at,ended_at,deleted_at) VALUES ('u',0,100,unixepoch())`,
			`INSERT INTO thoughts(user_id,thought,observed_at) VALUES ('u','before account',0),('u','unassigned',86399),('u','next day',86400),('u','current',172800),('u','future',250000),('other','foreign',0)`,
			`INSERT INTO thoughts(user_id,thought,observed_at,deleted_at) VALUES ('u','hidden',0,unixepoch())`,
		} {
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
		store := data.NewSQLiteMetricsStore(db)
		start, err := store.ActivityStart(t.Context(), "u", time.Unix(200000, 0))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := start.Unix(), int64(0); got != want {
			t.Fatalf("start: got %d, want %d", got, want)
		}
		boundaries := []time.Time{time.Unix(0, 0).UTC(), time.Unix(86400, 0).UTC(), time.Unix(172800, 0).UTC(), time.Unix(259200, 0).UTC()}
		counts, err := store.ActivityCounts(t.Context(), "u", boundaries, time.Unix(200000, 0))
		if err != nil {
			t.Fatal(err)
		}
		want := []data.ActivityCount{{EventDays: 2, Thoughts: 2}, {EventDays: 2, Thoughts: 1}, {EventDays: 1, Thoughts: 1}}
		for i := range want {
			if counts[i] != want[i] {
				t.Fatalf("bucket %d: got %+v, want %+v", i, counts[i], want[i])
			}
		}
		combined, err := store.ActivityCounts(t.Context(), "u", []time.Time{boundaries[0], boundaries[3]}, time.Unix(200000, 0))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := combined[0], (data.ActivityCount{EventDays: 5, Thoughts: 4}); got != want {
			t.Fatalf("combined: got %+v, want %+v", got, want)
		}
	})
	t.Run("counts civil days across daylight saving and spanning intervals", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		loc, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			t.Fatal(err)
		}
		from := time.Date(2026, 3, 7, 0, 0, 0, 0, loc)
		until := from.AddDate(0, 0, 3)
		if _, err := db.Exec(`INSERT INTO events(user_id,started_at,ended_at) VALUES ('u',?,?)`, from.Add(-time.Hour).Unix(), until.Unix()); err != nil {
			t.Fatal(err)
		}
		got, err := data.NewSQLiteMetricsStore(db).ActivityCounts(t.Context(), "u", []time.Time{from, from.AddDate(0, 0, 1), until}, until)
		if err != nil {
			t.Fatal(err)
		}
		if got[0].EventDays != 1 || got[1].EventDays != 2 {
			t.Fatalf("DST event days: got %+v, want 1 and 2", got)
		}
	})
}

func TestDayActivityBoundaries_SQLite(t *testing.T) {
	t.Run("long events fill interior buckets across fall daylight saving", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		loc, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			t.Fatal(err)
		}
		from := time.Date(2026, 10, 30, 0, 0, 0, 0, loc)
		boundaries := []time.Time{from, from.AddDate(0, 0, 1), from.AddDate(0, 0, 3), from.AddDate(0, 0, 5), from.AddDate(0, 0, 6)}
		until := boundaries[4]
		if _, err := db.Exec(`INSERT INTO events(user_id,started_at,ended_at) VALUES('u',?,?)`, from.Add(time.Hour).Unix(), until.Add(-time.Hour).Unix()); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteMetricsStore(db)
		counts, err := store.ActivityCounts(t.Context(), "u", boundaries, until)
		if err != nil {
			t.Fatal(err)
		}
		for i, want := range []int64{1, 2, 2, 1} {
			if counts[i].EventDays != want {
				t.Fatalf("bucket %d: got %d, want %d", i, counts[i].EventDays, want)
			}
		}
	})
	t.Run("empty and deleted users have no activity", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		// Explicit lifecycle times keep this deletion fixture independent of wall-clock seconds.
		if _, err := db.Exec(`UPDATE users SET created_at=1,updated_at=1 WHERE user_id='u'`); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteMetricsStore(db)
		cutoff := time.Unix(86400, 0).UTC()
		boundaries := []time.Time{time.Unix(0, 0).UTC(), cutoff}
		for _, deleted := range []bool{false, true} {
			if deleted {
				if _, err := db.Exec(`INSERT INTO thoughts(user_id,thought,observed_at) VALUES('u','synthetic',0); UPDATE users SET deleted_at=2,updated_at=2 WHERE user_id='u'`); err != nil {
					t.Fatal(err)
				}
			}
			first, err := store.ActivityStart(t.Context(), "u", cutoff)
			if err != nil {
				t.Fatal(err)
			}
			if !first.IsZero() {
				t.Fatalf("start: got %v, want zero", first)
			}
			counts, err := store.ActivityCounts(t.Context(), "u", boundaries, cutoff)
			if err != nil {
				t.Fatal(err)
			}
			if counts[0] != (data.ActivityCount{}) {
				t.Fatalf("counts: got %+v, want zero", counts)
			}
		}
	})
}

func TestDayActivityReadContract_SQLite(t *testing.T) {
	t.Run("rejects unbounded or nonchronological buckets and respects cancellation", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "u")
		store := data.NewSQLiteMetricsStore(db)
		from := time.Unix(0, 0).UTC()
		until := from.AddDate(0, 0, 1)
		for _, bounds := range [][]time.Time{nil, {from}, {until, from}, {from.Add(time.Second), until}, make([]time.Time, 258)} {
			if _, err := store.ActivityCounts(t.Context(), "u", bounds, until); err == nil {
				t.Fatalf("bounds %v: got success, want rejection", bounds)
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := store.ActivityStart(ctx, "u", until); !errors.Is(err, context.Canceled) {
			t.Fatalf("start: got %v, want cancellation", err)
		}
		if _, err := store.ActivityCounts(ctx, "u", []time.Time{from, until}, until); !errors.Is(err, context.Canceled) {
			t.Fatalf("counts: got %v, want cancellation", err)
		}
	})
}
