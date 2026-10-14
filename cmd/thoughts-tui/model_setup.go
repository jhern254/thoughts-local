package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

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
type browserModelSetup struct {
	modelDirectory  string
	description     modelassets.ModelDescription
	checkRuntime    func(context.Context) error
	loadTranscriber func(context.Context, *os.Root) (browserSpeechTranscriber, error)
	installModel    func(context.Context, *os.Root, func(modelassets.DownloadProgress)) error

	mu          sync.Mutex
	modelRoot   *os.Root
	transcriber browserSpeechTranscriber
	closed      bool
	closeErr    error
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
	return filepath.Join(homeDirectory, "Thoughts", "models")
}
func newBrowserModelSetup(configuredDirectory string) (*browserModelSetup, error) {
	homeDirectory, err := os.UserHomeDir()
	if configuredDirectory == "" && err != nil {
		return nil, &browserSpeechError{category: errBrowserSpeechSetup, cause: err}
	}
	description, err := modelassets.DescribeModel(modelassets.MoonshineSmallStreamingEnglish)
	if err != nil {
		return nil, err
	}
	return &browserModelSetup{
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
		installModel: func(ctx context.Context, root *os.Root, report func(modelassets.DownloadProgress)) error {
			installer, err := modelassets.NewInstaller(root)
			if err != nil {
				return err
			}
			_, err = installer.InstallWithProgress(ctx, modelassets.MoonshineSmallStreamingEnglish, report)
			return err
		},
	}, nil
}
func (setup *browserModelSetup) Description() modelassets.ModelDescription { return setup.description }
func (setup *browserModelSetup) Directory() string                         { return setup.modelDirectory }
func (setup *browserModelSetup) LoadInstalledModel(ctx context.Context) (tui.ModelSetupStatus, error) {
	if err := ctx.Err(); err != nil {
		return tui.ModelSetupStatus{}, err
	}
	setup.mu.Lock()
	ready := setup.transcriber != nil
	closed := setup.closed
	setup.mu.Unlock()
	if closed {
		return tui.ModelSetupStatus{}, speech.ErrClosed
	}
	if ready {
		return tui.ModelSetupStatus{Ready: true}, nil
	}
	if err := setup.checkRuntime(ctx); err != nil {
		if ctx.Err() != nil {
			return tui.ModelSetupStatus{}, ctx.Err()
		}
		return tui.ModelSetupStatus{Message: "Local speech isn't available in this build. You can still type and save."}, nil
	}
	if setup.modelRoot == nil {
		modelRoot, err := os.OpenRoot(setup.modelDirectory)
		if errors.Is(err, os.ErrNotExist) {
			return tui.ModelSetupStatus{CanDownload: true}, nil
		}
		if err != nil {
			return tui.ModelSetupStatus{}, &browserSpeechError{category: modelassets.ErrFilesystem, cause: err}
		}
		setup.modelRoot = modelRoot
	}
	if err := setup.loadInstalledTranscriber(ctx); err != nil {
		if errors.Is(err, modelassets.ErrNotInstalled) {
			// Absence permits a new installation; an incomplete destination must never
			// be overwritten. Neither existence check establishes model validity.
			_, statErr := setup.modelRoot.Lstat(string(setup.description.ID) + "/" + setup.description.Revision)
			if errors.Is(statErr, os.ErrNotExist) {
				return tui.ModelSetupStatus{CanDownload: true}, nil
			}
		}
		return tui.ModelSetupStatus{}, err
	}
	return tui.ModelSetupStatus{Ready: true}, nil
}
func (setup *browserModelSetup) loadInstalledTranscriber(ctx context.Context) error {
	transcriber, err := setup.loadTranscriber(ctx, setup.modelRoot)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return &browserSpeechError{category: err, cause: errors.Join(err, transcriber.Close())}
	}
	setup.mu.Lock()
	defer setup.mu.Unlock()
	setup.transcriber = transcriber
	return nil
}
func (setup *browserModelSetup) InstallAndEnableModel(ctx context.Context, report func(modelassets.DownloadProgress)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	setup.mu.Lock()
	closed, ready := setup.closed, setup.transcriber != nil
	setup.mu.Unlock()
	if closed {
		return speech.ErrClosed
	}
	if ready {
		return nil
	}
	if err := setup.checkRuntime(ctx); err != nil {
		return err
	}
	if setup.modelRoot == nil {
		if err := os.MkdirAll(setup.modelDirectory, 0700); err != nil {
			return &browserSpeechError{category: modelassets.ErrFilesystem, cause: err}
		}
		modelRoot, err := os.OpenRoot(setup.modelDirectory)
		if err != nil {
			return &browserSpeechError{category: modelassets.ErrFilesystem, cause: err}
		}
		setup.modelRoot = modelRoot
	}
	if err := setup.installModel(ctx, setup.modelRoot, report); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return setup.loadInstalledTranscriber(ctx)
}
func (setup *browserModelSetup) ConsumerFactory() browserterm.PCMConsumerFactory {
	return func(ctx context.Context, recording voice.RecordingKey, publish func(voice.TranscriptUpdate) bool) (browserterm.RecordingPCMConsumer, error) {
		setup.mu.Lock()
		transcriber := setup.transcriber
		closed := setup.closed
		setup.mu.Unlock()
		if closed {
			return nil, speech.ErrClosed
		}
		if transcriber == nil {
			return nil, speech.ErrRuntime
		}
		stream, err := transcriber.StartRecordingStream(ctx)
		if err != nil {
			return nil, err
		}
		return &browserSpeechConsumer{moonshineStream: stream, recording: recording, publish: publish}, nil
	}
}
func (setup *browserModelSetup) Close() error {
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
