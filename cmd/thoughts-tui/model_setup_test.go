package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/speech"
)

type setupTestTranscriber struct {
	closeCalls int
	streams    int
}

func (transcriber *setupTestTranscriber) StartRecordingStream(context.Context) (recordingSpeechStream, error) {
	transcriber.streams++
	return &testMoonshineStream{}, nil
}
func (transcriber *setupTestTranscriber) Close() error { transcriber.closeCalls++; return nil }
func TestBrowserModelSetup(t *testing.T) {
	t.Run("default uses Documents and explicit directory wins", func(t *testing.T) {
		home := t.TempDir()
		if got, want := resolveModelDirectory("", home), filepath.Join(home, "Documents", "Thoughts", "models"); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		if got := resolveModelDirectory("custom models", home); got != "custom models" {
			t.Fatalf("override=%q", got)
		}
	})
	t.Run("unavailable engine makes no model directory or install request", func(t *testing.T) {
		setup, err := newBrowserModelSetup(filepath.Join(t.TempDir(), "not-created"))
		if err != nil {
			t.Fatal(err)
		}
		setup.checkRuntime = func(context.Context) error { return speech.ErrRuntime }
		setup.installModel = func(context.Context, *os.Root, func(modelassets.DownloadProgress)) error {
			t.Fatal("installed without engine")
			return nil
		}
		status, err := setup.LoadInstalledModel(t.Context())
		if err != nil || status.CanDownload || status.Ready || status.Message == "" {
			t.Fatalf("got %+v, %v", status, err)
		}
		if _, err := os.Stat(setup.Directory()); !os.IsNotExist(err) {
			t.Fatalf("created model directory: %v", err)
		}
	})
	t.Run("missing model downloads once then reuses loaded engine", func(t *testing.T) {
		setup, err := newBrowserModelSetup(filepath.Join(t.TempDir(), "models"))
		if err != nil {
			t.Fatal(err)
		}
		defer setup.Close()
		setup.checkRuntime = func(context.Context) error { return nil }
		installed := false
		loads := 0
		backend := &setupTestTranscriber{}
		setup.loadTranscriber = func(context.Context, *os.Root) (browserSpeechTranscriber, error) {
			loads++
			if !installed {
				return nil, modelassets.ErrNotInstalled
			}
			return backend, nil
		}
		installs := 0
		setup.installModel = func(_ context.Context, _ *os.Root, report func(modelassets.DownloadProgress)) error {
			installs++
			installed = true
			report(modelassets.DownloadProgress{DownloadedBytes: 4, TotalBytes: 4})
			return nil
		}
		status, err := setup.LoadInstalledModel(t.Context())
		if err != nil || !status.CanDownload {
			t.Fatalf("got %+v, %v", status, err)
		}
		progress := 0
		if err := setup.InstallAndEnableModel(t.Context(), func(modelassets.DownloadProgress) { progress++ }); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			status, err = setup.LoadInstalledModel(t.Context())
			if err != nil || !status.Ready {
				t.Fatalf("got %+v, %v", status, err)
			}
		}
		if installs != 1 || loads != 1 || progress != 1 {
			t.Fatalf("installs=%d loads=%d progress=%d", installs, loads, progress)
		}
		if err := setup.Close(); err != nil {
			t.Fatal(err)
		}
		if err := setup.Close(); err != nil {
			t.Fatal(err)
		}
		if backend.closeCalls != 1 {
			t.Fatalf("close calls=%d, want 1", backend.closeCalls)
		}
	})
	t.Run("cancellation during native load closes backend without readiness", func(t *testing.T) {
		setup, err := newBrowserModelSetup(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer setup.Close()
		setup.checkRuntime = func(context.Context) error { return nil }
		backend := &setupTestTranscriber{}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		setup.loadTranscriber = func(context.Context, *os.Root) (browserSpeechTranscriber, error) { cancel(); return backend, nil }
		_, err = setup.LoadInstalledModel(ctx)
		if !errors.Is(err, context.Canceled) || backend.closeCalls != 1 || setup.transcriber != nil {
			t.Fatalf("err=%v closes=%d ready=%v", err, backend.closeCalls, setup.transcriber != nil)
		}
	})
	t.Run("closed owner rejects setup without invoking collaborators", func(t *testing.T) {
		setup, err := newBrowserModelSetup(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		setup.checkRuntime = func(context.Context) error { t.Fatal("checked runtime after close"); return nil }
		if err := setup.Close(); err != nil {
			t.Fatal(err)
		}
		if err := setup.InstallAndEnableModel(t.Context(), nil); !errors.Is(err, speech.ErrClosed) {
			t.Fatalf("got %v, want closed", err)
		}
	})
	t.Run("incomplete destination is never offered as a new download", func(t *testing.T) {
		setup, err := newBrowserModelSetup(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer setup.Close()
		setup.checkRuntime = func(context.Context) error { return nil }
		setup.loadTranscriber = func(context.Context, *os.Root) (browserSpeechTranscriber, error) {
			return nil, modelassets.ErrNotInstalled
		}
		if err := os.MkdirAll(filepath.Join(setup.Directory(), string(setup.description.ID), setup.description.Revision), 0700); err != nil {
			t.Fatal(err)
		}
		status, err := setup.LoadInstalledModel(t.Context())
		if !errors.Is(err, modelassets.ErrNotInstalled) || status.CanDownload {
			t.Fatalf("got %+v, %v", status, err)
		}
	})
}
