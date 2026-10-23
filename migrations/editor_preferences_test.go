package migrations

import "testing"

func TestEditorPreferences(t *testing.T) {
	t.Run("defaults to steady unhighlighted editing and two second pauses", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO editor_preferences(user_id) VALUES ('user-1')"); err != nil {
			t.Fatal(err)
		}
		var highlight, pause int
		if err := db.QueryRow("SELECT highlight_current_line,speech_pause_ms FROM editor_preferences").Scan(&highlight, &pause); err != nil {
			t.Fatal(err)
		}
		if highlight != 0 || pause != 2000 {
			t.Fatalf("got %d %d, want 0 2000", highlight, pause)
		}
	})
	for _, value := range []string{"-1", "1", "499", "10001", "500.5", "NULL"} {
		t.Run("rejects pause "+value, func(t *testing.T) {
			db := openMigratedDatabase(t)
			insertUsers(t, db)
			if _, err := db.Exec("INSERT INTO editor_preferences(user_id,speech_pause_ms) VALUES ('user-1'," + value + ")"); err == nil {
				t.Fatal("accepted invalid pause")
			}
		})
	}
}

func TestEditorPreferences_Ownership(t *testing.T) {
	t.Run("enforces user ownership and deletes preferences with their user", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec("INSERT INTO editor_preferences(user_id) VALUES ('missing')"); err == nil {
			t.Fatal("accepted unknown user")
		}
		if _, err := db.Exec("INSERT INTO editor_preferences(user_id,highlight_current_line) VALUES ('user-1',2)"); err == nil {
			t.Fatal("accepted nonboolean highlight")
		}
		if _, err := db.Exec("INSERT INTO editor_preferences(user_id) VALUES ('user-1')"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO editor_preferences(user_id) VALUES ('user-1')"); err == nil {
			t.Fatal("accepted duplicate user preferences")
		}
		if _, err := db.Exec("DELETE FROM users WHERE user_id='user-1'"); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM editor_preferences").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("got %d preferences after user deletion, want 0", count)
		}
	})
}
