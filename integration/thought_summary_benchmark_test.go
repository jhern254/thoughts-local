//go:build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

// Run without other builds/tests: go test -tags=integration ./integration -run '^$' -bench '^BenchmarkThoughtSummarySQLite$' -benchmem -count=5 -benchtime=1s
// Each fixture uses a migrated on-disk SQLite DB and one connection, like the app.
// Migrations, inserts, warmup and correctness guards stay outside timed loops.
// This measures warm reads; it does not simulate a cold OS page cache.
func BenchmarkThoughtSummarySQLite(b *testing.B) {
	for _, fixture := range []struct {
		name                 string
		thoughts, characters int
	}{
		{"BoundedPage", 230, 512},
		{"LargeBodies", 2000, 65536},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			db, _ := openMigratedSQLite(b)
			ctx := b.Context()
			if _, err := db.ExecContext(ctx, "INSERT INTO users(user_id) VALUES ('bench')"); err != nil {
				b.Fatal(err)
			}
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				b.Fatal(err)
			}
			defer tx.Rollback()
			insert, err := tx.PrepareContext(ctx, "INSERT INTO thoughts(user_id,thought,observed_at,created_at,updated_at) VALUES ('bench',?,?,?,?)")
			if err != nil {
				b.Fatal(err)
			}
			defer insert.Close()
			// Alternate ASCII and Unicode, with four reproducible lengths. The longest
			// appears beyond the first 50-row page too; all rows belong to one frozen interval.
			for i := 0; i < fixture.thoughts; i++ {
				characters := fixture.characters - (i%4)*fixture.characters/8
				body := strings.Repeat("a界", characters/2)
				observed := int64(1700000000 + i)
				if _, err := insert.ExecContext(ctx, body, observed, observed, observed); err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				b.Fatal(err)
			}
			store := data.NewSQLiteThoughtStore(db)
			from := time.Unix(1700000000, 0)
			until := from.Add(time.Duration(fixture.thoughts) * time.Second)
			b.Run("BrowseThoughtsView", func(b *testing.B) {
				view, err := store.BrowseThoughtsView(ctx, "bench", data.ThoughtSummaryViewRequest{})
				if err != nil || len(view.Items) != 50 || !view.More {
					b.Fatalf("initial page: got %d, more=%t, %v; want 50 and more", len(view.Items), view.More, err)
				}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := store.BrowseThoughtsView(ctx, "bench", data.ThoughtSummaryViewRequest{}); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("BrowseThoughtsViewInRange", func(b *testing.B) {
				view, err := store.BrowseThoughtsViewInRange(ctx, "bench", from, until, data.ThoughtSummaryViewRequest{})
				if err != nil || len(view.Items) != 50 || !view.More {
					b.Fatalf("initial interval page: got %d, more=%t, %v; want 50 and more", len(view.Items), view.More, err)
				}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := store.BrowseThoughtsViewInRange(ctx, "bench", from, until, data.ThoughtSummaryViewRequest{}); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("LatestThoughtInRange", func(b *testing.B) {
				latest, err := store.LatestThoughtInRange(ctx, "bench", from, until)
				if err != nil || latest == nil || latest.ThoughtID != int64(fixture.thoughts) {
					b.Fatalf("latest: got %v, %v; want thought %d", latest, err, fixture.thoughts)
				}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := store.LatestThoughtInRange(ctx, "bench", from, until); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("ThoughtStatsInRange", func(b *testing.B) {
				benchmarkThoughtStats(b, db, from, until, int64(fixture.thoughts), int64(fixture.characters))
			})
		})
	}
}
