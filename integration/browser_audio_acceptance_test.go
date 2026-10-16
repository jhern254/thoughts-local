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
	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/voice"
)

// This opt-in host owns both Playwright and its disposable app. Normal Go
// checks remain network-free and require neither Node nor browser binaries.
func TestBrowserAudioWorkflow_Playwright(t *testing.T) {
	if os.Getenv("THOUGHTS_BROWSER_AUDIO_TEST") != "1" {
		t.Skip("set THOUGHTS_BROWSER_AUDIO_TEST=1 after installing browser test dependencies")
	}
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("browser video acceptance requires %s on PATH", tool)
		}
	}
	// Keep synthetic imports out of the user's shared assets while reusing the
	// browser binaries installed before this test changes its application home.
	if os.Getenv("PLAYWRIGHT_BROWSERS_PATH") == "" {
		cacheDirectory, err := os.UserCacheDir()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("PLAYWRIGHT_BROWSERS_PATH", filepath.Join(cacheDirectory, "ms-playwright"))
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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
			model := tui.NewModel(session, runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), runtime.Events(), runtime.TimelineView(), logging.Nop())
			if os.Getenv("THOUGHTS_MODELS_ONBOARDING_TEST") == "1" {
				model.ConfigureSpeechModelSetup(&browserSpeechSetupFixture{})
			}
			return model
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

// The first download is held until Cancel; the retry completes. Browser actions,
// not elapsed-time sleeps or additional HTTP control routes, drive the fixture.
type browserSpeechSetupFixture struct {
	attempts int
	ready    bool
}

func (*browserSpeechSetupFixture) Description() modelassets.ModelDescription {
	description, _ := modelassets.DescribeModel(modelassets.MoonshineSmallStreamingEnglish)
	return description
}
func (*browserSpeechSetupFixture) Directory() string { return "~/Documents/Thoughts/models" }
func (fixture *browserSpeechSetupFixture) LoadInstalledSpeechModel(context.Context) (tui.SpeechModelSetupStatus, error) {
	return tui.SpeechModelSetupStatus{Ready: fixture.ready, CanDownload: !fixture.ready}, nil
}
func (fixture *browserSpeechSetupFixture) InstallAndEnableSpeechModel(ctx context.Context, report func(modelassets.DownloadProgress)) error {
	fixture.attempts++
	description := fixture.Description()
	report(modelassets.DownloadProgress{DownloadedBytes: description.ExpectedSizeBytes / 2, TotalBytes: description.ExpectedSizeBytes})
	if fixture.attempts == 1 {
		<-ctx.Done()
		return ctx.Err()
	}
	report(modelassets.DownloadProgress{DownloadedBytes: description.ExpectedSizeBytes, TotalBytes: description.ExpectedSizeBytes})
	fixture.ready = true
	return nil
}
