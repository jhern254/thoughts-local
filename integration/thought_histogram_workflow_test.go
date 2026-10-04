//go:build integration

package integration_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestThoughtHistogramWorkflow_SQLite(t *testing.T) {
	t.Run("counts full Unicode bodies including spaces and embedded NUL", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		if _, err := db.Exec(`INSERT INTO users(user_id) VALUES ('u')`); err != nil {
			t.Fatal(err)
		}
		body := strings.Repeat("界 🙂 ", 30) + "a\x00b"
		if _, err := db.Exec(`INSERT INTO thoughts(user_id,thought,observed_at) VALUES ('u',?,100)`, body); err != nil {
			t.Fatal(err)
		}
		view, err := data.NewSQLiteThoughtStore(db).BrowseThoughtsViewInRange(t.Context(), "u", time.Unix(100, 0), time.Unix(200, 0), data.ThoughtSummaryViewRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := view.Items[0].CharacterCount, int64(utf8.RuneCountInString(body)); got != want {
			t.Fatalf("got character count %d, want %d", got, want)
		}
		if len([]rune(view.Items[0].Preview)) != 81 {
			t.Fatal("preview is no longer bounded")
		}
		stats, err := data.NewSQLiteMetricsStore(db).ThoughtStatsInRange(t.Context(), "u", time.Unix(100, 0), time.Unix(200, 0))
		if err != nil || stats.Count != 1 || stats.MaxCharacters != int64(utf8.RuneCountInString(body)) {
			t.Fatalf("got stats %+v, %v; want one thought and full body length", stats, err)
		}
	})
	t.Run("maximum includes older pages and excludes invisible or out of interval thoughts", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		for _, q := range []string{
			`INSERT INTO users(user_id) VALUES ('u'),('other'),('deleted')`,
			`INSERT INTO thoughts(user_id,thought,observed_at) VALUES ('u','longest visible',100),('u','outside interval and longer',200),('other','foreign and much longer than visible',150),('deleted','hidden user',150)`,
			`INSERT INTO thoughts(user_id,thought,observed_at,deleted_at) VALUES ('u','deleted and much longer than visible',150,unixepoch())`,
			`UPDATE users SET deleted_at=unixepoch(),updated_at=unixepoch() WHERE user_id='deleted'`,
		} {
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 60; i++ {
			if _, err := db.Exec(`INSERT INTO thoughts(user_id,thought,observed_at) VALUES ('u','short',?)`, 101+i); err != nil {
				t.Fatal(err)
			}
		}
		store := data.NewSQLiteMetricsStore(db)
		stats, err := store.ThoughtStatsInRange(t.Context(), "u", time.Unix(100, 0), time.Unix(200, 0))
		if err != nil || stats.Count != 61 || stats.MaxCharacters != 15 {
			t.Fatalf("got stats %+v, %v; want count 61, max 15", stats, err)
		}
		for _, user := range []string{"u", "deleted"} {
			from := time.Unix(200, 0)
			until := from
			if user == "deleted" {
				from = time.Unix(100, 0)
			}
			empty, err := store.ThoughtStatsInRange(t.Context(), user, from, until)
			if err != nil || empty != (data.ThoughtIntervalStats{}) {
				t.Fatalf("got empty stats %+v, %v; want zero", empty, err)
			}
		}
	})
}
