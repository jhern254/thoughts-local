package browserterm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	return testServerListener(t, ln, factory, logging.Nop())
}

func testServerListener(t *testing.T, ln net.Listener, factory func(context.Context) tea.Model, logger logging.Logger) (string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, factory, logger, nil) }()
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
		err = Serve(context.Background(), ln, func(context.Context) tea.Model { t.Fatal("constructed model with tracing enabled"); return nil }, logging.Nop(), nil)
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
			if response.Header.Get("Permissions-Policy") != "microphone=(self), camera=()" {
				t.Fatal("missing microphone permission policy")
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
		req, _ := http.NewRequest("GET", address+"/ws", nil)
		req.Header["Origin"] = []string{address, address}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("duplicate Origin got status %d, want 403", response.StatusCode)
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

func TestServerInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		frame  string
		tooBig bool
	}{
		{"malformed resize", `2{"cols":0,"rows":24,"text":"PRIVATE-FRAME-MARKER"}`, false},
		{"malformed voice control", `v{"action":"recording","draftID":-1,"text":"PRIVATE-FRAME-MARKER"}`, false},
		{"oversized paste", "p" + strings.Repeat("x", maxPasteBytes) + "PRIVATE-FRAME-MARKER", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger, err := logging.New(&logs, "browser-test", "debug")
			if err != nil {
				t.Fatal(err)
			}
			ln, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			created := make(chan *workModel, 2)
			address, stop := testServerListener(t, ln, func(ctx context.Context) tea.Model {
				m := newWorkModel(ctx)
				close(m.release)
				created <- m
				return m
			}, logger)
			defer func() {
				stop()
				// Neither rejected input nor library errors are operational diagnostics.
				if got := logs.String(); got != "" {
					t.Errorf("got logs %q, want no operational events for rejected input", got)
				}
			}()
			conn := connect(t, address)
			writeFrame(t, conn, `2{"cols":80,"rows":24}`)
			readUntil(t, conn, "1")
			m := <-created
			waitClosed(t, m.started, time.Second, "session command startup")
			writeFrame(t, conn, tc.frame)
			if tc.tooBig {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				var err error
				for err == nil {
					_, frame, readErr := conn.Read(ctx)
					err = readErr
					if bytes.Contains(frame, []byte("PRIVATE-FRAME-MARKER")) {
						t.Fatal("rejected input leaked into terminal output")
					}
				}
				cancel()
				var closed websocket.CloseError
				if !errors.As(err, &closed) || closed.Code != websocket.StatusMessageTooBig || closed.Reason != fmt.Sprintf("read limited at %d bytes", maxPasteBytes+2) {
					t.Fatalf("got close %v, want fixed message-too-big limit diagnostic", err)
				}
			} else {
				// Cancellation must not depend on an invalid peer acknowledging close.
				waitClosed(t, m.cancelled, time.Second, "invalid-input cancellation before close acknowledgement")
				if got := readUntil(t, conn, "7"); got != "7input" {
					t.Fatalf("got %q, want fixed 7input", got)
				}
			}
			waitClosed(t, m.cleaned, time.Second, "invalid-input command cleanup")
			freshSession(t, address)
			next := <-created
			if next.ctx.Err() != nil {
				t.Fatal("fresh valid session was cancelled")
			}
		})
	}
}

// The server already accepts a net.Listener. A close-aware write gate models a
// full socket send buffer without depending on kernel buffer sizes or megabytes
// of output. HTTP upgrade and WebSocket framing still use the real library/TCP.
type stalledConn struct {
	net.Conn
	stall     atomic.Bool
	blocked   chan struct{}
	closed    chan struct{}
	blockOnce sync.Once
	closeOnce sync.Once
}

func (c *stalledConn) Write(data []byte) (int, error) {
	if c.stall.Load() {
		c.blockOnce.Do(func() { close(c.blocked) })
		<-c.closed
		return 0, net.ErrClosed
	}
	return c.Conn.Write(data)
}
func (c *stalledConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

type stalledListener struct {
	net.Listener
	accepted chan *stalledConn
}

func (l *stalledListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	c := &stalledConn{Conn: conn, blocked: make(chan struct{}), closed: make(chan struct{})}
	select {
	case l.accepted <- c:
	default:
	}
	return c, nil
}

type workModel struct {
	ctx                                  context.Context
	started, cancelled, release, cleaned chan struct{}
	output                               string
}

func newWorkModel(ctx context.Context) *workModel {
	return &workModel{
		ctx:       ctx,
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
		cleaned:   make(chan struct{}),
		output:    "browser probe",
	}
}
func (m *workModel) Init() tea.Cmd {
	return func() tea.Msg {
		close(m.started)
		<-m.ctx.Done()
		close(m.cancelled)
		<-m.release
		close(m.cleaned)
		return nil
	}
}
func (m *workModel) View() tea.View { return tea.NewView(m.output) }
func (m *workModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "x" {
		m.output = "changed output"
	}
	return m, nil
}
func waitClosed(t *testing.T, ch <-chan struct{}, bound time.Duration, operation string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(bound):
		t.Fatalf("%s did not finish within %s", operation, bound)
	}
}
func freshSession(t *testing.T, address string) *websocket.Conn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn := connect(t, address)
		writeFrame(t, conn, `2{"cols":80,"rows":24}`)
		frame := readUntil(t, conn, "")
		if strings.HasPrefix(frame, "1") {
			return conn
		}
		conn.CloseNow()
		if frame != "7busy" || time.Now().After(deadline) {
			t.Fatalf("got %q, want a healthy new session", frame)
		}
	}
}

func TestServerStalledOutput(t *testing.T) {
	t.Run("write timeout cancels work releases admission and shutdown joins cleanup", func(t *testing.T) {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listener := &stalledListener{Listener: ln, accepted: make(chan *stalledConn, 10)}
		created := make(chan *workModel, 3)
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "browser-test", "debug")
		if err != nil {
			t.Fatal(err)
		}
		address, stop := testServerListener(t, listener, func(ctx context.Context) tea.Model {
			m := newWorkModel(ctx)
			created <- m
			return m
		}, logger)
		var stopOnce sync.Once
		stopServer := func() { stopOnce.Do(stop) }
		defer stopServer()
		conn := connect(t, address)
		writeFrame(t, conn, `2{"cols":80,"rows":24}`)
		readUntil(t, conn, "1")
		transport := <-listener.accepted
		m := <-created
		// Always release held cleanup on assertion failures before stopping Serve.
		defer func() {
			select {
			case <-m.release:
			default:
				close(m.release)
			}
		}()
		waitClosed(t, m.started, time.Second, "tracked command startup")
		transport.stall.Store(true)
		writeFrame(t, conn, "0x")
		waitClosed(t, transport.blocked, time.Second, "real socket output blocking")
		// The client stops reading. Only terminalWriter's configured deadline can
		// close this blocked write; the test neither releases it nor shuts down.
		waitClosed(t, m.cancelled, writeTimeout+time.Second, "stalled-output cancellation")
		waitClosed(t, transport.closed, time.Second, "timed-out socket closure")
		busy := connect(t, address)
		if got := readUntil(t, busy, "7"); got != "7busy" {
			t.Fatalf("got %q, want busy during tracked cleanup", got)
		}
		close(m.release)
		waitClosed(t, m.cleaned, time.Second, "timed-out session cleanup")
		freshSession(t, address)
		next := <-created
		defer func() {
			select {
			case <-next.release:
			default:
				close(next.release)
			}
		}()
		waitClosed(t, next.started, time.Second, "replacement command startup")
		resourcesClosed := make(chan struct{})
		go func() { stopServer(); close(resourcesClosed) }()
		waitClosed(t, next.cancelled, time.Second, "shutdown cancellation")
		select {
		case <-resourcesClosed:
			t.Fatal("shared runtime boundary closed before tracked work finished")
		default:
		}
		close(next.release)
		waitClosed(t, next.cleaned, time.Second, "shutdown command cleanup")
		waitClosed(t, resourcesClosed, time.Second, "server shutdown after cleanup")
		if got := logs.String(); got != "" {
			t.Fatalf("got logs %q, want no raw output/write errors", got)
		}
	})
}
