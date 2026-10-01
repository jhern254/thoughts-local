package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/logging"
)

type failedBrowserOutput struct{}

func (failedBrowserOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRunBrowser(t *testing.T) {
	t.Run("occupied port fails without printing an address or constructing a model", func(t *testing.T) {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		var out bytes.Buffer
		err = runBrowser(context.Background(), ln.Addr().(*net.TCPAddr).Port, func(context.Context) tea.Model { t.Fatal("constructed model before bind succeeded"); return nil }, &out, logging.Nop())
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
		err = runBrowser(context.Background(), port, func(context.Context) tea.Model { return nil }, failedBrowserOutput{}, logging.Nop())
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
