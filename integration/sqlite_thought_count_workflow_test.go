//go:build integration

package integration_test

import (
	"fmt"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/metrics"
)

func TestThoughtCountWorkflow_SQLite(t *testing.T) {
	for _, count := range []int{0, 1, 230} {
		t.Run(fmt.Sprintf("counts %d active assigned and Misc thoughts", count), func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u", "other", "deleted")
			if _, err := db.Exec(`INSERT INTO subjects (subject_id,user_id,subject_name) VALUES (1,'u','subject');
				INSERT INTO thoughts (user_id,thought) VALUES ('other','foreign'),('deleted','retained');
				INSERT INTO thoughts (user_id,thought,deleted_at) VALUES ('u','removed',unixepoch());
				UPDATE users SET deleted_at=unixepoch(),updated_at=unixepoch() WHERE user_id='deleted'`); err != nil {
				t.Fatal(err)
			}
			for i := range count {
				var subjectID *int64
				if i%2 == 0 {
					id := int64(1)
					subjectID = &id
				}
				if _, err := db.Exec(`INSERT INTO thoughts (user_id,subject_id,thought) VALUES ('u',?,'body')`, subjectID); err != nil {
					t.Fatal(err)
				}
			}
			service := metrics.NewService(data.NewSQLiteMetricsStore(db))
			for userID, want := range map[string]int64{"u": int64(count), "other": 1, "deleted": 0, "missing": 0} {
				got, err := service.CountThoughts(t.Context(), userID)
				if err != nil || got != want {
					t.Fatalf("count for %s: got %d, %v; want %d, nil", userID, got, err, want)
				}
			}
		})
	}
}
