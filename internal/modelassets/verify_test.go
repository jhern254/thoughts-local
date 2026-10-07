package modelassets

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestCopyAndVerifyModelFile_Write(t *testing.T) {
	t.Run("short write is a filesystem failure", func(t *testing.T) {
		err := copyAndVerifyModelFile(context.Background(), strings.NewReader("tiny"), shortWriter{}, fixtureModelManifest("https://example.test/file", "tiny").Files[0], nil)
		assertError(t, err, ErrFilesystem)
		assertError(t, err, io.ErrShortWrite)
	})
}

type truncatedReader struct{}

func (truncatedReader) Read(p []byte) (int, error) { return copy(p, "tin"), io.ErrUnexpectedEOF }

func TestCopyAndVerifyModelFile_Truncation(t *testing.T) {
	t.Run("framing truncation is an integrity failure", func(t *testing.T) {
		err := copyAndVerifyModelFile(context.Background(), truncatedReader{}, io.Discard, fixtureModelManifest("https://example.test/file", "tiny").Files[0], nil)
		assertError(t, err, ErrIntegrity)
		assertError(t, err, io.ErrUnexpectedEOF)
	})
}

type countingReader struct {
	bytes  int
	reader io.Reader
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytes += n
	return n, err
}

type cancelingReader struct{ cancel context.CancelFunc }

func (reader cancelingReader) Read(buffer []byte) (int, error) {
	reader.cancel()
	return copy(buffer, "tiny"), nil
}

func TestCopyAndVerifyModelFile_Bounds(t *testing.T) {
	t.Run("reads at most the expected size plus one", func(t *testing.T) {
		r := &countingReader{reader: strings.NewReader(strings.Repeat("x", 100))}
		err := copyAndVerifyModelFile(context.Background(), r, io.Discard, fixtureModelManifest("https://example.test/file", "tiny").Files[0], nil)
		assertError(t, err, ErrIntegrity)
		if r.bytes != 5 {
			t.Fatalf("bytes read got %d, want 5", r.bytes)
		}
	})
	t.Run("checks cancellation while streaming discovery data", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := copyAndVerifyModelFile(ctx, cancelingReader{cancel: cancel}, io.Discard, fixtureModelManifest("https://example.test/file", "tiny").Files[0], nil)
		assertError(t, err, context.Canceled)
	})
}

// Allow preflight to succeed so interruption is observed with a model file open,
// without depending on file size, scheduler timing or wall-clock delays.
type contextInterruptedAfterPreflight struct {
	context.Context
	preflightChecked bool
}

func (ctx *contextInterruptedAfterPreflight) Err() error {
	if !ctx.preflightChecked {
		ctx.preflightChecked = true
		return nil
	}
	return ctx.Context.Err()
}

func TestInstaller_VerifyInstallationInterruption(t *testing.T) {
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	expiredContext, stopDeadline := context.WithDeadline(context.Background(), time.Time{})
	defer stopDeadline()
	for _, interruption := range []struct {
		name          string
		ctx           context.Context
		expectedError error
	}{
		{name: "cancellation", ctx: canceledContext, expectedError: context.Canceled},
		{name: "deadline", ctx: expiredContext, expectedError: context.DeadlineExceeded},
	} {
		t.Run(interruption.name, func(t *testing.T) {
			for _, closure := range []struct {
				name       string
				closeError error
			}{
				{name: "closes the file and returns a lifecycle result"},
				{name: "retains close failure without exposing its text", closeError: &os.PathError{Op: "close", Path: "sensitive-model-path", Err: errors.New("sensitive close cause")}},
			} {
				t.Run(closure.name, func(t *testing.T) {
					modelRoot := openTestModelRoot(t)
					manifest := fixtureModelManifest("https://example.test/file", "tiny")
					targetInstallation := installationForManifest(manifest)
					writeFixtureModelFile(t, modelRoot, targetInstallation.Directory, "tiny")
					if err := modelRoot.Mkdir(targetInstallation.Directory+"/.complete", 0700); err != nil {
						t.Fatal(err)
					}
					installer := newTestInstaller(t, modelRoot, []modelManifest{manifest}, &http.Client{})
					var closedModelFile *os.File
					installer.closeFile = func(modelFile *os.File) error {
						closedModelFile = modelFile
						return errors.Join(modelFile.Close(), closure.closeError)
					}
					ctx := &contextInterruptedAfterPreflight{Context: interruption.ctx}
					installation, err := installer.VerifyInstallation(ctx, manifest.ID)
					assertError(t, err, interruption.expectedError)
					if installation != (Installation{}) {
						t.Fatalf("installation got %v, want no verified installation", installation)
					}
					if err.Error() != interruption.expectedError.Error() {
						t.Errorf("public error got %q, want %q", err.Error(), interruption.expectedError.Error())
					}
					if got := errors.Is(err, ErrFilesystem); got != (closure.closeError != nil) {
						t.Errorf("filesystem category got %v, want %v", got, closure.closeError != nil)
					}
					if closure.closeError != nil {
						assertError(t, err, closure.closeError)
						var pathError *os.PathError
						if !errors.As(err, &pathError) {
							t.Error("close cause type was not retained")
						}
					}
					if closedModelFile == nil {
						t.Fatal("model file was not closed after interruption")
					}
					if _, writeErr := closedModelFile.Write(nil); !errors.Is(writeErr, os.ErrClosed) {
						t.Fatalf("write after verification got %v, want closed handle", writeErr)
					}
					assertNoInstallerArtifacts(t, modelRoot)
					installer.closeFile = (*os.File).Close
					if _, err := installer.VerifyInstallation(context.Background(), manifest.ID); err != nil {
						t.Fatalf("installation changed after interruption: %v", err)
					}
				})
			}
		})
	}
}
