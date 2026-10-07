//go:build linux && amd64

package moonshine

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"

	"github.com/jhern254/go-thoughts/internal/modelassets"
)

func smallStreamingModelFilenames() [8]string {
	return [8]string{
		"adapter.ort",
		"cross_kv.ort",
		"decoder_kv.ort",
		"encoder.ort",
		"frontend.model.ort",
		"frontend.weights.ort",
		"streaming_config.json",
		"tokenizer.bin",
	}
}

type mappedModelFile struct {
	filename string
	contents []byte
}
type mappedModelFiles struct {
	buffers  []mappedModelFile
	closeErr error
}

// Native ONNX sessions retain these non-Go buffers. Opening descriptors through
// os.Root confines access without giving the native runtime a filesystem path.
// Only call after VerifyInstallation; mapped files must remain unmodified.
func mapModelFiles(ctx context.Context, modelRoot *os.Root, installation modelassets.Installation) (*mappedModelFiles, error) {
	modelFiles := &mappedModelFiles{}
	for _, filename := range smallStreamingModelFilenames() {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(err, modelFiles.Close())
		}
		contents, err := mapModelFile(modelRoot, installation.Directory+"/"+filename)
		if err != nil {
			return nil, errors.Join(err, modelFiles.Close())
		}
		modelFiles.buffers = append(modelFiles.buffers, mappedModelFile{filename: filename, contents: contents})
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, modelFiles.Close())
	}
	return modelFiles, nil
}
func mapModelFile(modelRoot *os.Root, relativePath string) ([]byte, error) {
	modelFile, err := modelRoot.Open(relativePath)
	if err != nil {
		return nil, err
	}
	metadata, err := modelFile.Stat()
	if err != nil {
		return nil, errors.Join(err, modelFile.Close())
	}
	if !metadata.Mode().IsRegular() || metadata.Size() <= 0 {
		return nil, errors.Join(io.ErrUnexpectedEOF, modelFile.Close())
	}
	contents, mapErr := syscall.Mmap(int(modelFile.Fd()), 0, int(metadata.Size()), syscall.PROT_READ, syscall.MAP_PRIVATE)
	closeErr := modelFile.Close()
	if mapErr != nil || closeErr != nil {
		var unmapErr error
		if contents != nil {
			unmapErr = syscall.Munmap(contents)
		}
		return nil, errors.Join(mapErr, closeErr, unmapErr)
	}
	return contents, nil
}
func (modelFiles *mappedModelFiles) Close() error {
	for _, buffer := range modelFiles.buffers {
		modelFiles.closeErr = errors.Join(modelFiles.closeErr, syscall.Munmap(buffer.contents))
	}
	modelFiles.buffers = nil
	return modelFiles.closeErr
}
