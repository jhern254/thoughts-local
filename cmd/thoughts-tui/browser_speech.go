package main

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/jhern254/go-thoughts/internal/speech"
	"github.com/jhern254/go-thoughts/internal/voice"
)

var errBrowserSpeechSetup = errors.New("Could not start speech recognition. Check the configured model installation and native runtime.")

// The interface keeps adapter tests independent of native models and libraries.
type recordingSpeechStream interface {
	AddAudio(ctx context.Context, audioSamples []float32, sampleRateHz int) (speech.Transcript, error)
	Close() error
}
type browserSpeechConsumer struct {
	moonshineStream         recordingSpeechStream
	recording               voice.RecordingKey
	publish                 func(voice.TranscriptUpdate) bool
	normalizedSamples       [1600]float32
	lastPublishedTranscript string
	transcriptRevision      uint64
	formatter               voice.TranscriptFormatter
}

func (consumer *browserSpeechConsumer) ConsumePCM(ctx context.Context, pcmSamples []int16) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(pcmSamples) != len(consumer.normalizedSamples) {
		return speech.ErrInvalidAudio
	}
	// Signed PCM16 has one extra negative value. Separate scales map both ends
	// to the float amplitude range exactly: -32768 → -1 and 32767 → 1.
	for sampleIndex, pcmSample := range pcmSamples {
		if pcmSample < 0 {
			consumer.normalizedSamples[sampleIndex] = float32(pcmSample) / 32768
		} else {
			consumer.normalizedSamples[sampleIndex] = float32(pcmSample) / 32767
		}
	}
	defer clear(consumer.normalizedSamples[:])
	transcript, err := consumer.moonshineStream.AddAudio(ctx, consumer.normalizedSamples[:], 16000)
	if lifecycleErr := ctx.Err(); lifecycleErr != nil {
		return lifecycleErr
	}
	if err != nil {
		return err
	}
	if !utf8.ValidString(transcript.Text) || len(transcript.Text) > voice.MaximumTranscriptBytes {
		return speech.ErrTranscription
	}
	transcript.Text = consumer.formatter.Format(transcript)
	if !utf8.ValidString(transcript.Text) || len(transcript.Text) > voice.MaximumTranscriptBytes {
		return speech.ErrTranscription
	}
	if transcript.Text == consumer.lastPublishedTranscript {
		return nil
	}
	nextRevision := consumer.transcriptRevision + 1
	update := voice.TranscriptUpdate{Recording: consumer.recording, Revision: nextRevision, Text: transcript.Text}
	if !consumer.publish(update) {
		if err := ctx.Err(); err != nil {
			return err
		}
		return speech.ErrTranscription
	}
	consumer.transcriptRevision = nextRevision
	consumer.lastPublishedTranscript = transcript.Text
	return nil
}
func (consumer *browserSpeechConsumer) Close() error {
	clear(consumer.normalizedSamples[:])
	consumer.lastPublishedTranscript = ""
	consumer.formatter = voice.TranscriptFormatter{}
	return consumer.moonshineStream.Close()
}

// Public text stays fixed; native diagnostics and model paths are reachable
// through errors.Is/As for controlled diagnostics, never presentation or logs.
type browserSpeechError struct {
	category error
	cause    error
}

func (err *browserSpeechError) Error() string   { return err.category.Error() }
func (err *browserSpeechError) Unwrap() []error { return []error{err.category, err.cause} }
