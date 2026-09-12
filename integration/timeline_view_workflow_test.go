//go:build integration

package integration_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestTimelineViewWorkflow_SQLite(t *testing.T) {
	t.Run("counts whole intervals including Misc and excluding invisible thoughts", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		for _, query := range []string{
			`INSERT INTO users(user_id) VALUES ('u'), ('other'), ('deleted')`,
			`INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','Books')`,
			`INSERT INTO events(event_id,user_id,started_at,ended_at) VALUES (1,'u',100,200),(2,'u',200,200),(3,'u',200,NULL),(4,'other',100,200),(5,'deleted',100,200)`,
			`INSERT INTO thoughts(user_id,subject_id,thought,observed_at) VALUES ('u',1,'assigned',100),('u',NULL,'Misc',199),('u',NULL,'current',200),('u',NULL,'future',301),('other',NULL,'foreign',150),('deleted',NULL,'hidden',150)`,
			`INSERT INTO thoughts(user_id,thought,observed_at,deleted_at) VALUES ('u','deleted',150,unixepoch())`,
			`UPDATE users SET deleted_at=unixepoch(), updated_at=unixepoch() WHERE user_id='deleted'`,
		} {
			if _, err := db.Exec(query); err != nil {
				t.Fatal(err)
			}
		}
		s := data.NewSQLiteMetricsStore(db)
		counts, err := s.ThoughtCountsByEvent(t.Context(), "u", time.Unix(150, 0), time.Unix(250, 0), time.Unix(301, 0))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(counts) != "[{1 2} {2 0} {3 1}]" {
			t.Fatalf("got counts %v, want events 1:2, 2:0, 3:1", counts)
		}
		for _, user := range []string{"u", "deleted"} {
			got, err := s.CountThoughtsInRange(t.Context(), user, time.Unix(100, 0), time.Unix(200, 0))
			want := int64(2)
			if user == "deleted" {
				want = 0
			}
			if err != nil || got != want {
				t.Fatalf("got %s count %d, %v; want %d, nil", user, got, err, want)
			}
		}
	})
	t.Run("browses bounded previews in both directions without crossing interval", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		if _, err := db.Exec(`INSERT INTO users(user_id) VALUES ('u')`); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 232; i++ {
			if _, err := db.Exec(`INSERT INTO thoughts(user_id,thought,observed_at,created_at,updated_at) VALUES ('u',?,?,?,?)`, strings.Repeat("界", 200), i+100, i+100, i+100); err != nil {
				t.Fatal(err)
			}
		}
		s := data.NewSQLiteThoughtStore(db)
		from, until := time.Unix(101, 0), time.Unix(331, 0)
		view, err := s.BrowseThoughtsViewInRange(t.Context(), "u", from, until, data.ThoughtViewRequest{})
		if err != nil || len(view.Items) != 50 || !view.More || view.Items[0].ObservedAt.Unix() != 330 || len([]rune(view.Items[0].Preview)) != 81 {
			t.Fatalf("got initial view %+v, %v; want 50 bounded newest summaries", view, err)
		}
		cursor := view.Items[49].Cursor()
		older, err := s.BrowseThoughtsViewInRange(t.Context(), "u", from, until, data.ThoughtViewRequest{Cursor: &cursor})
		if err != nil || len(older.Items) != 50 || older.Items[0].ObservedAt.Unix() != 280 {
			t.Fatalf("got older view %+v, %v; want next 50", older, err)
		}
		cursor = older.Items[0].Cursor()
		newer, err := s.BrowseThoughtsViewInRange(t.Context(), "u", from, until, data.ThoughtViewRequest{Cursor: &cursor, Direction: data.ThoughtsNewer})
		if err != nil || len(newer.Items) != 50 || newer.More || newer.Items[0].ObservedAt.Unix() != 330 {
			t.Fatalf("got newer view %+v, %v; want initial 50", newer, err)
		}
		latest, err := s.LatestThoughtInRange(t.Context(), "u", from, until)
		if err != nil || latest == nil || latest.ThoughtID != view.Items[0].ThoughtID {
			t.Fatalf("got latest %+v, %v; want first summary", latest, err)
		}
		empty, err := s.LatestThoughtInRange(t.Context(), "u", from, from)
		if err != nil || empty != nil {
			t.Fatalf("got empty %+v, %v; want nil, nil", empty, err)
		}
	})
}
