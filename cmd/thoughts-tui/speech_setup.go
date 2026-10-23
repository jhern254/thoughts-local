package main

import (
	"context"
	"errors"
	"github.com/jhern254/go-thoughts/internal/editorprefs"
	"os"
	"path/filepath"
	"sync"
	"time"

	appcore "github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/browserterm"
	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/speech"
	"github.com/jhern254/go-thoughts/internal/speech/moonshine"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/voice"
)

// Setup calls are serialized by the single browser session. Only consumer
// creation reads readiness concurrently. Serve joins setup/recording commands
// before Close releases the model and its root.
type browserSpeechSetup struct {
	modelDirectory     string
	description        modelassets.ModelDescription
	checkRuntime       func(context.Context) error
	loadTranscriber    func(context.Context, *os.Root) (browserSpeechTranscriber, error)
	installSpeechModel func(context.Context, *os.Root, func(modelassets.DownloadProgress)) error
	repairSpeechModel  func(context.Context, *os.Root, func(modelassets.DownloadProgress)) error

	mu              sync.Mutex
	modelRoot       *os.Root
	transcriber     browserSpeechTranscriber
	closed          bool
	closeErr        error
	modelReleaseErr error
}
type browserSpeechTranscriber interface {
	StartRecordingStream(context.Context) (recordingSpeechStream, error)
	Close() error
}
type loadedMoonshineTranscriber struct{ *moonshine.Transcriber }

func (transcriber loadedMoonshineTranscriber) StartRecordingStream(ctx context.Context) (recordingSpeechStream, error) {
	return transcriber.StartStream(ctx)
}

func resolveModelDirectory(configuredDirectory, homeDirectory string) string {
	if configuredDirectory != "" {
		return configuredDirectory
	}
	return filepath.Join(appcore.DataDirectory(homeDirectory), "models")
}
func newBrowserSpeechSetup(configuredDirectory string) (*browserSpeechSetup, error) {
	homeDirectory, err := os.UserHomeDir()
	if configuredDirectory == "" && err != nil {
		return nil, &browserSpeechError{category: errBrowserSpeechSetup, cause: err}
	}
	description, err := modelassets.DescribeModel(modelassets.MoonshineSmallStreamingEnglish)
	if err != nil {
		return nil, err
	}
	return &browserSpeechSetup{
		modelDirectory: resolveModelDirectory(configuredDirectory, homeDirectory),
		description:    description,
		checkRuntime:   moonshine.CheckRuntime,
		loadTranscriber: func(ctx context.Context, root *os.Root) (browserSpeechTranscriber, error) {
			transcriber, err := moonshine.Open(ctx, root)
			if err != nil {
				return nil, err
			}
			return loadedMoonshineTranscriber{transcriber}, nil
		},
		installSpeechModel: func(ctx context.Context, root *os.Root, report func(modelassets.DownloadProgress)) error {
			installer, err := modelassets.NewInstaller(root)
			if err != nil {
				return err
			}
			_, err = installer.InstallWithProgress(ctx, modelassets.MoonshineSmallStreamingEnglish, report)
			return err
		},
		repairSpeechModel: func(ctx context.Context, root *os.Root, report func(modelassets.DownloadProgress)) error {
			installer, err := modelassets.NewInstaller(root)
			if err != nil {
				return err
			}
			_, err = installer.RepairWithProgress(ctx, modelassets.MoonshineSmallStreamingEnglish, report)
			return err
		},
	}, nil
}
func (setup *browserSpeechSetup) Description() modelassets.ModelDescription { return setup.description }
func (setup *browserSpeechSetup) Directory() string                         { return setup.modelDirectory }
func (setup *browserSpeechSetup) LoadInstalledSpeechModel(ctx context.Context) (tui.SpeechModelSetupStatus, error) {
	if err := ctx.Err(); err != nil {
		return tui.SpeechModelSetupStatus{}, err
	}
	setup.mu.Lock()
	ready := setup.transcriber != nil
	closed := setup.closed
	releaseErr := setup.modelReleaseErr
	setup.mu.Unlock()
	if closed {
		return tui.SpeechModelSetupStatus{}, speech.ErrClosed
	}
	if releaseErr != nil {
		return tui.SpeechModelSetupStatus{}, releaseErr
	}
	if ready {
		return tui.SpeechModelSetupStatus{Ready: true}, nil
	}
	if err := setup.checkRuntime(ctx); err != nil {
		if ctx.Err() != nil {
			return tui.SpeechModelSetupStatus{}, ctx.Err()
		}
		return tui.SpeechModelSetupStatus{Message: "Local speech isn't available in this build. You can still type and save."}, nil
	}
	if setup.modelRoot == nil {
		modelRoot, err := os.OpenRoot(setup.modelDirectory)
		if errors.Is(err, os.ErrNotExist) {
			return tui.SpeechModelSetupStatus{CanDownload: true}, nil
		}
		if err != nil {
			return tui.SpeechModelSetupStatus{}, &browserSpeechError{category: modelassets.ErrFilesystem, cause: err}
		}
		setup.modelRoot = modelRoot
	}
	if err := setup.loadInstalledTranscriber(ctx); err != nil {
		status := setup.statusAfterFailedSetup(err)
		if errors.Is(err, modelassets.ErrFilesystem) {
			return status, err
		}
		if errors.Is(err, modelassets.ErrNotInstalled) || errors.Is(err, modelassets.ErrIntegrity) {
			if status.CanDownload || status.CanRepair {
				return status, nil
			}
		}
		return status, err
	}
	return tui.SpeechModelSetupStatus{Ready: true}, nil
}
func (setup *browserSpeechSetup) loadInstalledTranscriber(ctx context.Context) error {
	transcriber, err := setup.loadTranscriber(ctx, setup.modelRoot)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		closeErr := transcriber.Close()
		if closeErr != nil {
			setup.mu.Lock()
			setup.transcriber = transcriber
			setup.modelReleaseErr = closeErr
			setup.mu.Unlock()
		}
		return &browserSpeechError{category: err, cause: errors.Join(err, closeErr)}
	}
	setup.mu.Lock()
	defer setup.mu.Unlock()
	setup.transcriber = transcriber
	return nil
}
func (setup *browserSpeechSetup) InstallAndEnableSpeechModel(ctx context.Context, report func(modelassets.DownloadProgress)) (tui.SpeechModelSetupStatus, error) {
	return setup.installAndEnableSpeechModel(ctx, report, false)
}

func (setup *browserSpeechSetup) RepairAndEnableSpeechModel(ctx context.Context, report func(modelassets.DownloadProgress)) (tui.SpeechModelSetupStatus, error) {
	return setup.installAndEnableSpeechModel(ctx, report, true)
}

func (setup *browserSpeechSetup) installAndEnableSpeechModel(ctx context.Context, report func(modelassets.DownloadProgress), repair bool) (status tui.SpeechModelSetupStatus, operationErr error) {
	defer func() {
		if operationErr != nil {
			status = setup.statusAfterFailedSetup(operationErr)
		}
	}()
	if err := ctx.Err(); err != nil {
		return status, err
	}
	setup.mu.Lock()
	closed, ready, releaseErr := setup.closed, setup.transcriber != nil, setup.modelReleaseErr
	setup.mu.Unlock()
	if closed {
		return status, speech.ErrClosed
	}
	if releaseErr != nil {
		return status, releaseErr
	}
	if ready && !repair {
		return tui.SpeechModelSetupStatus{Ready: true}, nil
	}
	if err := setup.checkRuntime(ctx); err != nil {
		return status, err
	}
	if repair {
		// Stop handing out the old model before native cleanup. Close waits for
		// native calls and releases streams/mappings before repair can delete files.
		setup.mu.Lock()
		if setup.transcriber != nil {
			setup.modelReleaseErr = setup.transcriber.Close()
			if setup.modelReleaseErr == nil {
				setup.transcriber = nil
			}
		}
		releaseErr = setup.modelReleaseErr
		setup.mu.Unlock()
		if releaseErr != nil {
			return status, releaseErr
		}
	}
	if setup.modelRoot == nil {
		if err := os.MkdirAll(setup.modelDirectory, 0700); err != nil {
			return status, &browserSpeechError{category: modelassets.ErrFilesystem, cause: err}
		}
		modelRoot, err := os.OpenRoot(setup.modelDirectory)
		if err != nil {
			return status, &browserSpeechError{category: modelassets.ErrFilesystem, cause: err}
		}
		setup.modelRoot = modelRoot
	}
	install := setup.installSpeechModel
	if repair {
		install = setup.repairSpeechModel
	}
	if err := install(ctx, setup.modelRoot, report); err != nil {
		return status, err
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	if err := setup.loadInstalledTranscriber(ctx); err != nil {
		return status, err
	}
	return tui.SpeechModelSetupStatus{Ready: true}, nil
}

// Failed/cancelled attempts inspect only enough metadata to offer a safe next
// action. A completion marker alone never establishes Ready; Retry check must
// perform full verification and loading. Cancellation does not trigger a reload.
func (setup *browserSpeechSetup) statusAfterFailedSetup(operationErr error) tui.SpeechModelSetupStatus {
	if errors.Is(operationErr, speech.ErrRuntime) || errors.Is(operationErr, speech.ErrClosed) || setup.modelReleaseErr != nil {
		return tui.SpeechModelSetupStatus{}
	}
	if setup.modelRoot == nil {
		return tui.SpeechModelSetupStatus{CanDownload: !errors.Is(operationErr, modelassets.ErrFilesystem)}
	}
	revisionDirectory := string(setup.description.ID) + "/" + setup.description.Revision
	metadata, err := setup.modelRoot.Lstat(revisionDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return tui.SpeechModelSetupStatus{CanDownload: true}
	}
	if err != nil || !metadata.IsDir() || errors.Is(operationErr, modelassets.ErrFilesystem) {
		return tui.SpeechModelSetupStatus{}
	}
	if errors.Is(operationErr, modelassets.ErrNotInstalled) || errors.Is(operationErr, modelassets.ErrIntegrity) {
		return tui.SpeechModelSetupStatus{CanRepair: true}
	}
	marker, err := setup.modelRoot.Lstat(revisionDirectory + "/.complete")
	if errors.Is(err, os.ErrNotExist) || (err == nil && !marker.IsDir()) {
		return tui.SpeechModelSetupStatus{CanRepair: true}
	}
	return tui.SpeechModelSetupStatus{}
}

func (setup *browserSpeechSetup) RecordingConsumerFactory(preferences *editorprefs.Service) browserterm.PCMConsumerFactory {
	return func(ctx context.Context, recording voice.RecordingKey, publish func(voice.TranscriptUpdate) bool) (browserterm.RecordingPCMConsumer, error) {
		pause := 2 * time.Second
		if preferences != nil {
			settings, err := preferences.Load(ctx)
			if err != nil {
				return nil, err
			}
			pause = time.Duration(settings.SpeechPauseMS) * time.Millisecond
		}
		setup.mu.Lock()
		defer setup.mu.Unlock()
		transcriber := setup.transcriber
		closed := setup.closed
		if closed {
			return nil, speech.ErrClosed
		}
		if transcriber == nil || setup.modelReleaseErr != nil {
			return nil, speech.ErrRuntime
		}
		stream, err := transcriber.StartRecordingStream(ctx)
		if err != nil {
			return nil, err
		}
		return &browserSpeechConsumer{moonshineStream: stream, recording: recording, publish: publish, formatter: voice.TranscriptFormatter{Pause: pause}}, nil
	}
}
func (setup *browserSpeechSetup) Close() error {
	setup.mu.Lock()
	defer setup.mu.Unlock()
	if setup.closed {
		return setup.closeErr
	}
	setup.closed = true
	var transcriberErr, rootErr error
	if setup.transcriber != nil {
		transcriberErr = setup.transcriber.Close()
	}
	if setup.modelRoot != nil {
		rootErr = setup.modelRoot.Close()
	}
	if cause := errors.Join(transcriberErr, rootErr); cause != nil {
		setup.closeErr = &browserSpeechError{category: speech.ErrRuntime, cause: cause}
	}
	return setup.closeErr
}
