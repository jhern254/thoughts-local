//go:build (linux && amd64) || (darwin && !ios && (amd64 || arm64)) || (windows && amd64)

package moonshine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/jhern254/go-thoughts/internal/modelassets"
)

const windowsSymlinkPrivilegeNotHeld = syscall.Errno(1314)

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
		if len(modelFiles.files) != 8 {
			t.Fatalf("buffers got %d, want 8", len(modelFiles.files))
		}
		for _, modelFile := range modelFiles.files {
			if string(modelFile.mappedBytes) != "model" {
				t.Fatalf("mapped file %s: got %q, want model", modelFile.filename, modelFile.mappedBytes)
			}
		}
		for range 2 {
			if err := modelFiles.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if len(modelFiles.files) != 0 {
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
			if errors.Is(err, os.ErrPermission) || (runtime.GOOS == "windows" && errors.Is(err, windowsSymlinkPrivilegeNotHeld)) {
				t.Skip("symlink privileges unavailable")
			}
			t.Fatal(err)
		}
		if _, err := mapModelFiles(context.Background(), modelRoot, modelassets.Installation{Directory: "."}); err == nil {
			t.Fatal("mapped outside model root")
		}
		mappedBytes, err := os.ReadFile(outside)
		if err != nil {
			t.Fatal(err)
		}
		if string(mappedBytes) != "outside" {
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
		var firstMappedFile *mappedModelFile
		modelFiles, err := mapModelFilesWithMapper(context.Background(), modelRoot, modelassets.Installation{Directory: "."}, func(root *os.Root, relativePath string) (*mappedModelFile, error) {
			modelFile, err := mapModelFile(root, relativePath)
			if firstMappedFile == nil {
				firstMappedFile = modelFile
			}
			return modelFile, err
		})
		if err == nil || modelFiles != nil {
			t.Fatalf("got %v, %v, want failed construction", modelFiles, err)
		}
		if firstMappedFile == nil {
			t.Fatal("first model file was never mapped")
		}
		if firstMappedFile.mappedBytes != nil {
			t.Fatal("failed construction retained earlier mapping")
		}
	})
	t.Run("cancellation after mapping releases the created view", func(t *testing.T) {
		modelRoot, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer modelRoot.Close()
		if err := modelRoot.WriteFile(smallStreamingModelFilenames()[0], []byte("model"), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var mappedFile *mappedModelFile
		modelFiles, err := mapModelFilesWithMapper(ctx, modelRoot, modelassets.Installation{Directory: "."}, func(root *os.Root, relativePath string) (*mappedModelFile, error) {
			mappedFile, err = mapModelFile(root, relativePath)
			cancel()
			return mappedFile, err
		})
		if modelFiles != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, %v, want canceled construction", modelFiles, err)
		}
		if mappedFile == nil || mappedFile.mappedBytes != nil {
			t.Fatal("canceled construction retained mapping")
		}
	})
	for _, scenario := range []struct {
		description string
		createFile  func(*os.Root) error
	}{
		{description: "rejects an empty model file", createFile: func(root *os.Root) error { return root.WriteFile("model", nil, 0600) }},
		{description: "rejects a directory instead of a model file", createFile: func(root *os.Root) error { return root.Mkdir("model", 0700) }},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			modelRoot, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer modelRoot.Close()
			if err := scenario.createFile(modelRoot); err != nil {
				t.Fatal(err)
			}
			mappedFile, err := mapModelFile(modelRoot, "model")
			if err == nil || mappedFile != nil {
				t.Fatalf("got %v, %v, want rejected model file", mappedFile, err)
			}
		})
	}
}
