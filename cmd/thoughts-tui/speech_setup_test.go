package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/editorprefs"
	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/speech"
	"github.com/jhern254/go-thoughts/internal/voice"
)

type setupTestTranscriber struct {
	closeCalls int
	closeErr   error
	streams    int
}

func (transcriber *setupTestTranscriber) StartRecordingStream(context.Context) (recordingSpeechStream, error) {
	transcriber.streams++
	return &testMoonshineStream{}, nil
}
func (transcriber *setupTestTranscriber) Close() error {
	transcriber.closeCalls++
	return transcriber.closeErr
}
func TestBrowserSpeechSetup(t *testing.T) {
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
		setup, err := newBrowserSpeechSetup(filepath.Join(t.TempDir(), "not-created"))
		if err != nil {
			t.Fatal(err)
		}
		setup.checkRuntime = func(context.Context) error { return speech.ErrRuntime }
		setup.installSpeechModel = func(context.Context, *os.Root, func(modelassets.DownloadProgress)) error {
			t.Fatal("installed without engine")
			return nil
		}
		status, err := setup.LoadInstalledSpeechModel(t.Context())
		if err != nil || status.CanDownload || status.Ready || status.Message == "" {
			t.Fatalf("got %+v, %v", status, err)
		}
		if _, err := os.Stat(setup.Directory()); !os.IsNotExist(err) {
			t.Fatalf("created model directory: %v", err)
		}
	})
	t.Run("missing model downloads once then reuses loaded engine", func(t *testing.T) {
		setup, err := newBrowserSpeechSetup(filepath.Join(t.TempDir(), "models"))
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
		setup.installSpeechModel = func(_ context.Context, _ *os.Root, report func(modelassets.DownloadProgress)) error {
			installs++
			installed = true
			report(modelassets.DownloadProgress{DownloadedBytes: 4, TotalBytes: 4})
			return nil
		}
		status, err := setup.LoadInstalledSpeechModel(t.Context())
		if err != nil || !status.CanDownload {
			t.Fatalf("got %+v, %v", status, err)
		}
		progress := 0
		if _, err := setup.InstallAndEnableSpeechModel(t.Context(), func(modelassets.DownloadProgress) { progress++ }); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			status, err = setup.LoadInstalledSpeechModel(t.Context())
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
		setup, err := newBrowserSpeechSetup(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer setup.Close()
		setup.checkRuntime = func(context.Context) error { return nil }
		backend := &setupTestTranscriber{}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		setup.loadTranscriber = func(context.Context, *os.Root) (browserSpeechTranscriber, error) { cancel(); return backend, nil }
		_, err = setup.LoadInstalledSpeechModel(ctx)
		if !errors.Is(err, context.Canceled) || backend.closeCalls != 1 || setup.transcriber != nil {
			t.Fatalf("err=%v closes=%d ready=%v", err, backend.closeCalls, setup.transcriber != nil)
		}
	})
	t.Run("closed owner rejects setup without invoking collaborators", func(t *testing.T) {
		setup, err := newBrowserSpeechSetup(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		setup.checkRuntime = func(context.Context) error { t.Fatal("checked runtime after close"); return nil }
		if err := setup.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := setup.InstallAndEnableSpeechModel(t.Context(), nil); !errors.Is(err, speech.ErrClosed) {
			t.Fatalf("got %v, want closed", err)
		}
	})
	t.Run("incomplete destination is never offered as a new download", func(t *testing.T) {
		setup, err := newBrowserSpeechSetup(t.TempDir())
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
		status, err := setup.LoadInstalledSpeechModel(t.Context())
		if err != nil || !status.CanRepair || status.CanDownload {
			t.Fatalf("got %+v, %v", status, err)
		}
	})
}

func TestBrowserSpeechSetup_Repair(t *testing.T) {
	t.Run("closes old model before repair and enables replacement without restart", func(t *testing.T) {
		setup, err := newBrowserSpeechSetup(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer setup.Close()
		setup.checkRuntime = func(context.Context) error { return nil }
		oldBackend := &setupTestTranscriber{}
		replacement := &setupTestTranscriber{}
		setup.transcriber = oldBackend
		repairs := 0
		setup.repairSpeechModel = func(ctx context.Context, root *os.Root, report func(modelassets.DownloadProgress)) error {
			repairs++
			if oldBackend.closeCalls != 1 {
				t.Fatal("repair preceded native cleanup")
			}
			if _, err := setup.RecordingConsumerFactory(nil)(ctx, voice.RecordingKey{}, nil); !errors.Is(err, speech.ErrRuntime) {
				t.Fatalf("old consumer still available: %v", err)
			}
			return nil
		}
		setup.loadTranscriber = func(context.Context, *os.Root) (browserSpeechTranscriber, error) { return replacement, nil }
		status, err := setup.RepairAndEnableSpeechModel(t.Context(), nil)
		if err != nil || !status.Ready || repairs != 1 {
			t.Fatalf("status=%+v repairs=%d err=%v", status, repairs, err)
		}
		if setup.transcriber != replacement {
			t.Fatal("replacement not loaded")
		}
	})
	t.Run("cancellation after deletion leaves model downloadable and unloaded", func(t *testing.T) {
		setup, err := newBrowserSpeechSetup(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer setup.Close()
		setup.checkRuntime = func(context.Context) error { return nil }
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		setup.repairSpeechModel = func(context.Context, *os.Root, func(modelassets.DownloadProgress)) error {
			cancel()
			return context.Canceled
		}
		setup.loadTranscriber = func(context.Context, *os.Root) (browserSpeechTranscriber, error) {
			t.Fatal("loaded after cancellation")
			return nil, nil
		}
		status, err := setup.RepairAndEnableSpeechModel(ctx, nil)
		if !errors.Is(err, context.Canceled) || !status.CanDownload || status.Ready || setup.transcriber != nil {
			t.Fatalf("status=%+v err=%v", status, err)
		}
	})
	t.Run("failed native cleanup prevents deletion and all future setup attempts", func(t *testing.T) {
		setup, err := newBrowserSpeechSetup(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		setup.checkRuntime = func(context.Context) error { return nil }
		setup.transcriber = &setupTestTranscriber{closeErr: speech.ErrRuntime}
		setup.repairSpeechModel = func(context.Context, *os.Root, func(modelassets.DownloadProgress)) error {
			t.Fatal("repair after failed cleanup")
			return nil
		}
		for range 2 {
			if _, err := setup.RepairAndEnableSpeechModel(t.Context(), nil); !errors.Is(err, speech.ErrRuntime) {
				t.Fatalf("got %v", err)
			}
		}
	})
}

func TestBrowserSpeechSetup_RepairStatus(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		loadErr    error
		wantRepair bool
	}{
		{"missing required file", modelassets.ErrNotInstalled, true},
		{"wrong size or hash", modelassets.ErrIntegrity, true},
		{"unreadable model", modelassets.ErrFilesystem, false},
		{"native runtime failure", speech.ErrRuntime, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			setup, err := newBrowserSpeechSetup(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer setup.Close()
			setup.checkRuntime = func(context.Context) error { return nil }
			setup.loadTranscriber = func(context.Context, *os.Root) (browserSpeechTranscriber, error) { return nil, scenario.loadErr }
			revisionDirectory := filepath.Join(setup.Directory(), string(setup.description.ID), setup.description.Revision)
			if err := os.MkdirAll(revisionDirectory+"/.complete", 0700); err != nil {
				t.Fatal(err)
			}
			status, err := setup.LoadInstalledSpeechModel(t.Context())
			if status.CanRepair != scenario.wantRepair || status.CanDownload || status.Ready {
				t.Fatalf("status=%+v, want repair=%v", status, scenario.wantRepair)
			}
			if scenario.wantRepair && err != nil {
				t.Fatal(err)
			}
			if !scenario.wantRepair && !errors.Is(err, scenario.loadErr) {
				t.Fatalf("error=%v, want %v", err, scenario.loadErr)
			}
			if _, err := os.Stat(revisionDirectory); err != nil {
				t.Fatal("checking removed installation")
			}
		})
	}
	t.Run("unavailable runtime rejects explicit repair without mutation", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "absent")
		setup, err := newBrowserSpeechSetup(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer setup.Close()
		setup.checkRuntime = func(context.Context) error { return speech.ErrRuntime }
		status, err := setup.RepairAndEnableSpeechModel(t.Context(), nil)
		if !errors.Is(err, speech.ErrRuntime) || status.CanDownload || status.CanRepair {
			t.Fatalf("got %+v, %v", status, err)
		}
		if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected directory: %v", err)
		}
	})
}

type recordingPreferencesStore struct {
	pause int
	err   error
}

func (s *recordingPreferencesStore) Load(context.Context, string) (data.EditorPreferences, error) {
	return data.EditorPreferences{SpeechPauseMS: s.pause}, s.err
}
func (s *recordingPreferencesStore) Save(_ context.Context, _ string, p data.EditorPreferences) error {
	s.pause = p.SpeechPauseMS
	return s.err
}
func TestBrowserSpeechSetup_RecordingPreferences(t *testing.T) {
	t.Run("snapshots preferences once per recording and preserves active setting", func(t *testing.T) {
		backend := &setupTestTranscriber{}
		setup := &browserSpeechSetup{transcriber: backend}
		store := &recordingPreferencesStore{pause: 2000}
		factory := setup.RecordingConsumerFactory(editorprefs.NewService(store, "user"))
		first, err := factory(t.Context(), voice.RecordingKey{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Close()
		store.pause = 0
		second, err := factory(t.Context(), voice.RecordingKey{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer second.Close()
		if first.(*browserSpeechConsumer).formatter.Pause != 2*time.Second || second.(*browserSpeechConsumer).formatter.Pause != 0 {
			t.Fatal("preference changed active recording or failed to apply to next")
		}
	})
	t.Run("failed preference read does not start native work", func(t *testing.T) {
		backend := &setupTestTranscriber{}
		setup := &browserSpeechSetup{transcriber: backend}
		cause := errors.New("private preference read error")
		factory := setup.RecordingConsumerFactory(editorprefs.NewService(&recordingPreferencesStore{err: cause}, "user"))
		if _, err := factory(t.Context(), voice.RecordingKey{}, nil); !errors.Is(err, cause) || backend.streams != 0 {
			t.Fatal("failed read started native stream or hid error")
		}
	})
}
