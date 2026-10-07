//go:build linux && amd64

package moonshine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/jhern254/go-thoughts/internal/modelassets"
)

func TestModelFiles_Ownership(t *testing.T) {
	t.Run("maps confined model buffers and releases them idempotently", func(t *testing.T) {
		directory := t.TempDir()
		modelRoot, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer modelRoot.Close()
		for _, filename := range smallStreamingModelFilenames() {
			if err := modelRoot.WriteFile(filename, []byte("model"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		modelFiles, err := mapModelFiles(context.Background(), modelRoot, modelassets.Installation{Directory: "."})
		if err != nil {
			t.Fatal(err)
		}
		if len(modelFiles.buffers) != 8 {
			t.Fatalf("buffers got %d, want 8", len(modelFiles.buffers))
		}
		for _, buffer := range modelFiles.buffers {
			if string(buffer.contents) != "model" {
				t.Fatal("wrong mapped contents")
			}
		}
		for range 2 {
			if err := modelFiles.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if len(modelFiles.buffers) != 0 {
			t.Fatal("closed mappings retained")
		}
	})
	t.Run("rejects symlinks outside the root", func(t *testing.T) {
		directory := t.TempDir()
		outside := filepath.Join(t.TempDir(), "sentinel")
		if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
			t.Fatal(err)
		}
		modelRoot, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer modelRoot.Close()
		if err := modelRoot.Symlink(outside, smallStreamingModelFilenames()[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := mapModelFiles(context.Background(), modelRoot, modelassets.Installation{Directory: "."}); err == nil {
			t.Fatal("mapped outside model root")
		}
		contents, err := os.ReadFile(outside)
		if err != nil {
			t.Fatal(err)
		}
		if string(contents) != "outside" {
			t.Fatal("outside sentinel changed")
		}
	})
	t.Run("cancellation prevents mapping", func(t *testing.T) {
		modelRoot, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer modelRoot.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := mapModelFiles(ctx, modelRoot, modelassets.Installation{Directory: "."}); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want canceled", err)
		}
	})
	t.Run("partial mapping failure releases preceding buffers", func(t *testing.T) {
		modelRoot, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer modelRoot.Close()
		if err := modelRoot.WriteFile(smallStreamingModelFilenames()[0], []byte("model"), 0600); err != nil {
			t.Fatal(err)
		}
		modelFiles, err := mapModelFiles(context.Background(), modelRoot, modelassets.Installation{Directory: "."})
		if err == nil || modelFiles != nil {
			t.Fatalf("got %v, %v, want failed construction", modelFiles, err)
		}
	})
}

func TestModelFiles_Close(t *testing.T) {
	t.Run("attempts remaining unmaps after a failure and retains the original outcome", func(t *testing.T) {
		modelRoot, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer modelRoot.Close()
		if err := modelRoot.WriteFile("model", []byte("model"), 0600); err != nil {
			t.Fatal(err)
		}
		modelBytes, err := mapModelFile(modelRoot, "model")
		if err != nil {
			t.Fatal(err)
		}
		modelFiles := &mappedModelFiles{buffers: []mappedModelFile{{filename: "invalid", contents: []byte{1}}, {filename: "model", contents: modelBytes}}}
		for range 2 {
			if err := modelFiles.Close(); !errors.Is(err, syscall.EINVAL) {
				t.Fatalf("got %v, want retained unmap failure", err)
			}
		}
		if err := syscall.Munmap(modelBytes); !errors.Is(err, syscall.EINVAL) {
			t.Fatalf("valid mapping was not already released: %v", err)
		}
	})
}
