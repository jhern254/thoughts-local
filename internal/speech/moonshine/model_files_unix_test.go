//go:build (linux && amd64) || (darwin && !ios && (amd64 || arm64))

package moonshine

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

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
		modelFile, err := mapModelFile(modelRoot, "model")
		if err != nil {
			t.Fatal(err)
		}
		modelBytes := modelFile.mappedBytes
		modelFiles := &mappedModelFiles{files: []*mappedModelFile{{filename: "invalid", mappedBytes: []byte{1}}, modelFile}}
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
