# Local Moonshine PCM smoke test

This developer-only path proves approved model assets → local native inference
→ transcript text. It does not capture microphones or connect speech to Thoughts'
browser, TUI, application runtime or database. Ordinary builds and tests require
no native runtime; opening verified assets in those builds returns
`speech.ErrRuntime`.

## Upstream contract

The integration pins Moonshine Voice v0.1.5, upstream commit
`234f60faa0eb388b01cdf7e60aca232af37aefda`, C header/runtime version **3.0.0**
(`30000`) and architecture `SMALL_STREAMING` (`4`). The package version and C API
version are different. The header and runtime version must match this pin.

Official sources:

- https://github.com/moonshine-ai/moonshine/releases/tag/v0.1.5
- https://github.com/moonshine-ai/moonshine/blob/v0.1.5/core/moonshine-c-api.h
- https://github.com/moonshine-ai/moonshine/blob/v0.1.5/core/moonshine-model-catalog.cpp
- https://github.com/moonshine-ai/moonshine/blob/v0.1.5/core/moonshine-model-file-metadata.generated.cpp
- https://moonshine-voice.readthedocs.io/en/stable/api/options/

The approved model ID is `moonshine-small-streaming-en`, revision
`quantized_26_08_21`. The private modelassets manifest pins eight CDN files:
`adapter.ort`, `cross_kv.ort`, `decoder_kv.ort`, `encoder.ort`,
`frontend.model.ort`, `frontend.weights.ort`, `streaming_config.json`, and
`tokenizer.bin`. Each URL, exact size and SHA-256 is pinned in the catalog.
The optional attention decoder is omitted because word timestamps are disabled.
Upstream publishes dated directories as immutable; an unexpected byte change
fails verification rather than accepting an updated model.

Moonshine's code and this streaming English model are MIT, copyright 2025
Useful Sensors, Inc. (dba Moonshine AI); see
[the retained MIT notice](../internal/speech/moonshine/LICENSE-MOONSHINE) and
https://github.com/moonshine-ai/moonshine/blob/v0.1.5/LICENSE.
ONNX Runtime is MIT; other upstream third-party components retain their own
licenses, including Eigen's MPL-2.0 subset. No native binaries or weights are
redistributed in this repository. The separate speech fixture is CC BY 4.0;
see [its attribution and checksums](../internal/speech/moonshine/testdata/README.md).

## Local Linux x86-64 setup

Native code requires Linux amd64, cgo, a C compiler, the `moonshine` build tag,
the official header, `libmoonshine.so`, and the bundled `libonnxruntime.so.1`.
The tag is deliberate: missing tagged dependencies can fail linking or dynamic
loading before Go starts. Other platforms and untagged builds use the portable
unavailable-runtime implementation. This does not implement iOS.

Download and unpack only into a local directory outside Git. Do not use upstream's
helper installer, which downloads additional models. Do not install globally or
run `ldconfig`. For example:

```sh
mkdir -p /tmp/thoughts-moonshine-runtime
curl -fL https://github.com/moonshine-ai/moonshine/releases/download/v0.1.5/moonshine-voice-linux-x86_64.tar.gz \
  -o /tmp/thoughts-moonshine-runtime/runtime.tar.gz
printf '%s  %s\n' \
  9c3a87fea93ff2ad957938868f95a0a366dce9ff8ad86bde6cdcf5a4cadb51df \
  /tmp/thoughts-moonshine-runtime/runtime.tar.gz | sha256sum --check
# Extract only after that checksum succeeds.
tar -xzf /tmp/thoughts-moonshine-runtime/runtime.tar.gz -C /tmp/thoughts-moonshine-runtime

export THOUGHTS_MOONSHINE_RUNTIME=/tmp/thoughts-moonshine-runtime/moonshine-voice-linux-x86_64
export THOUGHTS_MODELS_DIR=/tmp/thoughts-moonshine-models
export CGO_CFLAGS="-I${THOUGHTS_MOONSHINE_RUNTIME}/include"
export CGO_LDFLAGS="-L${THOUGHTS_MOONSHINE_RUNTIME}/lib"
export LD_LIBRARY_PATH="${THOUGHTS_MOONSHINE_RUNTIME}/lib"
```

`THOUGHTS_MOONSHINE_RUNTIME` names the unpacked bundle for these explicit build
settings; Go does not read it to download or dynamically locate a runtime.
Preserve any additional flags your local build needs. Keep these exports scoped
to your development shell, rather than changing the application's environment.

## Installation, inference and timing

Installation is an explicit network operation through modelassets. Inference
never installs assets and fails if they are missing, incomplete or corrupt.

```sh
go run ./cmd/moonshine-smoke install -models-dir "$THOUGHTS_MODELS_DIR"

go test -tags=moonshine,moonshine_integration -count=1 -v \
  ./internal/speech/moonshine -run TestMoonshineNative_PCM

go run -tags=moonshine ./cmd/moonshine-smoke transcribe \
  -models-dir "$THOUGHTS_MODELS_DIR" \
  -pcm internal/speech/moonshine/testdata/1272-128104-0000.f32le \
  -sample-rate 16000
```

The developer command prints the requested transcript to stdout and numerical
measurements to stderr. It writes no audio or transcript files. PCM input is
mono, float32 little-endian; the adapter accepts an in-memory slice and explicit
positive sample rate. Samples must be finite and in `[-1, 1]`. The signed 32-bit
native sample-rate and batch-length limits are enforced. Moonshine handles its
internal 16 kHz conversion; Thoughts adds no resampler or WAV parser.

`verified_model_open_seconds` times full model verification, mapping and native
loading. It is not a measurement of native loading alone. Audio duration is
sample count divided by sample rate. `real_time_factor` is transcription wall
time divided by audio duration. Build the command before collecting repeated
measurements, run without competing tests/builds, and record filesystem-cache
conditions. Go heap statistics do not establish native memory usage. These
numbers impose no CI performance threshold and do not measure first partial
transcript latency or live-microphone behavior.

## Ownership, privacy and limits

`moonshine.Open(ctx, root)` always selects and fully verifies the approved model.
Keep the caller-owned root open until Open returns and leave its installed files
unchanged until Close. Read-only mappings opened through `os.Root` feed the
current named-memory-file loader: native code receives no model-directory path,
missing-buffer fallback or automatic downloader. The native transcriber owns
references to those buffers, so Close frees the handle before unmapping them.
Call Close explicitly; there are no finalizers. Calls are serialized across
adapter instances because upstream has process-wide diagnostics and partially
protected registry state. Do not call the shared C library directly alongside
this adapter. A transcriber value must not be copied.

Caller PCM must remain unchanged until Transcribe returns. Native inference is
synchronous and cannot be interrupted. Context checks prevent entry when already
canceled and discard results when canceled before returning; control may return
only after native inference finishes. There are no detached inference goroutines.
Close waits for native work and attempts all cleanup; repeated calls return the
original close outcome without freeing twice. Transcript text is copied into
Go-owned memory before native result invalidation. No-speech results are empty.

The adapter explicitly disables returned audio, API-call logging, ORT-run logging,
transcript logging, debug WAV output, speaker identification and word timestamps.
Only CPU execution is selected. Upstream can still emit unconditional native
failure diagnostics and ONNX warnings; the adapter does not claim to suppress
those. It never forwards native error strings to Go callers, logs authored
content, writes audio, persists text or returns native audio buffers.
Public error strings expose fixed categories; `errors.Is` retains lifecycle and
installation causes, and `errors.As` can retrieve a numeric `NativeStatusError`.
Unwrapped causes may contain paths and must not be logged.

Deferred: Tiny/Whisper and other models, automatic selection/hardware detection,
iOS/Android and other native platforms, browser/microphone transport, AudioWorklet,
PCM WebSockets, application/runtime wiring, resampling/VAD pipelines, partial UI,
draft mutation and session fencing, SQLite speech state, recommendations,
embeddings/LLMs/Python, automatic updates, and native runtime bundling/installers.
