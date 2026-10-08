//go:build windows && amd64

package moonshine

import (
	"errors"
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

type mappedModelFile struct {
	filename             string
	mappedBytes          []byte
	windowsMappingHandle syscall.Handle
	mappedAddress        uintptr
	closeErr             error
}

func mapModelFile(modelRoot *os.Root, relativePath string) (*mappedModelFile, error) {
	modelFile, sizeBytes, err := openModelFileForMapping(modelRoot, relativePath)
	if err != nil {
		return nil, err
	}
	windowsMappingHandle, mapErr := syscall.CreateFileMapping(syscall.Handle(modelFile.Fd()), nil, syscall.PAGE_READONLY, 0, 0, nil)
	closeErr := modelFile.Close()
	if mapErr != nil || closeErr != nil {
		var mappingCloseErr error
		if windowsMappingHandle != 0 {
			mappingCloseErr = syscall.CloseHandle(windowsMappingHandle)
		}
		return nil, errors.Join(mapErr, closeErr, mappingCloseErr)
	}
	mappedAddress, err := syscall.MapViewOfFile(windowsMappingHandle, syscall.FILE_MAP_READ, 0, 0, uintptr(sizeBytes))
	if err != nil {
		return nil, errors.Join(err, syscall.CloseHandle(windowsMappingHandle))
	}
	// Windows returns an integer address for OS-owned pages. This header belongs
	// to an actual slice, not a standalone SliceHeader allocation. The mapping
	// handle and view retain stable read-only storage until native destruction.
	var mappedBytes []byte
	mappedSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&mappedBytes))
	mappedSliceHeader.Data = mappedAddress
	mappedSliceHeader.Len = sizeBytes
	mappedSliceHeader.Cap = sizeBytes
	return &mappedModelFile{
		mappedBytes:          mappedBytes,
		windowsMappingHandle: windowsMappingHandle,
		mappedAddress:        mappedAddress,
	}, nil
}

func (modelFile *mappedModelFile) Close() error {
	if modelFile.mappedAddress != 0 {
		modelFile.closeErr = errors.Join(modelFile.closeErr, syscall.UnmapViewOfFile(modelFile.mappedAddress))
		modelFile.mappedAddress = 0
		modelFile.mappedBytes = nil
	}
	if modelFile.windowsMappingHandle != 0 {
		modelFile.closeErr = errors.Join(modelFile.closeErr, syscall.CloseHandle(modelFile.windowsMappingHandle))
		modelFile.windowsMappingHandle = 0
	}
	return modelFile.closeErr
}
