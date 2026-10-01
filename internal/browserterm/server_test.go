package browserterm

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/coder/websocket"
	"github.com/jhern254/go-thoughts/internal/logging"
)

type probeModel struct{ messages chan tea.Msg }

func (m probeModel) Init() tea.Cmd  { return nil }
func (m probeModel) View() tea.View { return tea.NewView("browser probe") }
func (m probeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.messages <- msg
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "q" {
		return m, tea.Quit
	}
	return m, nil
}

func testServer(t *testing.T, factory func(context.Context) tea.Model) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, factory, logging.Nop()) }()
	stop := func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("server did not stop")
		}
	}
	return "http://" + ln.Addr().String(), stop
}

func connect(t *testing.T, address string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(address, "http")+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {address}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func writeFrame(t *testing.T, conn *websocket.Conn, frame string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageBinary, []byte(frame)); err != nil {
		t.Fatal(err)
	}
}

func readUntil(t *testing.T, conn *websocket.Conn, prefix string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, frame, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(string(frame), prefix) {
			if len(frame) > 0 && frame[0] == '7' {
				_ = conn.Close(websocket.StatusNormalClosure, "")
			}
			return string(frame)
		}
	}
}

func TestServer(t *testing.T) {
	t.Run("rejects terminal tracing without creating a traffic log", func(t *testing.T) {
		trace := filepath.Join(t.TempDir(), "private-trace.log")
		t.Setenv("TEA_TRACE", trace)
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		err = Serve(context.Background(), ln, func(context.Context) tea.Model { t.Fatal("constructed model with tracing enabled"); return nil }, logging.Nop())
		if err == nil {
			t.Fatal("accepted terminal tracing")
		}
		if _, err := os.Stat(trace); !os.IsNotExist(err) {
			t.Fatalf("trace file stat got %v, want not exist", err)
		}
	})
	t.Run("handshake diagnostics never echo untrusted headers", func(t *testing.T) {
		address, stop := testServer(t, func(context.Context) tea.Model { return probeModel{make(chan tea.Msg, 100)} })
		defer stop()
		req, _ := http.NewRequest("GET", address+"/ws", nil)
		req.Header.Set("Origin", address)
		req.Header.Set("Connection", "PRIVATE-HEADER-MARKER")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(body), "WebSocket connection rejected.\n"; got != want {
			t.Fatalf("body got %q, want %q", got, want)
		}
	})
	t.Run("serves embedded assets and rejects unexpected hosts and origins", func(t *testing.T) {
		address, stop := testServer(t, func(context.Context) tea.Model { t.Error("unexpected model construction"); return nil })
		defer stop()
		for _, path := range []string{"/", "/static/client.js", "/static/xterm.js"} {
			response, err := http.Get(address + path)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || response.StatusCode != 200 || len(body) == 0 {
				t.Fatalf("asset %s: status %d, bytes %d, error %v", path, response.StatusCode, len(body), err)
			}
			if response.Header.Get("Content-Security-Policy") == "" {
				t.Fatal("missing CSP")
			}
		}
		for _, tc := range []struct{ path, host, origin string }{
			{"/", "evil.example", ""}, {"/static/client.js", "evil.example", ""},
			{"/ws", "", ""}, {"/ws", "", "null"}, {"/ws", "", "https://evil.example"},
		} {
			req, _ := http.NewRequest("GET", address+tc.path, nil)
			if tc.host != "" {
				req.Host = tc.host
			}
			req.Header.Set("Origin", tc.origin)
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusForbidden {
				t.Fatalf("got status %d, want 403", response.StatusCode)
			}
		}
	})
	t.Run("delivers input paste and resize and quits only the session", func(t *testing.T) {
		messages := make(chan tea.Msg, 100)
		created := make(chan context.Context, 3)
		address, stop := testServer(t, func(ctx context.Context) tea.Model { created <- ctx; return probeModel{messages} })
		defer stop()
		conn := connect(t, address)
		writeFrame(t, conn, `2{"cols":90,"rows":30}`)
		readUntil(t, conn, "1")
		sessionCtx := <-created
		second := connect(t, address)
		if got := readUntil(t, second, "7"); got != "7busy" {
			t.Fatalf("got %q, want busy", got)
		}
		writeFrame(t, conn, "0x")
		writeFrame(t, conn, "pfirst\n界\r\t\ufffd")
		writeFrame(t, conn, `2{"cols":100,"rows":35}`)
		writeFrame(t, conn, "0q")
		if got := readUntil(t, conn, "7"); got != "7quit" {
			t.Fatalf("got %q, want quit", got)
		}
		select {
		case <-sessionCtx.Done():
		case <-time.After(time.Second):
			t.Fatal("session context not cancelled")
		}
		var paste bool
		var resized bool
		for len(messages) > 0 {
			switch msg := (<-messages).(type) {
			case tea.PasteMsg:
				paste = msg.Content == "first\n界\r\t\ufffd"
			case tea.WindowSizeMsg:
				if msg.Width == 100 && msg.Height == 35 {
					resized = true
				}
			}
		}
		if !paste || !resized {
			t.Fatalf("paste=%v resize=%v, want both", paste, resized)
		}
		reopened := connect(t, address)
		writeFrame(t, reopened, `2{"cols":80,"rows":24}`)
		readUntil(t, reopened, "1")
		select {
		case <-created:
		case <-time.After(time.Second):
			t.Fatal("new session was not created")
		}
	})
	t.Run("disconnect releases the slot and server cancellation stops the session", func(t *testing.T) {
		created := make(chan context.Context, 2)
		address, stop := testServer(t, func(ctx context.Context) tea.Model { created <- ctx; return probeModel{make(chan tea.Msg, 100)} })
		conn := connect(t, address)
		writeFrame(t, conn, `2{"cols":80,"rows":24}`)
		readUntil(t, conn, "1")
		ctx := <-created
		conn.CloseNow()
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("disconnect did not cancel model context")
		}
		// The HTTP handler releases the slot after its program and commands join.
		deadline := time.Now().Add(5 * time.Second)
		for {
			next := connect(t, address)
			writeFrame(t, next, `2{"cols":80,"rows":24}`)
			msg := readUntil(t, next, "")
			if strings.HasPrefix(msg, "1") {
				break
			}
			next.CloseNow()
			if time.Now().After(deadline) {
				t.Fatal("connection slot never released")
			}
		}
		ctx = <-created
		stop()
		if ctx.Err() == nil {
			t.Fatal("server stopped before session cancellation")
		}
	})
}
