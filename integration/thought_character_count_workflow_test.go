//go:build integration

package integration_test

import (
	"errors"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestThoughtCharacterCountWorkflow_SQLite(t *testing.T) {
	t.Run("generated counts include Unicode whitespace and NUL through creates and updates", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		bodies := []string{"ASCII", "e\u0301界😀", "  line one\nline two\t ", "Ā\x00😀\x00tail", "a\x00\x00b", "𐀀\x00end"}
		for _, body := range bodies {
			item, err := service.Create(t.Context(), "owner", body, nil, time.Unix(100, 0))
			if err != nil {
				t.Fatal(err)
			}
			for _, replacement := range []string{body, body + "\n界\x00more"} {
				if replacement != body {
					item, err = service.Update(t.Context(), "owner", item.ThoughtID, replacement, item.Version)
					if err != nil {
						t.Fatal(err)
					}
				}
				var got int64
				// Force the active index too: the persisted index value must change with text.
				for _, query := range []string{
					"SELECT character_count FROM thoughts WHERE thought_id=?",
					"SELECT character_count FROM thoughts INDEXED BY idx_thoughts_active_user_observed_created_id WHERE user_id='owner' AND deleted_at IS NULL AND thought_id=?",
				} {
					if err := db.QueryRow(query, item.ThoughtID).Scan(&got); err != nil {
						t.Fatal(err)
					}
					if want := int64(utf8.RuneCountInString(replacement)); got != want {
						t.Fatalf("character count: got %d, want %d for %q", got, want, replacement)
					}
				}
			}
		}
		if _, err := db.Exec("UPDATE thoughts SET character_count=0 WHERE user_id='owner'"); err == nil {
			t.Fatal("generated count override: got success, want rejection")
		}
	})

	t.Run("failed writes leave counts intact and soft deletion retains the derived value", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		service := thought.NewService(data.NewSQLiteThoughtStore(db))
		item, err := service.Create(t.Context(), "owner", "initial", nil, time.Unix(100, 0))
		if err != nil {
			t.Fatal(err)
		}
		body := "Ā\x00new\n界😀"
		updated, err := service.Update(t.Context(), "owner", item.ThoughtID, body, item.Version)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Update(t.Context(), "owner", item.ThoughtID, "stale", item.Version); !errors.Is(err, data.ErrVersionConflict) {
			t.Fatalf("stale update: got %v, want version conflict", err)
		}
		store := data.NewSQLiteThoughtStore(db)
		page, err := store.BrowseThoughtsViewInRange(t.Context(), "owner", time.Unix(100, 0), time.Unix(101, 0), data.ThoughtSummaryViewRequest{})
		want := int64(utf8.RuneCountInString(body))
		if err != nil || len(page.Items) != 1 || page.Items[0].CharacterCount != want {
			t.Fatalf("page after stale update: got %+v, %v; want one thought with length %d", page, err, want)
		}
		if err := service.Delete(t.Context(), "owner", item.ThoughtID, updated.Version); err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := db.QueryRow("SELECT character_count FROM thoughts WHERE thought_id=?", item.ThoughtID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("deleted count: got %d, want %d", count, want)
		}
		stats, err := data.NewSQLiteMetricsStore(db).ThoughtStatsInRange(t.Context(), "owner", time.Unix(100, 0), time.Unix(101, 0))
		if err != nil || stats != (data.ThoughtIntervalStats{}) {
			t.Fatalf("deleted interval: got %+v, %v; want zero stats", stats, err)
		}
	})

	t.Run("raw SQL edits maintain indexed counts across connections", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		body := "seed Ā\x00😀\n "
		if _, err := db.Exec("INSERT INTO thoughts(thought_id,user_id,thought) VALUES(1,'owner',?)", body); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("UPDATE thoughts SET thought=? WHERE thought_id=1", body+"edited"); err != nil {
			t.Fatal(err)
		}
		reopened := openSQLite(t, dsn)
		var got int64
		if err := reopened.QueryRow("SELECT character_count FROM thoughts INDEXED BY idx_thoughts_active_user_observed_created_id WHERE user_id='owner' AND deleted_at IS NULL AND thought_id=1").Scan(&got); err != nil {
			t.Fatal(err)
		}
		if want := int64(utf8.RuneCountInString(body + "edited")); got != want {
			t.Fatalf("reopened indexed count: got %d, want %d", got, want)
		}
	})
}
