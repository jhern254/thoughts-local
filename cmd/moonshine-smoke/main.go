// moonshine-smoke is a developer tool. Installation and inference are explicit,
// separate commands; it does not capture audio or wire speech into the app.
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/speech"
	"github.com/jhern254/go-thoughts/internal/speech/moonshine"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(ctx context.Context, arguments []string, transcriptOutput, measurementsOutput io.Writer) (err error) {
	if len(arguments) == 0 || (arguments[0] != "install" && arguments[0] != "transcribe") {
		return errors.New("usage: moonshine-smoke install|transcribe -models-dir PATH [-pcm FILE -sample-rate 16000]")
	}
	operation := arguments[0]
	flags := flag.NewFlagSet("moonshine-smoke", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	modelDirectory := flags.String("models-dir", os.Getenv("THOUGHTS_MODELS_DIR"), "application-owned model root")
	pcmPath := flags.String("pcm", "", "float32 little-endian mono PCM file")
	sampleRateHz := flags.Int("sample-rate", 16000, "PCM sample rate in Hz")
	if parseErr := flags.Parse(arguments[1:]); parseErr != nil {
		return errors.New("invalid smoke command arguments")
	}
	if flags.NArg() != 0 || *modelDirectory == "" {
		return errors.New("model root is required")
	}
	if operation == "install" {
		return runInstall(ctx, *modelDirectory, *pcmPath, measurementsOutput)
	}
	return runTranscribe(ctx, *modelDirectory, *pcmPath, *sampleRateHz, transcriptOutput, measurementsOutput)
}

func runInstall(ctx context.Context, modelDirectory, pcmPath string, measurementsOutput io.Writer) (err error) {
	if pcmPath != "" {
		return errors.New("install does not accept audio")
	}
	if err := os.MkdirAll(modelDirectory, 0700); err != nil {
		return errors.New("cannot create model root")
	}
	modelRoot, err := os.OpenRoot(modelDirectory)
	if err != nil {
		return errors.New("cannot open model root")
	}
	defer func() {
		if closeErr := modelRoot.Close(); closeErr != nil {
			err = errors.Join(err, errors.New("cannot close model root"))
		}
	}()
	installer, err := modelassets.NewInstaller(modelRoot)
	if err != nil {
		return err
	}
	installation, err := installer.Install(ctx, modelassets.MoonshineSmallStreamingEnglish)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(measurementsOutput, "installed %s revision %s\n", installation.ModelID, installation.Revision)
	return err
}

func runTranscribe(ctx context.Context, modelDirectory, pcmPath string, sampleRateHz int, transcriptOutput, measurementsOutput io.Writer) (err error) {
	modelRoot, err := os.OpenRoot(modelDirectory)
	if err != nil {
		return errors.New("cannot open model root")
	}
	defer func() {
		if closeErr := modelRoot.Close(); closeErr != nil {
			err = errors.Join(err, errors.New("cannot close model root"))
		}
	}()
	if pcmPath == "" {
		return errors.New("transcribe requires an explicit PCM file")
	}
	pcmBytes, err := os.ReadFile(pcmPath)
	if err != nil {
		return errors.New("cannot read PCM fixture")
	}
	audioSamples, err := decodePCM(pcmBytes)
	if err != nil {
		return err
	}
	if sampleRateHz <= 0 {
		return speech.ErrInvalidAudio
	}
	loadStarted := time.Now()
	transcriber, err := moonshine.Open(ctx, modelRoot)
	if err != nil {
		return err
	}
	loadElapsed := time.Since(loadStarted)
	defer func() { err = errors.Join(err, transcriber.Close()) }()
	transcriptionStarted := time.Now()
	transcript, err := transcriber.Transcribe(ctx, audioSamples, sampleRateHz)
	if err != nil {
		return err
	}
	transcriptionElapsed := time.Since(transcriptionStarted)
	audioDurationSeconds := float64(len(audioSamples)) / float64(sampleRateHz)
	if _, err := fmt.Fprintf(measurementsOutput, "verified_model_open_seconds=%.6f audio_duration_seconds=%.6f transcription_seconds=%.6f real_time_factor=%.6f\n", loadElapsed.Seconds(), audioDurationSeconds, transcriptionElapsed.Seconds(), transcriptionElapsed.Seconds()/audioDurationSeconds); err != nil {
		return errors.New("cannot write smoke measurements")
	}
	// Transcript output is the explicit result of this developer command, not an
	// operational log. The adapter itself neither logs nor persists this text.
	if _, err := fmt.Fprintln(transcriptOutput, transcript.Text); err != nil {
		return errors.New("cannot write transcript result")
	}
	return nil
}
func decodePCM(encoded []byte) ([]float32, error) {
	if len(encoded) == 0 || len(encoded)%4 != 0 {
		return nil, speech.ErrInvalidAudio
	}
	audioSamples := make([]float32, len(encoded)/4)
	for sampleIndex := range audioSamples {
		audioSamples[sampleIndex] = math.Float32frombits(binary.LittleEndian.Uint32(encoded[sampleIndex*4:]))
	}
	return audioSamples, nil
}
