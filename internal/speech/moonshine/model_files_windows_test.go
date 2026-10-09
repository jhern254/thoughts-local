//go:build windows && amd64

package moonshine

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

const windowsInvalidHandle = syscall.Errno(6)

func TestModelFiles_WindowsOwnership(t *testing.T) {
	t.Run("releases both the mapped view and mapping handle", func(t *testing.T) {
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
		mappedAddress := modelFile.mappedAddress
		windowsMappingHandle := modelFile.windowsMappingHandle
		for range 2 {
			if err := modelFile.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if err := syscall.UnmapViewOfFile(mappedAddress); err == nil {
			t.Fatal("Close retained the mapped view")
		}
		if unexpectedAddress, err := syscall.MapViewOfFile(windowsMappingHandle, syscall.FILE_MAP_READ, 0, 0, 1); err == nil {
			syscall.UnmapViewOfFile(unexpectedAddress)
			t.Fatal("Close retained the mapping handle")
		}
	})
	t.Run("retains cleanup failure while releasing remaining mappings", func(t *testing.T) {
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
		mappedAddress := modelFile.mappedAddress
		modelFiles := &mappedModelFiles{files: []*mappedModelFile{
			{filename: "invalid", windowsMappingHandle: syscall.Handle(1)}, modelFile,
		}}
		firstCloseErr := modelFiles.Close()
		if !errors.Is(firstCloseErr, windowsInvalidHandle) {
			t.Fatalf("got %v, want invalid mapping handle", firstCloseErr)
		}
		if err := modelFiles.Close(); err != firstCloseErr {
			t.Fatalf("got %v, want stored close error", err)
		}
		if err := syscall.UnmapViewOfFile(mappedAddress); err == nil {
			t.Fatal("failed cleanup left the valid view mapped")
		}
	})
}
