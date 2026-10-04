//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/coder/websocket"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/browserterm"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

func TestVoiceTUIWorkflow_SQLite(t *testing.T) {
	for _, choice := range []string{"existing subject", "new subject", "unassigned"} {
		t.Run("saves and reopens with "+choice, func(t *testing.T) {
			db, dsn := openMigratedSQLite(t)
			ctx, cancel := context.WithCancel(t.Context())
			runtime, err := application.Open(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close()
			if choice == "existing subject" {
				if _, err := runtime.Subjects().Create(ctx, runtime.LocalUser().UserID, "Voice subject"); err != nil {
					t.Fatal(err)
				}
			}
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			views := make(chan string, 100)
			done := make(chan error, 1)
			go func() {
				done <- browserterm.Serve(ctx, listener, func(session context.Context) tea.Model {
					return browserObservedModel{Model: tui.NewModel(session, runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), runtime.Events(), runtime.TimelineView(), logging.Nop()), views: views}
				}, logging.Nop())
			}()
			defer func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(10 * time.Second):
					t.Error("server did not join work before runtime cleanup")
				}
			}()
			address := "http://" + listener.Addr().String()
			conn, _, err := websocket.Dial(ctx, "ws://"+listener.Addr().String()+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {address}}})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.CloseNow()
			write := func(frame string) {
				t.Helper()
				if err := conn.Write(ctx, websocket.MessageBinary, []byte(frame)); err != nil {
					t.Fatal(err)
				}
			}
			controls := make(chan thoughts.VoiceState, 20)
			readDone := make(chan struct{})
			go func() {
				defer close(readDone)
				for {
					_, frame, err := conn.Read(ctx)
					if err != nil {
						return
					}
					if len(frame) > 0 && frame[0] == 'v' {
						var state thoughts.VoiceState
						if json.Unmarshal(frame[1:], &state) == nil {
							select {
							case controls <- state:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}()
			defer func() { conn.CloseNow(); <-readDone }()
			waitControl := func(want func(thoughts.VoiceState) bool) thoughts.VoiceState {
				t.Helper()
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				for {
					select {
					case state := <-controls:
						if want(state) {
							return state
						}
					case <-timer.C:
						t.Fatal("voice control did not reach expected state")
						return thoughts.VoiceState{}
					}
				}
			}
			waitView := func(text string) {
				t.Helper()
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				var last string
				for {
					select {
					case last = <-views:
						if strings.Contains(last, text) && !strings.Contains(last, "Loading…") {
							return
						}
					case <-timer.C:
						t.Fatalf("view got %q, want %q", last, text)
					}
				}
			}
			write(`2{"cols":100,"rows":35}`)
			waitControl(func(s thoughts.VoiceState) bool { return s.CanOpen })
			write(`v{"action":"open"}`)
			state := waitControl(func(s thoughts.VoiceState) bool { return s.Draft != 0 })
			waitView("Voice thought")
			const body = "voice draft\n界 café 👩‍💻"
			write("p" + body)
			waitView("voice draft")
			// Exercise controls through the actual socket; no audio is sent over it.
			write(`v{"action":"start","draft":1}`)
			waitControl(func(s thoughts.VoiceState) bool { return s.State == "requesting" })
			write("pPRIVATE-NOT-INSERTED")
			write(`v{"action":"stop","draft":1,"recording":1}`)
			waitControl(func(s thoughts.VoiceState) bool { return s.State == "stopping" })
			write(`v{"action":"stopped","draft":1,"recording":1}`)
			waitControl(func(s thoughts.VoiceState) bool { return s.State == "idle" && s.Recording == 1 })
			if state.Draft != 1 {
				t.Fatalf("draft got %d, want 1", state.Draft)
			}
			if choice != "unassigned" {
				write("0\t")
				write("pVoice subject")
				waitView("Create subject…")
				write("0\x1b[B")
				if choice == "existing subject" {
					write("0\x1b[B")
				}
				write("0\r")
				if choice == "new subject" {
					waitView("Create subject\n")
					write("0\r")
				}
				waitView("Subject: Voice subject")
			}
			write("0\x13")
			waitView("Thought 1 •")
			reopened := openSQLite(t, dsn)
			var saved string
			var subjectID *int64
			if err := reopened.QueryRow("SELECT thought,subject_id FROM thoughts WHERE thought_id=1").Scan(&saved, &subjectID); err != nil {
				t.Fatal(err)
			}
			if saved != body {
				t.Fatalf("saved thought got %q, want %q", saved, body)
			}
			if choice == "unassigned" {
				if subjectID != nil {
					t.Fatalf("subject got %v, want nil", *subjectID)
				}
			} else {
				if subjectID == nil {
					t.Fatal("subject got nil, want selected subject")
				}
				var name string
				if err := db.QueryRow("SELECT subject_name FROM subjects WHERE subject_id=?", *subjectID).Scan(&name); err != nil {
					t.Fatal(err)
				}
				if name != "Voice subject" {
					t.Fatalf("subject name got %q, want Voice subject", name)
				}
			}
		})
	}
}
