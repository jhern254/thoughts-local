//go:build integration

package integration_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/browserterm"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/voice"
)

// This opt-in host owns both Playwright and its disposable app. Normal Go
// checks remain network-free and require neither Node nor browser binaries.
func TestBrowserAudioWorkflow_Playwright(t *testing.T) {
	if os.Getenv("THOUGHTS_BROWSER_AUDIO_TEST") != "1" {
		t.Skip("set THOUGHTS_BROWSER_AUDIO_TEST=1 after installing browser test dependencies")
	}
	database, dsn := openMigratedSQLite(t)
	seed, err := os.ReadFile("../scripts/demo.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(t.Context(), string(seed)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	runtime, err := application.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	visualService, err := runtime.BrowserVisual(ctx)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var revision atomic.Uint64
	consumer := func(ctx context.Context, recording voice.RecordingKey, samples []int16, publish func(voice.TranscriptUpdate) bool) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Browser media pipelines can resample and filter the synthetic tone.
		// Detect its presence, not an exact sample; silent control fixtures emit
		// no text. This fake does not measure speech recognition or audio quality.
		for _, sample := range samples {
			if sample > 1000 || sample < -1000 {
				publish(voice.TranscriptUpdate{Recording: recording, Revision: revision.Add(1), Text: "synthetic speech"})
				break
			}
		}
		return nil
	}
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- browserterm.ServeWithAudioConsumer(ctx, listener, func(session context.Context) tea.Model {
			return tui.NewModel(session, runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), runtime.Events(), runtime.TimelineView(), logging.Nop())
		}, logging.Nop(), consumer, visualService)
	}()
	defer func() {
		cancel()
		if err := <-serverDone; err != nil {
			t.Error(err)
		}
	}()
	clientDirectory := filepath.Join("..", "internal", "browserterm", "client")
	artifacts := os.Getenv("THOUGHTS_BROWSER_ARTIFACTS")
	if artifacts == "" {
		artifacts = t.TempDir()
	}
	artifacts, err = filepath.Abs(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "node", "node_modules/@playwright/test/cli.js", "test", "--output", filepath.Join(artifacts, "test-results"))
	if filter := os.Getenv("THOUGHTS_BROWSER_TEST_FILTER"); filter != "" {
		command.Args = append(command.Args, "--grep", filter)
	}
	command.Dir = clientDirectory
	command.Env = append(os.Environ(), "THOUGHTS_BROWSER_URL=http://"+listener.Addr().String(), "THOUGHTS_BROWSER_ARTIFACTS="+artifacts)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		t.Fatal("browser acceptance failed", err)
	}
}
