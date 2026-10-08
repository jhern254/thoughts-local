//go:build (linux && amd64) || (darwin && !ios && (amd64 || arm64)) || (windows && amd64)

package moonshine

import (
	"context"
	"errors"
	"io"
	"math"
	"os"

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

type mappedModelFiles struct {
	files    []*mappedModelFile
	closeErr error
}

// Native ONNX sessions retain these non-Go buffers. Opening through os.Root
// confines access without giving native code a filesystem path. Only call after
// VerifyInstallation; installed files must stay unmodified until Close.
func mapModelFiles(ctx context.Context, modelRoot *os.Root, installation modelassets.Installation) (*mappedModelFiles, error) {
	return mapModelFilesWithMapper(ctx, modelRoot, installation, mapModelFile)
}

func mapModelFilesWithMapper(
	ctx context.Context,
	modelRoot *os.Root,
	installation modelassets.Installation,
	mapFile func(*os.Root, string) (*mappedModelFile, error),
) (*mappedModelFiles, error) {
	modelFiles := &mappedModelFiles{}
	for _, filename := range smallStreamingModelFilenames() {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(err, modelFiles.Close())
		}
		modelFile, err := mapFile(modelRoot, installation.Directory+"/"+filename)
		if err != nil {
			return nil, errors.Join(err, modelFiles.Close())
		}
		modelFile.filename = filename
		modelFiles.files = append(modelFiles.files, modelFile)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, modelFiles.Close())
	}
	return modelFiles, nil
}

func openModelFileForMapping(modelRoot *os.Root, relativePath string) (*os.File, int, error) {
	modelFile, err := modelRoot.Open(relativePath)
	if err != nil {
		return nil, 0, err
	}
	metadata, err := modelFile.Stat()
	if err != nil {
		return nil, 0, errors.Join(err, modelFile.Close())
	}
	if !metadata.Mode().IsRegular() || metadata.Size() <= 0 || metadata.Size() > int64(math.MaxInt) {
		return nil, 0, errors.Join(io.ErrUnexpectedEOF, modelFile.Close())
	}
	return modelFile, int(metadata.Size()), nil
}

func (modelFiles *mappedModelFiles) Close() error {
	for _, modelFile := range modelFiles.files {
		modelFiles.closeErr = errors.Join(modelFiles.closeErr, modelFile.Close())
	}
	modelFiles.files = nil
	return modelFiles.closeErr
}
