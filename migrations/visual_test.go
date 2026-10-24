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
		var fit, quality string
		var darkness, zoom, x, y int
		if err := db.QueryRow("SELECT darkness,fit,zoom,position_x,position_y,video_quality FROM visual").Scan(&darkness, &fit, &zoom, &x, &y, &quality); err != nil {
			t.Fatal(err)
		}
		if darkness != 42 || fit != "fill" || zoom != 100 || x != 5000 || y != 5000 {
			t.Fatalf("defaults = %d %s %d %d %d, want 42 fill 100 5000 5000", darkness, fit, zoom, x, y)
		}
		if quality != "efficient" {
			t.Fatalf("video quality = %q, want efficient", quality)
		}
		applyMigrationFiles(t, db, "000012_create_visual.down.sql", false)
		applyMigrationFiles(t, db, "000012_create_visual.up.sql", false)
		if _, err := db.Exec("INSERT INTO visual(user_id,fit,zoom,position_x,position_y) VALUES ('user-1','fit',150,1000,9000)"); err != nil {
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
		if darkness != 42 {
			t.Fatalf("darkness = %d, want 42", darkness)
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
		if _, err := db.Exec("INSERT INTO visual(user_id) VALUES ('user-1')"); err != nil {
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

func TestVisualPlayback(t *testing.T) {
	t.Run("defaults to normal speed with no deliberate pause", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO visual(user_id,background_asset) VALUES ('user-1','0123456789abcdef0123456789abcdef.mp4')"); err != nil {
			t.Fatal(err)
		}
		var speed, paused int
		if err := db.QueryRow("SELECT speed_percent,user_paused FROM visual").Scan(&speed, &paused); err != nil {
			t.Fatal(err)
		}
		if speed != 100 || paused != 0 {
			t.Fatalf("got %d/%d, want 100/0", speed, paused)
		}
	})
	for _, assignment := range []string{"speed_percent=24", "speed_percent=101", "speed_percent=50.5", "user_paused=2", "user_paused=-1", "user_paused=0.5"} {
		t.Run("rejects "+assignment, func(t *testing.T) {
			db := openMigratedDatabase(t)
			insertUsers(t, db)
			if _, err := db.Exec("INSERT INTO visual(user_id) VALUES ('user-1')"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("UPDATE visual SET " + assignment); err == nil {
				t.Fatal("invalid playback preference accepted")
			}
		})
	}
}
