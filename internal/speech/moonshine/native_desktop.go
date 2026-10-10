//go:build moonshine && cgo && ((linux && amd64) || (darwin && !ios && (amd64 || arm64)) || (windows && amd64))

package moonshine

/*
#cgo linux LDFLAGS: -lmoonshine
#cgo darwin LDFLAGS: -lmoonshine -lc++ -framework CoreFoundation -framework Foundation
#cgo windows LDFLAGS: -lthoughts-moonshine
#include <moonshine-c-api.h>
#include <stdlib.h>
#include <string.h>
#if MOONSHINE_HEADER_VERSION != 30000
#error Thoughts requires the pinned Moonshine v0.1.5 C API 3.0.0 header
#endif

struct thoughts_model_file {
 const char *filename;
 const uint8_t *data;
 uint64_t size;
};

static int32_t load_small_streaming(
 const struct thoughts_model_file *model_files,
 const struct moonshine_option_t *options, uint64_t options_count) {
 const char *filenames[8];
 const uint8_t *buffers[8];
 uint64_t sizes[8];
 for (int file_index = 0; file_index < 8; file_index++) {
  filenames[file_index] = model_files[file_index].filename;
  buffers[file_index] = model_files[file_index].data;
  sizes[file_index] = model_files[file_index].size;
 }
 return moonshine_load_transcriber_from_memory_files(
  filenames, buffers, sizes, 8, MOONSHINE_MODEL_ARCH_SMALL_STREAMING,
  options, options_count, MOONSHINE_HEADER_VERSION);
}
*/
import "C"

import (
	"context"
	"errors"
	"math"
	"os"
	"runtime"
	"unsafe"

	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/speech"
)

type nativeModelTranscriber struct {
	transcriberHandle C.int32_t
	modelFiles        *mappedModelFiles
}

// The caller holds moonshineRuntimeGate through loading and result ownership transfer.
// Only the read-only mappings outlive this call; temporary C names/options are
// copied by Moonshine. Native sessions reference the mappings until destruction.
func openNativeTranscriber(ctx context.Context, modelRoot *os.Root, installation modelassets.Installation) (nativeTranscriber, error) {
	if C.moonshine_get_version() != C.MOONSHINE_HEADER_VERSION {
		return nil, speech.ErrRuntime
	}
	modelFiles, err := mapModelFiles(ctx, modelRoot, installation)
	if err != nil {
		return nil, err
	}
	nativeBackend, err := loadMappedTranscriber(ctx, modelFiles)
	if err != nil {
		return nil, errors.Join(err, modelFiles.Close())
	}
	return nativeBackend, nil
}
func loadMappedTranscriber(ctx context.Context, modelFiles *mappedModelFiles) (nativeTranscriber, error) {
	if len(modelFiles.files) != len(smallStreamingModelFilenames()) {
		return nil, speech.ErrModelLoad
	}
	// Empty buffers enable upstream filesystem fallback instead of memory loading.
	for _, modelFile := range modelFiles.files {
		if len(modelFile.mappedBytes) == 0 {
			return nil, speech.ErrModelLoad
		}
	}

	nativeFileStorage := C.malloc(C.size_t(len(modelFiles.files)) * C.size_t(C.sizeof_struct_thoughts_model_file))
	if nativeFileStorage == nil {
		return nil, speech.ErrRuntime
	}
	defer C.free(nativeFileStorage)
	nativeFiles := unsafe.Slice((*C.struct_thoughts_model_file)(nativeFileStorage), len(modelFiles.files))
	for fileIndex, modelFile := range modelFiles.files {
		filename := C.CString(modelFile.filename)
		defer C.free(unsafe.Pointer(filename))
		nativeFiles[fileIndex].filename = filename
		nativeFiles[fileIndex].data = (*C.uint8_t)(unsafe.Pointer(&modelFile.mappedBytes[0]))
		nativeFiles[fileIndex].size = C.uint64_t(len(modelFile.mappedBytes))
	}
	runtimeOptions := smallStreamingRuntimeOptions()
	nativeOptionStorage := C.malloc(C.size_t(len(runtimeOptions)) * C.size_t(C.sizeof_struct_moonshine_option_t))
	if nativeOptionStorage == nil {
		return nil, speech.ErrRuntime
	}
	defer C.free(nativeOptionStorage)
	nativeOptions := unsafe.Slice((*C.struct_moonshine_option_t)(nativeOptionStorage), len(runtimeOptions))
	optionIndex := 0
	for optionName, optionValue := range runtimeOptions {
		nativeName := C.CString(optionName)
		nativeValue := C.CString(optionValue)
		defer C.free(unsafe.Pointer(nativeName))
		defer C.free(unsafe.Pointer(nativeValue))
		nativeOptions[optionIndex].name = nativeName
		nativeOptions[optionIndex].value = nativeValue
		optionIndex++
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	transcriberHandle := C.load_small_streaming(&nativeFiles[0], &nativeOptions[0], C.uint64_t(len(nativeOptions)))
	runtime.KeepAlive(modelFiles)
	if transcriberHandle < 0 {
		return nil, &NativeStatusError{Code: int32(transcriberHandle)}
	}
	return &nativeModelTranscriber{
		transcriberHandle: transcriberHandle,
		modelFiles:        modelFiles,
	}, nil
}
func (nativeBackend *nativeModelTranscriber) Transcribe(audioSamples []float32, sampleRateHz int) ([]string, error) {
	var nativeTranscript *C.struct_transcript_t
	nativeStatus := C.moonshine_transcribe_without_streaming(
		nativeBackend.transcriberHandle, (*C.float)(unsafe.Pointer(&audioSamples[0])),
		C.uint64_t(len(audioSamples)), C.int32_t(sampleRateHz), 0, &nativeTranscript)
	// The synchronous C call consumes the caller's PCM; it does not retain a Go
	// pointer. Returned audio is disabled and never traversed by this adapter.
	runtime.KeepAlive(audioSamples)
	if nativeStatus != 0 {
		return nil, &NativeStatusError{Code: int32(nativeStatus)}
	}
	return copyNativeTranscript(nativeTranscript)
}

// Moonshine owns this memory until the next call. Copying under the runtime
// gate keeps borrowed C text inside this boundary for batch and streaming calls.
func copyNativeTranscript(nativeTranscript *C.struct_transcript_t) ([]string, error) {
	if nativeTranscript == nil || uint64(nativeTranscript.line_count) > uint64(math.MaxInt) {
		return nil, &NativeStatusError{Code: C.MOONSHINE_ERROR_UNKNOWN}
	}
	if nativeTranscript.line_count == 0 {
		return nil, nil
	}
	if nativeTranscript.lines == nil {
		return nil, &NativeStatusError{Code: C.MOONSHINE_ERROR_UNKNOWN}
	}
	nativeLines := unsafe.Slice(nativeTranscript.lines, int(nativeTranscript.line_count))
	transcriptLines := make([]string, 0, len(nativeLines))
	for _, nativeLine := range nativeLines {
		if nativeLine.text == nil {
			continue
		}
		textLength := uint64(C.strlen(nativeLine.text))
		if textLength > uint64(math.MaxInt) {
			return nil, &NativeStatusError{Code: C.MOONSHINE_ERROR_UNKNOWN}
		}
		borrowedTextBytes := unsafe.Slice((*byte)(unsafe.Pointer(nativeLine.text)), int(textLength))
		transcriptLines = append(transcriptLines, string(borrowedTextBytes))
	}
	return transcriptLines, nil
}
func (nativeBackend *nativeModelTranscriber) Close() error {
	// Session destruction must precede unmapping: ONNX holds these buffer pointers.
	C.moonshine_free_transcriber(nativeBackend.transcriberHandle)
	return nativeBackend.modelFiles.Close()
}

type nativeModelStream struct {
	transcriberHandle C.int32_t
	streamHandle      C.int32_t
}

func (nativeBackend *nativeModelTranscriber) StartStream() (nativeStream, error) {
	streamHandle := C.moonshine_create_stream(nativeBackend.transcriberHandle, 0)
	if streamHandle < 0 {
		return nil, &NativeStatusError{Code: int32(streamHandle)}
	}
	nativeStream := &nativeModelStream{transcriberHandle: nativeBackend.transcriberHandle, streamHandle: streamHandle}
	if nativeStatus := C.moonshine_start_stream(nativeBackend.transcriberHandle, streamHandle); nativeStatus != 0 {
		freeStatus := C.moonshine_free_stream(nativeBackend.transcriberHandle, streamHandle)
		return nil, errors.Join(nativeStatusError(nativeStatus), nativeStatusError(freeStatus))
	}
	return nativeStream, nil
}
func (stream *nativeModelStream) AddAudio(audioSamples []float32, sampleRateHz int) error {
	nativeStatus := C.moonshine_transcribe_add_audio_to_stream(stream.transcriberHandle, stream.streamHandle,
		(*C.float)(unsafe.Pointer(&audioSamples[0])), C.uint64_t(len(audioSamples)), C.int32_t(sampleRateHz), 0)
	// Upstream copies PCM into its stream buffer; no Go pointer survives this call.
	runtime.KeepAlive(audioSamples)
	return nativeStatusError(nativeStatus)
}
func (stream *nativeModelStream) Transcribe() ([]string, error) {
	var nativeTranscript *C.struct_transcript_t
	nativeStatus := C.moonshine_transcribe_stream(stream.transcriberHandle, stream.streamHandle, 0, &nativeTranscript)
	if err := nativeStatusError(nativeStatus); err != nil {
		return nil, err
	}
	return copyNativeTranscript(nativeTranscript)
}
func (stream *nativeModelStream) Close() error {
	// Thoughts revokes transcript authority on recording Stop. Intentionally
	// follow moonshine_stop_stream with moonshine_free_stream, without a final
	// moonshine_transcribe_stream, so cleanup does not produce text after Stop.
	stopStatus := C.moonshine_stop_stream(stream.transcriberHandle, stream.streamHandle)
	freeStatus := C.moonshine_free_stream(stream.transcriberHandle, stream.streamHandle)
	return errors.Join(nativeStatusError(stopStatus), nativeStatusError(freeStatus))
}
func nativeStatusError(nativeStatus C.int32_t) error {
	if nativeStatus == 0 {
		return nil
	}
	return &NativeStatusError{Code: int32(nativeStatus)}
}
