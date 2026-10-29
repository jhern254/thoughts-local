//go:build integration

package integration_test

import (
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func TestVisualSchemaWorkflow_SQLite(t *testing.T) {
	for _, column := range []string{"border_weight", "heavier_text", "text_size", "high_contrast"} {
		t.Run("detects missing "+column+" even without settings rows", func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			if _, err := db.ExecContext(t.Context(), "ALTER TABLE visual DROP COLUMN "+column); err != nil {
				t.Fatal(err)
			}
			_, err := data.NewSQLiteVisualStore(db).Load(t.Context(), "owner")
			if !errors.Is(err, data.ErrVisualSchemaMismatch) {
				t.Fatalf("load: got %v, want schema mismatch", err)
			}
		})
	}
	t.Run("detects current version with legacy columns and recovers after explicit repair", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		insertUsers(t, db, "owner")
		if _, err := db.ExecContext(t.Context(), `CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, dirty BOOLEAN NOT NULL); INSERT INTO schema_migrations VALUES(14,false); INSERT INTO visual(user_id,darkness) VALUES('owner',70)`); err != nil {
			t.Fatal(err)
		}
		columns := []struct{ name, definition string }{
			{"border_weight", "TEXT NOT NULL DEFAULT 'normal' CHECK (border_weight IN ('normal','heavy'))"},
			{"heavier_text", "INTEGER NOT NULL DEFAULT 0 CHECK (typeof(heavier_text) = 'integer' AND heavier_text IN (0,1))"},
			{"text_size", "TEXT NOT NULL DEFAULT 'standard' CHECK (text_size IN ('standard','larger'))"},
			{"high_contrast", "INTEGER NOT NULL DEFAULT 0 CHECK (typeof(high_contrast) = 'integer' AND high_contrast IN (0,1))"},
		}
		store := data.NewSQLiteVisualStore(db)
		service := visual.NewService(store, "owner", t.TempDir())
		settings, err := service.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, column := range columns {
			if _, err := db.ExecContext(t.Context(), "ALTER TABLE visual DROP COLUMN "+column.name); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := service.Load(t.Context()); !errors.Is(err, data.ErrVisualSchemaMismatch) || !errors.Is(err, visual.ErrSettings) {
			t.Fatalf("load: got %v, want schema and settings identities", err)
		}
		if err := store.Save(t.Context(), "owner", settings); !errors.Is(err, data.ErrVisualSchemaMismatch) {
			t.Fatalf("save: got %v, want schema mismatch", err)
		}
		for _, column := range columns {
			if _, err := db.ExecContext(t.Context(), "ALTER TABLE visual ADD COLUMN "+column.name+" "+column.definition); err != nil {
				t.Fatal(err)
			}
		}
		got, err := service.Load(t.Context())
		if err != nil || got != settings {
			t.Fatalf("repaired load: got %+v, %v, want %+v", got, err, settings)
		}
		got.Darkness = 60
		if err := store.Save(t.Context(), "owner", got); err != nil {
			t.Fatal(err)
		}
		var version int
		var dirty bool
		if err := db.QueryRowContext(t.Context(), "SELECT version,dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
			t.Fatal(err)
		}
		if version != 14 || dirty {
			t.Fatalf("migration: got %d, %v, want 14, false", version, dirty)
		}
	})
}
