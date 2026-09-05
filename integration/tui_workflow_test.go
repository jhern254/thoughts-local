//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	appcore "github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/tui"
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

func TestSubjectTUIWorkflow_SQLite(t *testing.T) {
	t.Run("creates a Subject through the TUI", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		ctx := context.Background()
		runtime, err := appcore.Open(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := runtime.Close(); err != nil {
				t.Error(err)
			}
		})

		var model tea.Model = tui.NewModel(ctx, runtime.LocalUser(), runtime.Subjects())
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		model = updateTUIModel(model, tuiKey(tea.KeyEnter))
		for _, value := range "coding" {
			model = updateTUIModel(model, tea.KeyPressMsg(tea.Key{Code: value, Text: string(value)}))
		}
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))

		view := model.View().Content
		for _, want := range []string{"Created subject", "coding"} {
			if !strings.Contains(view, want) {
				t.Fatalf("view %q does not contain %q", view, want)
			}
		}

		var name, userID string
		if err := db.QueryRow("SELECT subject_name, user_id FROM subjects").Scan(&name, &userID); err != nil {
			t.Fatal(err)
		}
		if name != "coding" || userID != runtime.LocalUser().UserID {
			t.Fatalf("got persisted Subject name %q and user ID %q", name, userID)
		}
	})
}

func runTUIModelCommand(t *testing.T, model tea.Model, message tea.Msg) tea.Model {
	t.Helper()
	updated, command := model.Update(message)
	if command == nil {
		t.Fatal("got nil command")
	}
	updated, _ = updated.Update(command())
	return updated
}

func updateTUIModel(model tea.Model, message tea.Msg) tea.Model {
	updated, _ := model.Update(message)
	return updated
}

func tuiKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code})
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
