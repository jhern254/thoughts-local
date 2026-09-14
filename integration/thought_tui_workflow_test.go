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
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
	thoughtstui "github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

func TestThoughtTUIWorkflow_SQLite(t *testing.T) {
	t.Run("creates unassigned thoughts in Misc without creating a subject", func(t *testing.T) {
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
		var model tea.Model = tui.NewModel(ctx, runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), logging.Nop())
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		model = updateTUIModel(model, tuiKey(tea.KeyDown))
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEnter))
		model = updateTUIModel(model, tuiKey(tea.KeyEnter))
		const body = "unassigned\ncomplete draft"
		model = updateTUIModel(model, tea.PasteMsg{Content: body})
		model, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		if cmd == nil {
			t.Fatal("got nil command, want creation")
		}
		model, changed := model.Update(cmd())
		if changed == nil {
			t.Fatal("got nil command, want count invalidation")
		}
		model, _ = model.Update(changed())
		var gotBody string
		var unassigned bool
		if err := db.QueryRow("SELECT thought, subject_id IS NULL FROM thoughts").Scan(&gotBody, &unassigned); err != nil {
			t.Fatal(err)
		}
		if gotBody != body || !unassigned {
			t.Fatalf("got body %q and unassigned %v, want %q and true", gotBody, unassigned, body)
		}
		var subjects int
		if err := db.QueryRow("SELECT count(*) FROM subjects").Scan(&subjects); err != nil {
			t.Fatal(err)
		}
		if subjects != 0 {
			t.Fatalf("got %d subjects, want 0", subjects)
		}
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEscape))
		model = runTUIModelCommand(t, model, tuiKey(tea.KeyEscape))
		if got := model.View().Content; !strings.Contains(got, "Misc thoughts") || !strings.Contains(got, "1 thought") || !strings.Contains(got, "\n0 subjects\n") {
			t.Fatalf("got view %q, want updated Misc count and zero subjects", got)
		}
	})
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

func TestThoughtTUIMutationsWorkflow_SQLite(t *testing.T) {
	for _, operation := range []string{"update", "delete"} {
		t.Run(operation+" persists from detail and refreshes All Thoughts", func(t *testing.T) {
			db, dsn := openMigratedSQLite(t)
			runtime, err := application.Open(t.Context(), dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := runtime.Close(); err != nil {
					t.Error(err)
				}
			})
			original, err := runtime.Thoughts().Create(t.Context(), runtime.LocalUser().UserID, "PRIVATE-ORIGINAL", nil, time.Unix(1700000000, 0))
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			logger, err := logging.New(&logs, "test", "info")
			if err != nil {
				t.Fatal(err)
			}
			model := thoughtstui.New(t.Context(), runtime.LocalUser().UserID, runtime.Thoughts(), logger)
			// Drain only commands returned by these service operations, never editor timers.
			apply := func(command tea.Cmd) {
				pending := []tea.Cmd{command}
				for len(pending) > 0 {
					cmd := pending[0]
					pending = pending[1:]
					if cmd == nil {
						continue
					}
					msg := cmd()
					if batch, ok := msg.(tea.BatchMsg); ok {
						pending = append(pending, batch...)
						continue
					}
					var next tea.Cmd
					model, next = model.Update(msg)
					if next != nil {
						pending = append(pending, next)
					}
				}
			}
			apply(model.OpenBrowseThoughtsView(runtime.Metrics()))
			model, _ = model.Update(tuiKey(tea.KeyDown))
			var cmd tea.Cmd
			model, cmd = model.Update(tuiKey(tea.KeyEnter))
			apply(cmd)
			wantBody, wantVersion := "PRIVATE-ORIGINAL-UPDATED", int64(2)
			if operation == "update" {
				model, _ = model.Update(tuiKey('e'))
				model, _ = model.Update(tea.PasteMsg{Content: "-UPDATED"})
				model, cmd = model.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
				apply(cmd)
				if !strings.Contains(model.View(), wantBody) {
					t.Fatalf("got detail %q, want updated content", model.View())
				}
				model, cmd = model.Update(tuiKey(tea.KeyEscape))
				apply(cmd)
				if !strings.Contains(model.View(), wantBody) || !strings.Contains(model.View(), "1 thought") {
					t.Fatalf("got browse view %q, want updated preview and count 1", model.View())
				}
			} else {
				wantBody = "PRIVATE-ORIGINAL"
				model, _ = model.Update(tuiKey('d'))
				model, cmd = model.Update(tuiKey('y'))
				apply(cmd)
				if strings.Contains(model.View(), wantBody) || !strings.Contains(model.View(), "0 thoughts") {
					t.Fatalf("got browse view %q, want no deleted preview and count 0", model.View())
				}
			}
			var body string
			var version int64
			var deleted bool
			if err := db.QueryRow(`SELECT thought,version,deleted_at IS NOT NULL FROM thoughts WHERE thought_id=?`, original.ThoughtID).Scan(&body, &version, &deleted); err != nil {
				t.Fatal(err)
			}
			if body != wantBody || version != wantVersion || deleted != (operation == "delete") {
				t.Fatalf("got stored body %q version %d deleted %v, want %q/%d/%v", body, version, deleted, wantBody, wantVersion, operation == "delete")
			}
			var event map[string]any
			if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			wantMessage := "thought updated"
			if operation == "delete" {
				wantMessage = "thought deleted"
			}
			if len(event) != 6 || event["thought_id"] != float64(original.ThoughtID) || event["message"] != wantMessage || strings.Contains(logs.String(), "PRIVATE") {
				t.Fatalf("got logs %v, want one metadata-only %s event", event, wantMessage)
			}
		})
	}
}
