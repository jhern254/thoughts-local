//go:build integration

package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

// Measures the real store write and read-back, including its commit. Body creation,
// migrations and create cleanup are untimed; cleanup bounds database growth.
// Run separately from other builds/tests with -benchmem -count=5 -benchtime=1s.
func BenchmarkThoughtWriteSQLite(b *testing.B) {
	for _, fixture := range []struct {
		name       string
		characters int
	}{
		{"Small", 512}, {"Large", 65536}, {"NearLimit", 1000000},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			for _, operation := range []string{"Create", "Update"} {
				b.Run(operation, func(b *testing.B) {
					db, _ := openMigratedSQLite(b)
					ctx := b.Context()
					if _, err := db.ExecContext(ctx, "INSERT INTO users(user_id) VALUES ('bench')"); err != nil {
						b.Fatal(err)
					}
					bodies := []string{strings.Repeat("a界", fixture.characters/2), strings.Repeat("b語", fixture.characters/2)}
					store := data.NewSQLiteThoughtStore(db)
					now := time.Unix(1700000000, 0)
					item := data.Thought{
						UserID:     "bench",
						Thought:    bodies[0],
						Version:    1,
						ObservedAt: now,
						CreatedAt:  now,
						UpdatedAt:  now,
					}
					current, err := store.CreateThought(ctx, &item)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					i := 0
					for b.Loop() {
						if operation == "Create" {
							created, err := store.CreateThought(ctx, &item)
							if err != nil {
								b.Fatal(err)
							}
							b.StopTimer()
							if _, err := db.ExecContext(ctx, "DELETE FROM thoughts WHERE thought_id=?", created.ThoughtID); err != nil {
								b.Fatal(err)
							}
							b.StartTimer()
						} else {
							current, err = store.UpdateThought(ctx, "bench", current.ThoughtID, bodies[(i+1)%2], current.Version, now)
							if err != nil {
								b.Fatal(err)
							}
							i++
						}
					}
				})
			}
		})
	}
}
