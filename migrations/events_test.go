package migrations

import (
	"database/sql"
	"testing"
	"time"
)

func TestEventLifecycleWorkflow_SQLite(t *testing.T) {
	t.Run("defaults to an ongoing event starting now", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		before := time.Now().Unix()
		if _, err := db.Exec("INSERT INTO events(user_id) VALUES ('user-1')"); err != nil {
			t.Fatal(err)
		}
		var start int64
		var end sql.NullInt64
		if err := db.QueryRow("SELECT started_at, ended_at FROM events").Scan(&start, &end); err != nil {
			t.Fatal(err)
		}
		if after := time.Now().Unix(); start < before || start > after {
			t.Fatalf("got start %d, want between %d and %d", start, before, after)
		}
		if end.Valid {
			t.Fatalf("got end %d, want NULL", end.Int64)
		}
	})

	for _, end := range []int64{100, 200} {
		name := "accepts a completed interval"
		if end == 100 {
			name = "accepts equal start and end timestamps"
		}
		t.Run(name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			insertUsers(t, db)
			if _, err := db.Exec("INSERT INTO events(user_id, started_at, ended_at) VALUES ('user-1', 100, ?)", end); err != nil {
				t.Fatalf("got completed event error %v, want nil", err)
			}
		})
	}

	for _, tc := range []struct {
		name       string
		start, end any
	}{
		{"rejects a missing start", nil, nil},
		{"rejects a fractional start", 100.5, nil},
		{"rejects a text start", "invalid", nil},
		{"rejects a blob start", []byte("100"), nil},
		{"rejects a fractional end", 100, 200.5},
		{"rejects a text end", 100, "invalid"},
		{"rejects a blob end", 100, []byte("200")},
		{"rejects an end before the start", 100, 99},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			insertUsers(t, db)
			if _, err := db.Exec("INSERT INTO events(user_id, started_at, ended_at) VALUES ('user-1', ?, ?)", tc.start, tc.end); err == nil {
				t.Fatal("got invalid interval insert error nil, want constraint error")
			}
			if _, err := db.Exec("INSERT INTO events(user_id, started_at, ended_at) VALUES ('user-1', 100, 200)"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("UPDATE events SET started_at = ?, ended_at = ?", tc.start, tc.end); err == nil {
				t.Fatal("got invalid interval update error nil, want constraint error")
			}
		})
	}

	t.Run("allows each user an ongoing event alongside completed events", func(t *testing.T) {
		db := openMigratedDatabase(t)
		insertUsers(t, db)
		if _, err := db.Exec(`INSERT INTO events(user_id, started_at, ended_at) VALUES
			('user-1', 100, 200), ('user-1', 200, 300),
			('user-1', 300, NULL), ('user-2', 300, NULL)`); err != nil {
			t.Fatalf("got independent event insert error %v, want nil", err)
		}
	})

	for _, tc := range []struct{ name, setup, change string }{
		{"rejects a second ongoing event", "",
			"INSERT INTO events(user_id, started_at) VALUES ('user-1', 200)"},
		{"rejects reopening another event while one is ongoing",
			"INSERT INTO events(event_id,user_id,started_at,ended_at) VALUES (2,'user-1',50,100)",
			"UPDATE events SET ended_at=NULL WHERE event_id=2"},
		{"rejects transferring an ongoing event to a user already tracking one",
			"INSERT INTO events(event_id,user_id,started_at) VALUES (2,'user-2',100)",
			"UPDATE events SET user_id='user-1' WHERE event_id=2"},
		{"rejects restoring another ongoing event",
			"INSERT INTO events(event_id,user_id,started_at,created_at,updated_at,deleted_at) VALUES (2,'user-1',100,100,200,200)",
			"UPDATE events SET deleted_at=NULL WHERE event_id=2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			insertUsers(t, db)
			if _, err := db.Exec("INSERT INTO events(event_id,user_id,started_at) VALUES (1,'user-1',100)"); err != nil {
				t.Fatal(err)
			}
			if tc.setup != "" {
				if _, err := db.Exec(tc.setup); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(tc.change); err == nil {
				t.Fatal("got conflicting ongoing event error nil, want uniqueness error")
			}
		})
	}

	for _, tc := range []struct{ name, change string }{
		{"ending an event allows a new ongoing event", "UPDATE events SET ended_at=200 WHERE event_id=1"},
		{"soft deleting an event allows a new ongoing event", "UPDATE events SET deleted_at=updated_at WHERE event_id=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			insertUsers(t, db)
			if _, err := db.Exec("INSERT INTO events(event_id,user_id,started_at) VALUES (1,'user-1',100)"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.change); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO events(user_id,started_at) VALUES ('user-1',200)"); err != nil {
				t.Fatalf("got replacement ongoing event error %v, want nil", err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM events").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if got, want := count, 2; got != want {
				t.Fatalf("got retained event count %d, want %d", got, want)
			}
		})
	}
}
