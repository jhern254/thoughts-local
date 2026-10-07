package migrations

import "testing"

func TestBrowserAppearance(t *testing.T) {
	t.Run("stores one selection per existing user", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO browser_appearance(user_id) VALUES ('user-1')"); err != nil {
			t.Fatal(err)
		}
		var darkness int
		if err := db.QueryRow("SELECT darkness FROM browser_appearance").Scan(&darkness); err != nil {
			t.Fatal(err)
		}
		if darkness != 70 {
			t.Fatalf("darkness = %d, want 70", darkness)
		}
		if _, err := db.Exec("INSERT INTO browser_appearance(user_id) VALUES ('user-1')"); err == nil {
			t.Fatal("duplicate user accepted")
		}
	})
	for _, query := range []string{
		"INSERT INTO browser_appearance(user_id) VALUES ('missing')",
		"INSERT INTO browser_appearance(user_id,darkness) VALUES ('user-1',96)",
		"INSERT INTO browser_appearance(user_id,darkness) VALUES ('user-1',-1)",
		"INSERT INTO browser_appearance(user_id,darkness) VALUES ('user-1',0.5)",
		"INSERT INTO browser_appearance(user_id,background_asset) VALUES ('user-1','../../private.png')",
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
