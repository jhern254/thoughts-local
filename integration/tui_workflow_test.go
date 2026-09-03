//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestTUIWorkflow_SQLite(t *testing.T) {
	t.Run("bootstraps and reuses local user", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)

		stdout, stderr, err := runTUI(t, dsn, "q")
		if err != nil {
			t.Fatalf("run TUI: %v: %s", err, stderr)
		}
		if stdout == "" {
			t.Fatal("TUI produced no terminal output")
		}
		localUser := getLocalUser(t, db)

		stdout, stderr, err = runTUI(t, dsn, "q")
		if err != nil {
			t.Fatalf("run TUI again: %v: %s", err, stderr)
		}
		reused := getLocalUser(t, db)
		if reused.UserID != localUser.UserID {
			t.Fatalf("got second user ID %q, want %q", reused.UserID, localUser.UserID)
		}
		if stdout == "" {
			t.Fatal("second TUI run produced no terminal output")
		}

		var userCount int
		if err := db.QueryRow("SELECT count(*) FROM users WHERE handle = 'local'").Scan(&userCount); err != nil {
			t.Fatal(err)
		}
		if userCount != 1 {
			t.Fatalf("got %d local users, want 1", userCount)
		}
	})
}

func runTUI(t *testing.T, dsn, input string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", "../cmd/thoughts-tui", "--db-dsn", dsn)
	command.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}
