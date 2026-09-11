package migrations

import (
	"strings"
	"testing"
)

func TestMigrations_ThoughtBrowse(t *testing.T) {
	t.Run("creates ordered active-owner index and removes it on rollback", func(t *testing.T) {
		db := openMigratedDatabase(t)
		var definition string
		if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='idx_thoughts_active_user_observed_created_id'`).Scan(&definition); err != nil {
			t.Fatal(err)
		}
		for _, part := range []string{"user_id, observed_at DESC, created_at DESC, thought_id DESC", "WHERE deleted_at IS NULL"} {
			if !strings.Contains(definition, part) {
				t.Fatalf("got index %q, want %q", definition, part)
			}
		}
		applyMigrationFiles(t, db, "*.down.sql", true)
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='idx_thoughts_active_user_observed_created_id'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("got %d indexes after rollback, want 0", count)
		}
		applyMigrationFiles(t, db, "*.up.sql", false)
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='idx_thoughts_active_user_observed_created_id'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("got %d indexes after redeploy, want 1", count)
		}
	})
}
