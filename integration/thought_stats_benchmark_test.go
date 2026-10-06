//go:build integration

package integration_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func benchmarkThoughtStats(b *testing.B, db *sql.DB, from, until time.Time, count, maximum int64) {
	store := data.NewSQLiteMetricsStore(db)
	ctx := b.Context()
	stats, err := store.ThoughtStatsInRange(ctx, "bench", from, until)
	if err != nil || stats.Count != count || stats.MaxCharacters != maximum {
		b.Fatalf("stats: got %+v, %v; want count %d, maximum %d", stats, err, count, maximum)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := store.ThoughtStatsInRange(ctx, "bench", from, until); err != nil {
			b.Fatal(err)
		}
	}
}
