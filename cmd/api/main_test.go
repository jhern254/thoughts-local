package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenDB(t *testing.T) {
	t.Run("enables foreign keys on every pooled connection", func(t *testing.T) {
		var cfg config
		cfg.db.dsn = "file:" + filepath.Join(t.TempDir(), "thoughts.db")
		cfg.db.maxOpenConns = 4
		cfg.db.maxIdleConns = 4
		cfg.db.maxIdleTime = "15m"
		db, err := openDB(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		connections := make([]*sql.Conn, 0, cfg.db.maxOpenConns)
		defer func() {
			for _, connection := range connections {
				connection.Close()
			}
		}()
		for range cfg.db.maxOpenConns {
			connection, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			connections = append(connections, connection)

			var enabled int
			if err := connection.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&enabled); err != nil {
				t.Fatal(err)
			}
			if enabled != 1 {
				t.Fatalf("got foreign_keys=%d", enabled)
			}
		}
	})
}

func TestSQLiteDSNWithForeignKeys(t *testing.T) {
	t.Run("adds pragma as first query parameter", func(t *testing.T) {
		got := sqliteDSNWithForeignKeys("file:data/thoughts.db")
		want := "file:data/thoughts.db?_pragma=foreign_keys(1)"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("preserves existing query parameters", func(t *testing.T) {
		got := sqliteDSNWithForeignKeys("file:data/thoughts.db?mode=rwc")
		want := "file:data/thoughts.db?mode=rwc&_pragma=foreign_keys(1)"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}
