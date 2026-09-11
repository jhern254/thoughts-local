//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	appcore "github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/thought"
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
			t.Fatalf("got TUI output %q, want nonempty terminal output", stdout)
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
			t.Fatalf("got second TUI output %q, want nonempty terminal output", stdout)
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
		const wantName = "coding"

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

		wantUserID := runtime.LocalUser().UserID
		var model tea.Model = tui.NewModel(ctx, runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), logging.Nop())
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		model = updateTUIModel(model, tuiKey(tea.KeyEnter))
		for _, value := range wantName {
			model = updateTUIModel(model, tea.KeyPressMsg(tea.Key{Code: value, Text: string(value)}))
		}
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))

		view := model.View().Content
		for _, want := range []string{"Created subject", wantName} {
			if !strings.Contains(view, want) {
				t.Fatalf("view %q does not contain %q", view, want)
			}
		}

		var name, userID string
		if err := db.QueryRow("SELECT subject_name, user_id FROM subjects").Scan(&name, &userID); err != nil {
			t.Fatal(err)
		}
		if name != wantName {
			t.Fatalf("got persisted Subject name %q, want %q", name, wantName)
		}
		if userID != wantUserID {
			t.Fatalf("got persisted user ID %q, want %q", userID, wantUserID)
		}
	})
	t.Run("renames a subject through the edit screen and persists it", func(t *testing.T) {
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
		item, err := runtime.Subjects().Create(ctx, runtime.LocalUser().UserID, "original")
		if err != nil {
			t.Fatal(err)
		}
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		var model tea.Model = tui.NewModel(ctx, runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), logger)
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		model = updateTUIModel(model, tuiKey(tea.KeyDown))
		model = updateTUIModel(model, tuiKey(tea.KeyDown))
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		model = updateTUIModel(model, tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
		model = updateTUIModel(model, tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
		const name = "PRIVATE-SUBJECT-RENAMED"
		for _, r := range name {
			model = updateTUIModel(model, tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		}
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		if got := model.View().Content; !strings.Contains(got, "Updated subject") || !strings.Contains(got, name) {
			t.Fatalf("got view %q, want renamed detail", got)
		}
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEscape))
		if got := model.View().Content; !strings.Contains(got, name) {
			t.Fatalf("got list %q, want renamed subject", got)
		}
		var stored string
		if err := db.QueryRow("SELECT subject_name FROM subjects WHERE subject_id=?", item.SubjectID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if got, want := stored, name; got != want {
			t.Fatalf("got persisted name %q, want %q", got, want)
		}
		if strings.Contains(logs.String(), name) || strings.Count(logs.String(), "subject updated") != 1 {
			t.Fatalf("got logs %q, want one metadata-only update event", logs.String())
		}
	})
	t.Run("confirmed deletion hides the subject and retains linked thoughts", func(t *testing.T) {
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
		const name = "PRIVATE-SUBJECT-DELETED"
		item, err := runtime.Subjects().Create(ctx, runtime.LocalUser().UserID, name)
		if err != nil {
			t.Fatal(err)
		}
		thoughts := thought.NewService(data.NewSQLiteThoughtStore(db))
		linked, err := thoughts.Create(ctx, runtime.LocalUser().UserID, "keep this thought", &item.SubjectID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		var model tea.Model = tui.NewModel(ctx, runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), logger)
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		model = updateTUIModel(model, tuiKey(tea.KeyDown))
		model = updateTUIModel(model, tuiKey(tea.KeyDown))
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		model = updateTUIModel(model, tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
		if got := model.View().Content; !strings.Contains(got, name) || !strings.Contains(got, "Existing thoughts will be kept") {
			t.Fatalf("got view %q, want delete confirmation", got)
		}
		pending, deleteCommand := model.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"}))
		if deleteCommand == nil {
			t.Fatal("got nil command, want deletion")
		}
		model, refresh := pending.Update(deleteCommand())
		if refresh == nil {
			t.Fatal("got nil command, want list refresh")
		}
		model, _ = model.Update(refresh())
		if got := model.View().Content; strings.Contains(got, name) {
			t.Fatalf("got deleted subject in list %q, want hidden subject", got)
		}
		if _, err := runtime.Subjects().Get(ctx, runtime.LocalUser().UserID, item.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got subject error %v, want not found", err)
		}
		kept, err := thoughts.Get(ctx, runtime.LocalUser().UserID, linked.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		if kept.Thought != "keep this thought" || kept.SubjectID != nil {
			t.Fatalf("got thought %v, want retained content without subject assignment", kept)
		}
		var unlinked bool
		if err := db.QueryRow("SELECT subject_id IS NULL FROM thoughts WHERE thought_id=?", linked.ThoughtID).Scan(&unlinked); err != nil {
			t.Fatal(err)
		}
		if !unlinked {
			t.Fatalf("got stored subject unlinked %v, want true", unlinked)
		}
		if strings.Contains(logs.String(), name) || strings.Count(logs.String(), "subject deleted") != 1 {
			t.Fatalf("got logs %q, want one metadata-only delete event", logs.String())
		}
	})

}

func runTUIModelCommand(t *testing.T, model tea.Model, message tea.Msg) tea.Model {
	t.Helper()
	updated, command := model.Update(message)
	if command == nil {
		t.Fatal("got nil command, want a command to execute")
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
	command.Env = append(os.Environ(), "THOUGHTS_LOG_LEVEL=disabled")
	command.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}
