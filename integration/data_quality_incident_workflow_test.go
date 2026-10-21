//go:build integration

package integration_test

import (
	"database/sql"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestDataQualityIncidentWorkflow_SQLite(t *testing.T) {
	query, err := os.ReadFile("../docs/data-quality/incident-0001-thought-timestamps.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name   string
		cutoff any
		ids    []int64
	}{
		{"unknown cutover retains every record", nil, []int64{1, 2, 3, 4, 5, 6, 7, 8}},
		{"verified cutover includes boundary and all historical cases", int64(100), []int64{1, 2, 3, 4, 5, 6, 7}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			insertUsers(t, db, "u", "other", "deleted")
			_, err := db.ExecContext(t.Context(), `
				UPDATE users SET deleted_at=updated_at WHERE user_id='deleted';
				INSERT INTO subjects(subject_id,user_id,subject_name) VALUES (1,'u','subject');
				INSERT INTO events(event_id,user_id,started_at) VALUES (1,'u',1);
				INSERT INTO thoughts(thought_id,user_id,subject_id,event_id,thought,observed_at,created_at,updated_at,deleted_at) VALUES
				 (1,'u',NULL,NULL,'equal',50,50,50,NULL),
				 (2,'other',NULL,NULL,'explicit observation',10,50,50,NULL),
				 (3,'deleted',NULL,NULL,'deleted owner',50,50,50,NULL),
				 (4,'u',NULL,NULL,'deleted thought',50,50,150,150),
				 (5,'u',1,1,'edited after cutover',50,50,200,NULL),
				 (6,'u',NULL,NULL,'future observation',200,50,50,NULL),
				 (7,'u',NULL,NULL,'boundary second',100,100,100,NULL),
				 (8,'u',NULL,NULL,'post cutover backdated observation',10,101,101,NULL);`)
			if err != nil {
				t.Fatal(err)
			}
			// Execute the documented query itself against the migrated schema.
			selectIDs := "SELECT thought_id FROM (" + strings.TrimSuffix(strings.TrimSpace(string(query)), ";") + ") ORDER BY thought_id"
			rows, err := db.QueryContext(t.Context(), selectIDs, sql.Named("fixed_writers_started_at_unix", scenario.cutoff))
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var ids []int64
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				ids = append(ids, id)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(ids, scenario.ids) {
				t.Fatalf("review thought IDs got %v, want %v", ids, scenario.ids)
			}
		})
	}
}
