package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/logging"
)

type failedBrowserOutput struct{}

func (failedBrowserOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type launchOutput struct {
	bytes.Buffer
	failed chan struct{}
}

func (w *launchOutput) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	if bytes.Contains(data, []byte("Could not open the default browser.")) {
		close(w.failed)
	}
	return n, err
}

func TestRunBrowser(t *testing.T) {
	t.Run("shutdown cancels and joins the browser launcher", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started := make(chan struct{})
		cancelled := make(chan struct{})
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- serveBrowser(ctx, 0, "", func(context.Context) tea.Model { return nil }, io.Discard, logging.Nop(), func(ctx context.Context, _ string) error {
				close(started)
				<-ctx.Done()
				close(cancelled)
				<-release
				return ctx.Err()
			})
		}()
		<-started
		cancel()
		<-cancelled
		select {
		case err := <-done:
			close(release)
			t.Fatalf("server returned %v before launcher cleanup", err)
		default:
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
	t.Run("opens the served address and joins launcher cleanup", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		called := false
		err := serveBrowser(ctx, 0, "", func(context.Context) tea.Model { return nil }, io.Discard, logging.Nop(), func(ctx context.Context, address string) error {
			defer cancel()
			called = true
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
			if err != nil {
				return err
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Errorf("opening served page: %v", err)
				return err
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Errorf("got HTTP %d, want 200", response.StatusCode)
			}
			return nil
		})
		if err != nil || !called {
			t.Fatalf("got err %v, opened %v; want serving and launcher cleanup", err, called)
		}
	})
	t.Run("launcher failure keeps serving and prints only safe fallback text", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		out := launchOutput{failed: make(chan struct{})}
		address := make(chan string, 1)
		go func() {
			defer cancel()
			select {
			case <-out.failed:
			case <-ctx.Done():
				return
			}
			response, err := http.Get(<-address)
			if err != nil {
				t.Errorf("server stopped after launcher failure: %v", err)
				return
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Errorf("got HTTP %d after launcher failure, want 200", response.StatusCode)
			}
		}()
		err := serveBrowser(ctx, 0, "", func(context.Context) tea.Model { return nil }, &out, logging.Nop(), func(_ context.Context, url string) error {
			address <- url
			return errors.New("PRIVATE-LAUNCH-ERROR")
		})
		if err != nil || strings.Contains(out.String(), "PRIVATE-LAUNCH-ERROR") || !strings.Contains(out.String(), "Open the printed address manually.") {
			t.Fatalf("got err %v, output %q; want safe nonfatal launcher failure", err, out.String())
		}
	})
	t.Run("startup rejection does not open a browser", func(t *testing.T) {
		t.Setenv("TEA_TRACE", "trace")
		err := serveBrowser(context.Background(), 0, "", func(context.Context) tea.Model { return nil }, io.Discard, logging.Nop(), func(context.Context, string) error {
			t.Error("opened browser before startup validation")
			return nil
		})
		if err == nil {
			t.Fatal("accepted terminal tracing")
		}
	})
	t.Run("occupied port fails without printing an address or constructing a model", func(t *testing.T) {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		var out bytes.Buffer
		err = runBrowser(context.Background(), ln.Addr().(*net.TCPAddr).Port, false, "", func(context.Context) tea.Model { t.Fatal("constructed model before bind succeeded"); return nil }, &out, logging.Nop())
		if err == nil || out.Len() != 0 {
			t.Fatalf("got err %v, output %q; want bind failure and no address", err, out.String())
		}
	})
	t.Run("output failure releases the bound listener", func(t *testing.T) {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := ln.Addr().String()
		port := ln.Addr().(*net.TCPAddr).Port
		ln.Close()
		err = runBrowser(context.Background(), port, false, "", func(context.Context) tea.Model { return nil }, failedBrowserOutput{}, logging.Nop())
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("got %v, want output failure", err)
		}
		ln, err = net.Listen("tcp4", address)
		if err != nil {
			t.Fatalf("listener was retained: %v", err)
		}
		ln.Close()
	})
}
