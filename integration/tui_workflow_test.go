//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

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
	t.Run("wires bootstrapped user and Subject service into model", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
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

		model := tui.NewModel(ctx, runtime.LocalUser(), runtime.Subjects())
		if view := model.View().Content; !strings.Contains(view, "Subjects") {
			t.Fatalf("view %q does not contain Subjects", view)
		}

		listed, err := runtime.Subjects().List(ctx, runtime.LocalUser().UserID)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 0 {
			t.Fatalf("got %d initial subjects, want 0", len(listed))
		}

		created, err := runtime.Subjects().Create(ctx, runtime.LocalUser().UserID, "coding")
		if err != nil {
			t.Fatal(err)
		}
		if created.UserID != runtime.LocalUser().UserID {
			t.Fatalf("got owner %q, want %q", created.UserID, runtime.LocalUser().UserID)
		}

		listed, err = runtime.Subjects().List(ctx, runtime.LocalUser().UserID)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 1 || listed[0].SubjectID != created.SubjectID {
			t.Fatalf("got subjects %#v, want created subject %#v", listed, created)
		}

		found, err := runtime.Subjects().Get(ctx, runtime.LocalUser().UserID, created.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		if found.SubjectName != "coding" {
			t.Fatalf("got subject name %q, want coding", found.SubjectName)
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
