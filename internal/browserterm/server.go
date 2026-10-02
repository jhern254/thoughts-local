// Package browserterm serves the native Thoughts TUI to one local browser.
package browserterm

import (
	"bufio"
	"context"
	"embed"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/coder/websocket"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
)

//go:embed static/*
var assets embed.FS

const (
	initialTimeout = 10 * time.Second
	writeTimeout   = 5 * time.Second
)

type server struct {
	newModel  func(context.Context) tea.Model
	logger    logging.Logger
	authority string
	ctx       context.Context
	mu        sync.Mutex
	closing   bool
	active    bool
	sessions  sync.WaitGroup
}

// Serve owns a previously bound IPv4 loopback listener. It returns after HTTP
// handlers, the active program, and its started commands have stopped. Factories
// must give all service operations the supplied session context.
func Serve(ctx context.Context, listener net.Listener, newModel func(context.Context) tea.Model, logger logging.Logger) error {
	defer listener.Close()
	// Bubble Tea reads this directly from the process environment, independent
	// of WithEnvironment, and records terminal traffic. Refuse it in browser mode.
	if os.Getenv("TEA_TRACE") != "" {
		return errors.New("disable TEA_TRACE before starting browser mode")
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		return errors.New("browser listener must bind to 127.0.0.1")
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s := &server{newModel: newModel, logger: logger, authority: listener.Addr().String(), ctx: sessionCtx}
	httpServer := &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       initialTimeout,
		WriteTimeout:      initialTimeout,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          log.New(httpDiagnostics{logger}, "", 0),
		BaseContext:       func(net.Listener) context.Context { return sessionCtx },
	}
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	var serveErr error
	select {
	case serveErr = <-done:
	case <-ctx.Done():
	}
	// Stop admission before cancelling and joining hijacked WebSocket handlers:
	// net/http.Shutdown alone does not wait for them.
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), initialTimeout)
	defer stop()
	shutdownErr := httpServer.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		_ = httpServer.Close()
	}
	s.sessions.Wait()
	if serveErr == nil {
		serveErr = <-done
	}
	if errors.Is(serveErr, http.ErrServerClosed) || errors.Is(serveErr, net.ErrClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, shutdownErr)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	if r.Host != s.authority || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+s.authority) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/ws" {
		if len(r.Header.Values("Origin")) != 1 || r.Header.Get("Origin") != "http://"+s.authority {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		s.connect(w, r)
		return
	}
	name := r.URL.Path
	if name == "/" {
		name = "/static/index.html"
	}
	// Exact embedded names only. There is no disk-backed file server or directory listing.
	if path.Dir(name) != "/static" {
		http.NotFound(w, r)
		return
	}
	data, err := assets.ReadFile(name[1:])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
	_, _ = w.Write(data)
}

func (s *server) connect(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		http.Error(w, "Server stopping", http.StatusServiceUnavailable)
		return
	}
	busy := s.active
	if !busy {
		s.active = true
	}
	s.sessions.Add(1)
	s.mu.Unlock()
	slotHeld := !busy
	releaseSlot := func() {
		if slotHeld {
			s.mu.Lock()
			s.active = false
			s.mu.Unlock()
			slotHeld = false
		}
	}
	defer s.sessions.Done()
	defer releaseSlot()
	// The library can include request headers in handshake error bodies. Retain
	// status codes while replacing those bodies with fixed diagnostic text.
	conn, err := websocket.Accept(&handshakeWriter{ResponseWriter: w}, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	if busy {
		sendStatus(s.ctx, conn, "busy")
		return
	}
	conn.SetReadLimit(maxPasteBytes + 1)
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	startCtx, stop := context.WithTimeout(ctx, initialTimeout)
	kind, frame, err := conn.Read(startCtx)
	stop()
	if err != nil {
		return
	}
	messages, err := inputMessages(frame)
	if err != nil || kind != websocket.MessageBinary || len(frame) == 0 || frame[0] != '2' {
		sendStatus(ctx, conn, "input")
		return
	}
	size := messages[0].(tea.WindowSizeMsg)
	guard := newSessionModel(ctx, cancel, s.newModel)
	program := newProgram(ctx, guard, &terminalWriter{ctx: ctx, conn: conn, cancel: cancel}, size)
	readCtx, stopReading := context.WithCancel(s.ctx)
	defer stopReading()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		defer cancel()
		for {
			kind, frame, err := conn.Read(readCtx)
			if err != nil {
				return
			}
			messages, err := inputMessages(frame)
			if err != nil || kind != websocket.MessageBinary {
				rejectInput(s.ctx, conn, cancel)
				return
			}
			for _, msg := range messages {
				program.Send(msg)
			}
		}
	}()
	err = runProgram(program)
	wasCancelled := ctx.Err() != nil
	cancel()
	guard.stop()
	// Editing has stopped. Release admission before announcing quit so a client
	// that immediately reconnects cannot race the old socket's close handshake.
	releaseSlot()
	if guard.failed.Load() {
		err = errProgram
	}
	if err != nil && (!wasCancelled || guard.failed.Load()) {
		if category, emit := failure.Classify(logging.TUIRun, err); emit {
			s.logger.Failure(logging.TUIRun, category)
		}
		sendStatus(s.ctx, conn, "failed")
	} else if !wasCancelled {
		sendStatus(s.ctx, conn, "quit")
	}
	stopReading()
	<-readDone
}

func sendStatus(ctx context.Context, conn *websocket.Conn, status string) {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if conn.Write(ctx, websocket.MessageBinary, []byte("7"+status)) == nil {
		// Complete the close handshake so unread browser input cannot make a TCP
		// reset discard the final status. The library bounds each phase at 5s.
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
}

func rejectInput(ctx context.Context, conn *websocket.Conn, cancelSession context.CancelFunc) {
	ctx, stop := context.WithTimeout(ctx, writeTimeout)
	err := conn.Write(ctx, websocket.MessageBinary, []byte("7input"))
	stop()
	// Queue the fixed rejection before cancellation can interrupt terminal I/O,
	// then cancel application work before waiting for the peer's acknowledgement.
	cancelSession()
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
}

type terminalWriter struct {
	ctx    context.Context
	conn   *websocket.Conn
	cancel context.CancelFunc
}

func (w *terminalWriter) Write(data []byte) (int, error) {
	total := len(data)
	for len(data) > 0 {
		if w.ctx.Err() != nil {
			return total - len(data), io.ErrClosedPipe
		}
		n := min(len(data), 64<<10)
		frame := make([]byte, n+1)
		frame[0] = '1'
		copy(frame[1:], data[:n])
		ctx, cancel := context.WithTimeout(w.ctx, writeTimeout)
		err := w.conn.Write(ctx, websocket.MessageBinary, frame)
		cancel()
		if err != nil {
			w.cancel()
			return total - len(data), io.ErrClosedPipe
		}
		data = data[n:]
	}
	return total, nil
}

type handshakeWriter struct {
	http.ResponseWriter
	failed  bool
	written bool
}

func (w *handshakeWriter) WriteHeader(status int) {
	w.failed = status >= 400
	w.ResponseWriter.WriteHeader(status)
}
func (w *handshakeWriter) Write(data []byte) (int, error) {
	if !w.failed {
		return w.ResponseWriter.Write(data)
	}
	if !w.written {
		w.written = true
		_, err := io.WriteString(w.ResponseWriter, "WebSocket connection rejected.\n")
		if err != nil {
			return 0, err
		}
	}
	return len(data), nil
}
func (w *handshakeWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

type httpDiagnostics struct{ logger logging.Logger }

func (w httpDiagnostics) Write(data []byte) (int, error) {
	w.logger.Failure(logging.TUIRun, logging.UnexpectedFailure)
	return len(data), nil
}
