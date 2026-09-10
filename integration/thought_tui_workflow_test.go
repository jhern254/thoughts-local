//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
)

func TestThoughtTUIWorkflow_SQLite(t *testing.T) {
	t.Run("rejects unsupported input without saving a silently shortened thought", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		ctx := context.Background()
		runtime, err := application.Open(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := runtime.Close(); err != nil {
				t.Error(err)
			}
		})
		if _, err := runtime.Subjects().Create(ctx, runtime.LocalUser().UserID, "coding"); err != nil {
			t.Fatal(err)
		}
		var model tea.Model = tui.NewModel(ctx, runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), logging.Nop())
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		model = updateTUIModel(model, tuiKey(tea.KeyDown))
		model, cmd := model.Update(tuiKey(tea.KeyEnter))
		if cmd == nil {
			t.Fatal("got nil command, want subject get")
		}
		model, cmd = model.Update(cmd())
		if cmd == nil {
			t.Fatal("got nil command, want thought list")
		}
		model, _ = model.Update(cmd())
		model = updateTUIModel(model, tuiKey(tea.KeyEnter))
		const draft = "keep my complete draft"
		model = updateTUIModel(model, tea.PasteMsg{Content: draft})
		model = updateTUIModel(model, tea.KeyPressMsg(tea.Key{Code: 'g', Mod: tea.ModCtrl}))
		model = updateTUIModel(model, tea.PasteMsg{Content: strings.Repeat("x\n", 499999) + "xy"})
		if got := model.View().Content; !strings.Contains(got, "Input rejected") || !strings.Contains(got, "Draft unchanged") {
			t.Fatalf("got view %q, want explicit rejection and unchanged-draft feedback", got)
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM thoughts").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("got %d persisted thoughts after rejected input, want 0", count)
		}
		model = runTUIModelCommand(t, model, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		var body string
		if err := db.QueryRow("SELECT thought FROM thoughts").Scan(&body); err != nil {
			t.Fatal(err)
		}
		if body != draft {
			t.Fatalf("got saved text %q, want original draft %q", body, draft)
		}
	})
	t.Run("creates a thought under a subject and refreshes TUI and CLI counts", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		ctx := context.Background()
		runtime, err := application.Open(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := runtime.Close(); err != nil {
				t.Error(err)
			}
		})
		subject, err := runtime.Subjects().Create(ctx, runtime.LocalUser().UserID, "coding")
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
		if got := model.View().Content; !strings.Contains(got, "0 thoughts") {
			t.Fatalf("got view %q, want zero count", got)
		}
		model = updateTUIModel(model, tuiKey(tea.KeyDown))
		model, cmd := model.Update(tuiKey(tea.KeyEnter))
		if cmd == nil {
			t.Fatal("got nil command, want subject get")
		}
		model, cmd = model.Update(cmd())
		if cmd == nil {
			t.Fatal("got nil command, want thought list")
		}
		model, _ = model.Update(cmd())
		model = updateTUIModel(model, tuiKey(tea.KeyEnter))
		const body = "private-thought-marker\nsecond line"
		model = updateTUIModel(model, tea.PasteMsg{Content: body})
		model, cmd = model.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		if cmd == nil {
			t.Fatal("got nil command, want thought create")
		}
		model, cmd = model.Update(cmd())
		if cmd == nil {
			t.Fatal("got nil command, want count invalidation")
		}
		model, _ = model.Update(cmd())
		if got := model.View().Content; !strings.Contains(got, "private-thought-marker") {
			t.Fatalf("got view %q, want intentional thought display", got)
		}
		var gotBody string
		var gotSubjectID, thoughtID int64
		if err := db.QueryRow("SELECT thought_id,subject_id,thought FROM thoughts").Scan(&thoughtID, &gotSubjectID, &gotBody); err != nil {
			t.Fatal(err)
		}
		if gotBody != body || gotSubjectID != subject.SubjectID {
			t.Fatalf("got text %q and subject %d, want %q and %d", gotBody, gotSubjectID, body, subject.SubjectID)
		}
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEscape))
		model = updateTUIModel(model, tuiKey(tea.KeyDown))
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		if got := model.View().Content; !strings.Contains(got, "second line") {
			t.Fatalf("got detail %q, want retrieved full text", got)
		}
		model = updateTUIModel(model, tuiKey(tea.KeyEscape))
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEscape))
		if got := model.View().Content; !strings.Contains(got, "1 thought") {
			t.Fatalf("got subject list %q, want refreshed count", got)
		}
		var event map[string]any
		if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if len(event) != 6 || event["thought_id"] != float64(thoughtID) || event["message"] != "thought created" || event["level"] != "info" || event["application"] != "test" || event["caller"] == nil || event["time"] == nil {
			t.Fatalf("got event %v, want approved thought mutation fields", event)
		}
		if strings.Contains(logs.String(), "private-thought-marker") {
			t.Fatalf("got logs %q, want no authored text", &logs)
		}
		command := exec.CommandContext(ctx, "go", "run", "../cmd/thoughts", "--db-dsn", dsn, "metrics", "thought-counts")
		command.Env = append(os.Environ(), "THOUGHTS_LOG_LEVEL=disabled")
		var out, stderr bytes.Buffer
		command.Stdout = &out
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("got CLI error %v and stderr %q, want success", err, &stderr)
		}
		if got, want := out.String(), fmt.Sprintf("%d\t1\n", subject.SubjectID); got != want {
			t.Fatalf("got CLI output %q, want %q", got, want)
		}
		if got := stderr.String(); got != "" {
			t.Fatalf("got stderr %q, want empty", got)
		}
	})
}
