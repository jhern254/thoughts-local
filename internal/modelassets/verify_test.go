package modelassets

import (
	"context"
	"io"
	"strings"
	"testing"
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
