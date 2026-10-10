//go:build moonshine_integration && moonshine && cgo && ((linux && amd64) || (darwin && !ios && (amd64 || arm64)) || (windows && amd64))

package moonshine

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/jhern254/go-thoughts/internal/modelassets"
)

func TestMoonshineNative_PCM(t *testing.T) {
	modelDirectory := os.Getenv("THOUGHTS_MODELS_DIR")
	if modelDirectory == "" {
		t.Fatal("opt-in test requires THOUGHTS_MODELS_DIR with explicitly installed assets")
	}
	modelDirectory, err := filepath.Abs(modelDirectory)
	if err != nil {
		t.Fatal(err)
	}
	fixtureBytes, err := os.ReadFile("testdata/1272-128104-0000.f32le")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(fixtureBytes)
	if hex.EncodeToString(digest[:]) != "22d472b40f913206b6917115d136306be88df3b278e77f68be5910423250fa4d" {
		t.Fatal("licensed fixture checksum differs")
	}
	audioSamples := make([]float32, len(fixtureBytes)/4)
	for sampleIndex := range audioSamples {
		audioSamples[sampleIndex] = math.Float32frombits(binary.LittleEndian.Uint32(fixtureBytes[sampleIndex*4:]))
	}
	modelRoot, err := os.OpenRoot(modelDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := modelRoot.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	installer, err := modelassets.NewInstaller(modelRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.VerifyInstallation(ctx, modelassets.MoonshineSmallStreamingEnglish); err != nil {
		t.Fatal(err)
	}

	t.Run("recognizes licensed speech and preserves copied text after silence and closure", func(t *testing.T) {
		workingDirectory := t.TempDir()
		t.Chdir(workingDirectory)
		loadStarted := time.Now()
		transcriber, err := Open(ctx, modelRoot)
		if err != nil {
			t.Fatal(err)
		}
		loadElapsed := time.Since(loadStarted)
		t.Cleanup(func() {
			if err := transcriber.Close(); err != nil {
				t.Error(err)
			}
		})
		transcriptionStarted := time.Now()
		transcript, err := transcriber.Transcribe(ctx, audioSamples, 16000)
		if err != nil {
			t.Fatal(err)
		}
		transcriptionElapsed := time.Since(transcriptionStarted)
		normalizedText := strings.ToLower(strings.Join(strings.FieldsFunc(transcript.Text, func(character rune) bool { return !unicode.IsLetter(character) }), " "))
		if !strings.Contains(normalizedText, "middle classes") || !strings.Contains(normalizedText, "gospel") {
			t.Fatal("recognition did not contain the reference phrases")
		}
		nativeLines, originalNativeLines := func() ([]string, []string) {
			if err := acquireMoonshineRuntime(ctx); err != nil {
				t.Fatal(err)
			}
			defer releaseMoonshineRuntime()
			nativeLines, err := transcriber.state.nativeBackend.Transcribe(audioSamples, 16000)
			if err != nil {
				t.Fatal(err)
			}
			if len(nativeLines) == 0 {
				t.Fatal("native speech result had no lines")
			}
			originalNativeLines := make([]string, len(nativeLines))
			for lineIndex, transcriptText := range nativeLines {
				originalNativeLines[lineIndex] = strings.Clone(transcriptText)
			}
			if _, err := transcriber.state.nativeBackend.Transcribe(make([]float32, 32000), 16000); err != nil {
				t.Fatal(err)
			}
			for lineIndex, transcriptText := range nativeLines {
				if transcriptText != originalNativeLines[lineIndex] {
					t.Fatal("later native inference mutated returned Go-owned line")
				}
			}
			return nativeLines, originalNativeLines
		}()

		originalText := strings.Clone(transcript.Text)
		silence, err := transcriber.Transcribe(ctx, make([]float32, 32000), 16000)
		if err != nil {
			t.Fatal(err)
		}
		if silence.Text != "" {
			t.Fatal("silence produced unexpected text")
		}
		if err := transcriber.Close(); err != nil {
			t.Fatal(err)
		}
		for lineIndex, transcriptText := range nativeLines {
			if transcriptText != originalNativeLines[lineIndex] {
				t.Fatal("native closure mutated returned Go-owned line")
			}
		}

		if transcript.Text != originalText {
			t.Fatal("native result invalidation changed returned Go text")
		}
		createdFiles, err := os.ReadDir(workingDirectory)
		if err != nil {
			t.Fatal(err)
		}
		if len(createdFiles) != 0 {
			t.Fatal("native inference wrote files in its working directory")
		}
		audioDurationSeconds := float64(len(audioSamples)) / 16000
		t.Logf("verified_model_open_seconds=%.6f audio_duration_seconds=%.6f transcription_seconds=%.6f real_time_factor=%.6f", loadElapsed.Seconds(), audioDurationSeconds, transcriptionElapsed.Seconds(), transcriptionElapsed.Seconds()/audioDurationSeconds)
	})
	t.Run("streams 100 ms chunks and reuses the loaded model without final drain", func(t *testing.T) {
		workingDirectory := t.TempDir()
		t.Chdir(workingDirectory)
		transcriber, err := Open(ctx, modelRoot)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := transcriber.Close(); err != nil {
				t.Error(err)
			}
		})
		stream, err := transcriber.StartStream(ctx)
		if err != nil {
			t.Fatal(err)
		}
		frame := make([]float32, 1600)
		var transcriptText string
		for offset := 0; offset < len(audioSamples); offset += len(frame) {
			clear(frame)
			copy(frame, audioSamples[offset:min(offset+len(frame), len(audioSamples))])
			transcript, err := stream.AddAudio(ctx, frame, 16000)
			if err != nil {
				t.Fatal(err)
			}
			transcriptText = transcript.Text
		}
		// Silence arrives while the recording is active, allowing the engine to
		// complete the spoken phrase without changing Thoughts' hard Stop boundary.
		clear(frame)
		for range 20 {
			transcript, err := stream.AddAudio(ctx, frame, 16000)
			if err != nil {
				t.Fatal(err)
			}
			transcriptText = transcript.Text
		}
		normalizedText := strings.ToLower(strings.Join(strings.FieldsFunc(transcriptText, func(character rune) bool { return !unicode.IsLetter(character) }), " "))
		if !strings.Contains(normalizedText, "middle classes") || !strings.Contains(normalizedText, "gospel") {
			t.Fatal("streaming recognition did not contain reference phrases")
		}
		originalText := strings.Clone(transcriptText)
		if err := stream.Close(); err != nil {
			t.Fatal(err)
		}
		nextStream, err := transcriber.StartStream(ctx)
		if err != nil {
			t.Fatal(err)
		}
		silence, err := nextStream.AddAudio(ctx, frame, 16000)
		if err != nil {
			t.Fatal(err)
		}
		if silence.Text != "" {
			t.Fatal("new silent recording inherited previous transcript")
		}
		if err := transcriber.Close(); err != nil {
			t.Fatal(err)
		}
		if transcriptText != originalText {
			t.Fatal("later inference or closure mutated Go-owned text")
		}
		if err := nextStream.Close(); err != nil {
			t.Fatal(err)
		}
		createdFiles, err := os.ReadDir(workingDirectory)
		if err != nil {
			t.Fatal(err)
		}
		if len(createdFiles) != 0 {
			t.Fatal("streaming inference wrote unexpected files")
		}
	})

}
