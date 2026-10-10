package migrations

import (
	"os"
	"testing"
)

func TestVisual(t *testing.T) {
	t.Run("creates visual directly in its initial migration", func(t *testing.T) {
		db := openMigratedDatabase(t)
		applyMigrationFiles(t, db, "*.down.sql", true)
		applyMigrationFiles(t, db, "000001_create_users_table.up.sql", false)
		createVisual, err := os.ReadFile("000012_create_visual.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(createVisual)); err != nil {
			t.Fatal(err)
		}
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO visual(user_id) VALUES ('user-1')"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("stores one selection per existing user", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO visual(user_id) VALUES ('user-1')"); err != nil {
			t.Fatal(err)
		}
		var darkness int
		if err := db.QueryRow("SELECT darkness FROM visual").Scan(&darkness); err != nil {
			t.Fatal(err)
		}
		if darkness != 70 {
			t.Fatalf("darkness = %d, want 70", darkness)
		}
		if _, err := db.Exec("INSERT INTO visual(user_id) VALUES ('user-1')"); err == nil {
			t.Fatal("duplicate user accepted")
		}
	})
	for _, query := range []string{
		"INSERT INTO visual(user_id) VALUES ('missing')",
		"INSERT INTO visual(user_id,darkness) VALUES ('user-1',96)",
		"INSERT INTO visual(user_id,darkness) VALUES ('user-1',-1)",
		"INSERT INTO visual(user_id,darkness) VALUES ('user-1',0.5)",
		"INSERT INTO visual(user_id,background_asset) VALUES ('user-1','../../private.png')",
	} {
		t.Run("rejects "+query, func(t *testing.T) {
			db := openMigratedDatabase(t)
			insertUsers(t, db)
			if _, err := db.Exec(query); err == nil {
				t.Fatal("invalid settings accepted")
			}
		})
	}
}

func TestBackgroundFraming(t *testing.T) {
	t.Run("defaults to centered unzoomed Fill", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		down, err := os.ReadFile("000013_add_background_framing.down.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(down)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO visual(user_id) VALUES ('user-1')"); err != nil {
			t.Fatal(err)
		}
		up, err := os.ReadFile("000013_add_background_framing.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(up)); err != nil {
			t.Fatal(err)
		}
		var fit string
		var zoom, x, y int
		if err := db.QueryRow("SELECT fit,zoom,position_x,position_y FROM visual").Scan(&fit, &zoom, &x, &y); err != nil {
			t.Fatal(err)
		}
		if fit != "fill" || zoom != 100 || x != 5000 || y != 5000 {
			t.Fatalf("framing = %s %d %d %d, want fill 100 5000 5000", fit, zoom, x, y)
		}
	})
	for _, assignment := range []string{"fit='stretch'", "zoom=99", "zoom=301", "zoom=100.5", "position_x=-1", "position_y=10001", "position_x=0.5"} {
		t.Run("rejects "+assignment, func(t *testing.T) {
			db := openMigratedDatabase(t)
			insertUsers(t, db)
			if _, err := db.Exec("INSERT INTO visual(user_id) VALUES ('user-1')"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("UPDATE visual SET " + assignment); err == nil {
				t.Fatal("accepted invalid framing")
			}
		})
	}
}
