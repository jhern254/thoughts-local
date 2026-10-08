//go:build (linux && amd64) || (darwin && !ios && (amd64 || arm64))

package moonshine

import (
	"errors"
	"os"
	"syscall"
)

type mappedModelFile struct {
	filename    string
	mappedBytes []byte
	closeErr    error
}

func mapModelFile(modelRoot *os.Root, relativePath string) (*mappedModelFile, error) {
	modelFile, sizeBytes, err := openModelFileForMapping(modelRoot, relativePath)
	if err != nil {
		return nil, err
	}
	mappedBytes, mapErr := syscall.Mmap(int(modelFile.Fd()), 0, sizeBytes, syscall.PROT_READ, syscall.MAP_PRIVATE)
	closeErr := modelFile.Close()
	if mapErr != nil || closeErr != nil {
		var unmapErr error
		if mappedBytes != nil {
			unmapErr = syscall.Munmap(mappedBytes)
		}
		return nil, errors.Join(mapErr, closeErr, unmapErr)
	}
	return &mappedModelFile{mappedBytes: mappedBytes}, nil
}

func (modelFile *mappedModelFile) Close() error {
	if modelFile.mappedBytes != nil {
		modelFile.closeErr = syscall.Munmap(modelFile.mappedBytes)
		modelFile.mappedBytes = nil
	}
	return modelFile.closeErr
}
