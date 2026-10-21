package migrations

import "testing"

func TestVisualQuality(t *testing.T) {
	t.Run("defaults existing visual settings to efficient without changing selection", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO visual(user_id,background_asset,darkness) VALUES ('user-1','0123456789abcdef0123456789abcdef.mp4',42)"); err != nil {
			t.Fatal(err)
		}
		var quality, asset string
		var darkness int
		if err := db.QueryRow("SELECT video_quality,background_asset,darkness FROM visual WHERE user_id='user-1'").Scan(&quality, &asset, &darkness); err != nil {
			t.Fatal(err)
		}
		if quality != "efficient" || asset != "0123456789abcdef0123456789abcdef.mp4" || darkness != 42 {
			t.Fatalf("settings=%q %q %d, want efficient, preserved selection, 42", quality, asset, darkness)
		}
	})
}

func TestVisualQualityUpgrade(t *testing.T) {
	t.Run("adds default without rewriting existing preferences and rolls back independently", func(t *testing.T) {
		db := openMigratedDatabase(t)
		applyMigrationFiles(t, db, "000014_video_quality.down.sql", false)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO visual(user_id,background_asset,darkness,fit,zoom,position_x,position_y,speed_percent,user_paused) VALUES ('user-1','0123456789abcdef0123456789abcdef.mp4',42,'fit',150,1000,9000,50,1)"); err != nil {
			t.Fatal(err)
		}
		applyMigrationFiles(t, db, "000014_video_quality.up.sql", false)
		var quality, asset, fit string
		var darkness, zoom, x, y, speed, paused int
		if err := db.QueryRow("SELECT video_quality,background_asset,darkness,fit,zoom,position_x,position_y,speed_percent,user_paused FROM visual").Scan(&quality, &asset, &darkness, &fit, &zoom, &x, &y, &speed, &paused); err != nil {
			t.Fatal(err)
		}
		if quality != "efficient" || asset != "0123456789abcdef0123456789abcdef.mp4" || darkness != 42 || fit != "fit" || zoom != 150 || x != 1000 || y != 9000 || speed != 50 || paused != 1 {
			t.Fatalf("upgrade changed existing settings: %q %q %d %s %d %d %d %d %d", quality, asset, darkness, fit, zoom, x, y, speed, paused)
		}
		applyMigrationFiles(t, db, "000014_video_quality.down.sql", false)
		var count int
		if err := db.QueryRow("SELECT count(*) FROM visual WHERE darkness=42 AND speed_percent=50").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("retained rows=%d, want 1", count)
		}
	})
	for _, quality := range []string{"unknown", "", "High quality"} {
		t.Run("rejects invalid tier "+quality, func(t *testing.T) {
			db := openMigratedDatabase(t)
			insertUsers(t, db)
			if _, err := db.Exec("INSERT INTO visual(user_id,video_quality) VALUES ('user-1',?)", quality); err == nil {
				t.Fatal("invalid quality accepted")
			}
		})
	}
}
