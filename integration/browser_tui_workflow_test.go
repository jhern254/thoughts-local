//go:build integration

package integration_test

import (
	"context"
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

// Observes the actual model running behind the socket. Rendering/transport
// assertions live in browserterm and browser tests, not SQLite workflows.
type browserObservedModel struct {
	tea.Model
	views chan string
}

func (m browserObservedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	return m, cmd
}
func (m browserObservedModel) VoiceState() thoughts.VoiceState {
	return m.Model.(tui.Model).VoiceState()
}
func (m browserObservedModel) View() tea.View {
	view := m.Model.View()
	select {
	case m.views <- view.Content:
	default:
	}
	return view
}

func TestBrowserTUIWorkflow_SQLite(t *testing.T) {
	t.Run("saves multiline Unicode through the browser and reads it after reconnect", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runtime, err := application.Open(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer runtime.Close()
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		instances := make(chan chan string, 2)
		done := make(chan error, 1)
		go func() {
			done <- browserterm.Serve(ctx, listener, func(session context.Context) tea.Model {
				views := make(chan string, 100)
				instances <- views
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
				t.Error("server did not release shared runtime")
			}
		}()
		address := "http://" + listener.Addr().String()
		connect := func() (*websocket.Conn, chan string, chan struct{}) {
			t.Helper()
			conn, _, err := websocket.Dial(ctx, "ws://"+listener.Addr().String()+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {address}}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { conn.CloseNow() })
			if err := conn.Write(ctx, websocket.MessageBinary, []byte(`2{"cols":100,"rows":35}`)); err != nil {
				t.Fatal(err)
			}
			closed := make(chan struct{})
			go func() {
				defer close(closed)
				for {
					if _, _, err := conn.Read(ctx); err != nil {
						return
					}
				}
			}()
			select {
			case views := <-instances:
				return conn, views, closed
			case <-time.After(5 * time.Second):
				t.Fatal("browser session did not start")
				return nil, nil, nil
			}
		}
		waitView := func(views chan string, text string) {
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
					t.Fatalf("last view %q, want %q", last, text)
				}
			}
		}
		conn, views, closed := connect()
		write := func(frame string) {
			t.Helper()
			if err := conn.Write(ctx, websocket.MessageBinary, []byte(frame)); err != nil {
				t.Fatal(err)
			}
		}
		waitView(views, "Events")
		write("0\t\r")
		waitView(views, "Misc thoughts")
		write("0\x1b[B\r")
		waitView(views, "Create thought")
		write("0\r")
		waitView(views, "Ctrl+S")
		const body = "browser draft\n界 café 👩‍💻"
		write("p" + body)
		waitView(views, "browser draft")
		write("0\x13")
		waitView(views, "Thought 1 •")
		var saved string
		if err := db.QueryRow("SELECT thought FROM thoughts").Scan(&saved); err != nil {
			t.Fatal(err)
		}
		if saved != body {
			t.Fatalf("saved thought got %q, want %q", saved, body)
		}
		write("0\x03")
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Fatal("session quit did not close connection")
		}
		conn, views, _ = connect()
		waitView(views, "Events")
		write("0\t\r")
		waitView(views, "Misc thoughts")
		write("0\x1b[B\r")
		waitView(views, "browser draft")
		// Reopen SQLite independently to prove the saved data is durable.
		reopened := openSQLite(t, dsn)
		if err := reopened.QueryRow("SELECT thought FROM thoughts").Scan(&saved); err != nil {
			t.Fatal(err)
		}
		if saved != body {
			t.Fatalf("reopened thought got %q, want %q", saved, body)
		}
	})
}
