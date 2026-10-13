# Local Moonshine speech

Moonshine provides local speech recognition for browser thought drafts and a
separate developer PCM smoke command. Both use the same verified model and
native boundary. Ordinary builds and tests require no native runtime; explicitly
configuring speech in those builds fails safely instead of capturing audio.

## Browser speech

First complete the platform's native setup below and explicitly install the
approved model. No browser action downloads models or runtime libraries.
Then build with the native tag in the same configured shell:

```sh
go run ./cmd/moonshine-smoke install -models-dir "$THOUGHTS_MODELS_DIR"
GOFLAGS=-tags=moonshine make tui/browser
# Or supply the model root directly:
go run -tags=moonshine ./cmd/thoughts-tui --browser --models-dir "$THOUGHTS_MODELS_DIR"
```

On Windows, set `$env:GOFLAGS = '-tags=moonshine'` before running `make tui/browser`,
or use `go run -tags=moonshine ./cmd/thoughts-tui --browser --models-dir $env:THOUGHTS_MODELS_DIR`.
The database must already have the normal application migrations when using
`go run` directly.

On the Events timeline, F8 opens a draft. Press F8 again to record, then F8 to
stop. Text remains unsaved until the normal Thought save. Without a model root
configured, typed drafts remain available and recording shows a fixed
unavailable message. An explicitly configured but invalid model/runtime prevents
browser startup with safe feedback. `--models-dir` overrides `THOUGHTS_MODELS_DIR`.
Terminal-only mode does not initialize speech.

The browser server keeps one verified transcriber loaded, then creates and
closes one stream per recording. `StartStream` permits one active stream on that
transcriber. Each `AddAudio` supplies a short chunk and returns cumulative text;
unchanged text is not republished. PCM16 browser samples become float amplitude
samples in the adapter, without another resampler or retaining previous chunks.

Stop revokes draft authority before cleanup. `Stream.Close` calls the native
stop/free functions but does not request a final transcript. Pending words may
therefore be discarded on Stop; this is deliberately not a Finish operation.
Stream copies share cleanup state, and parent Close frees an active stream
before releasing its model. The browser server joins all recording work before
closing that parent.

The pinned API internally limits how often it analyzes new audio. Thoughts asks
for updated text after each 100 ms chunk without forcing extra analysis or
adding another scheduler. Cancellation discards results but must wait for a
native call already in progress.

## PCM and the input format

**PCM (pulse-code modulation)** represents an audio waveform as a sequence of
numbers called samples. Each sample records the signal's amplitude at one point
in time. The speech adapter accepts these numbers directly as `[]float32`; it
does not accept a WAV file, MP3 file or microphone device.

- **Mono** means one audio channel: each successive value is the next sample in
  time. Stereo has two channels and is not this input format.
- **Sample rate** is the number of samples per second, measured in hertz (Hz).
  At 16,000 Hz, 16,000 mono samples represent one second. Pass the recording's
  actual rate; changing the rate argument does not convert the samples.
- **Float32** is a 32-bit floating-point number. Samples use normalized amplitude
  in `[-1, 1]`, with `0` representing zero signal amplitude. Negative amplitudes
  are normal. NaN (not a number), infinity and values outside that range are
  rejected, rather than silently changed.

### Why the smoke file says little-endian

The developer command reads a raw `.f32le` fixture: **f**loat, **32** bits,
**l**ittle-**e**ndian. It has no header, channel information or sample-rate
metadata. Every four bytes encode one IEEE 754 float32 sample, in time order;
`-sample-rate` supplies the rate separately. A WAV container can hold PCM, but
its header makes it a different file format; do not pass WAV bytes to this
command. The same applies to signed 16-bit PCM, stereo or compressed audio.

**Endianness means byte order.** A 32-bit value occupies four bytes. Little-endian
stores the least significant byte first; big-endian stores it last. For example,
the floating-point sample `1.0` has the bit pattern `0x3f800000`:

| Encoding | Four bytes in the file (hexadecimal) |
|---|---|
| Little-endian, accepted by this command | `00 00 80 3f` |
| Big-endian, a different encoding | `3f 80 00 00` |

`decodePCM` first reads four bytes in the specified order using
`binary.LittleEndian.Uint32`, then `math.Float32frombits` interprets those bits
as a floating-point sample. This is not a numerical integer-to-float conversion:
converting the integer `0x3f800000` numerically would produce a large number,
rather than the sample `1.0`. The decoder rejects empty files and partial
four-byte samples; the speech adapter checks amplitude and finiteness.

Byte order matters at this file boundary. Callers already holding `[]float32`
pass those values directly to `Transcribe`; they do not serialize them or choose
an endian format. Keep the slice unchanged until the call returns.

The committed fixture contains 93,680 mono samples at 16,000 Hz:
`93,680 / 16,000 = 5.855` seconds and `93,680 × 4 = 374,720` bytes. See
[the fixture notes](../internal/speech/moonshine/testdata/README.md) for the
licensed source, conversion and checksums. This decoder is developer-tool
plumbing, not a browser audio protocol or a production audio-file importer.

## Architecture and where ownership changes

```mermaid
flowchart TD
    A[Approved model ID] --> B[modelassets.Install: explicit network operation]
    B --> C[Verified files and completion marker]
    C --> D[moonshine.Open: verifies every required file again]
    D --> E[Confined read-only model mappings]
    E --> F[Native Moonshine handle]
    G[Caller-owned mono float32 samples and sample rate] --> F
    F --> H[Native transcript lines copied into Go strings]
    H --> I[Go-owned Transcript.Text]
```

Installation and inference are separate so missing or changed assets cannot
cause inference to make a hidden network request. `Open` hashes all required
files through modelassets, so opening a model is deliberately more expensive
than checking whether a directory exists. URLs, hashes and model-file sizes
come only from the application-owned catalog.

| Code | Responsibility and lifetime |
|---|---|
| `internal/modelassets` | Downloads only the approved manifest; checks size/hash and commits installation through its completion marker. |
| `moonshine/transcriber.go` | Portable input validation, context checks, shared lifecycle state and fixed safe errors. Copies of a transcriber share this state. |
| `moonshine/model_files.go` and OS mapping helpers | Opens verified files through `os.Root` and maps their bytes read-only. A mapping makes file contents accessible in memory without copying the whole file into a Go slice allocation. |
| `moonshine/native_desktop.go` | Uses cgo (Go's bridge to C) to create/use/free the native handle. Model mappings must stay alive because native sessions retain pointers to them. Native transcript pointers never cross this boundary; returned lines are copied into Go strings. |
| `moonshineRuntimeGate` | Serializes all adapter instances, including loading, inference and cleanup, because Moonshine has process-wide state. Waiting for the gate is cancellable. |
| `Transcriber.Close` | Frees the native handle before unmapping its model files. This order prevents native code from accessing released memory. Repeated closes, including closes on copies, return the stored outcome. |
| `cmd/moonshine-smoke` | Developer-only file decoding, explicit installation and timing/output. The adapter neither reads audio files nor persists audio/transcripts. |

An entered native call is synchronous and cannot be interrupted through Go's
context. Cancellation prevents entry or discards its result after it finishes;
it does not let Close free resources while inference still uses them. Returned
Go strings remain valid across later inference and Close.

The `moonshine && cgo` build constraint, restricted to Linux amd64, macOS
amd64/arm64, and Windows amd64, isolates the native implementation. Go also sets the `darwin` tag for iOS, so the desktop
constraint explicitly excludes `ios`. **amd64** means the x86-64 CPU architecture; it is unrelated to
Moonshine's **model architecture**, which identifies a neural model layout such
as Small Streaming. Ordinary builds select the unavailable-runtime implementation
and need no C headers or libraries. The portable package owns lifecycle policy;
the tagged file owns C memory and calls. This split keeps platform mapping/linking separate from the shared C API
implementation. Browser startup owns optional speech separately from the SQLite application runtime. See
[platform support](platform-support.md) for actual runtime evidence.

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
loading before Go starts. Unsupported targets and untagged builds use the portable
unavailable-runtime implementation. Desktop support does not implement iOS.

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

## Local macOS setup (Apple Silicon and Intel)

Use the **desktop static library** from the official `moonshine-swift` v0.1.5
`Moonshine.xcframework.zip`. It contains complete arm64 and x86-64 C API code and
ONNX Runtime in `macos-arm64_x86_64/libmoonshine.a`, with C header version 30000.
This uses no Swift code and extracts no iOS library. The version and model pin
remain unchanged. Use Xcode Command Line Tools, cgo, and the `moonshine` tag;
the boundary links libc++, CoreFoundation, and Foundation. No separate ONNX
dylib or global installation is needed.

The main repository's `moonshine-voice-macos-arm64.tar.gz` appears universal,
but its Intel slice lacks Moonshine C API definitions. Actual Intel CI linking
caught this; architecture metadata alone does not prove a usable runtime. The
[official v0.1.5 Swift package manifest](https://github.com/moonshine-ai/moonshine-swift/blob/v0.1.5/Package.swift)
pins the complete XCFramework and its SHA-256. Only its desktop C library is used.

```sh
mkdir -p /tmp/thoughts-moonshine-runtime
curl -fL https://github.com/moonshine-ai/moonshine-swift/releases/download/v0.1.5/Moonshine.xcframework.zip \
  -o /tmp/thoughts-moonshine-runtime/Moonshine.xcframework.zip
printf '%s  %s\n' \
  6bc7fb4b6d3a470a2ae2d681299975f3ba9d710753786d1cd7e8beabaad066e8 \
  /tmp/thoughts-moonshine-runtime/Moonshine.xcframework.zip | shasum -a 256 --check
# Extract only after that checksum succeeds, selecting the desktop slice.
unzip -q /tmp/thoughts-moonshine-runtime/Moonshine.xcframework.zip \
  'Moonshine.xcframework/macos-arm64_x86_64/*' -d /tmp/thoughts-moonshine-runtime

export THOUGHTS_MOONSHINE_RUNTIME=/tmp/thoughts-moonshine-runtime/Moonshine.xcframework/macos-arm64_x86_64
export THOUGHTS_MODELS_DIR=/tmp/thoughts-moonshine-models
export CGO_CFLAGS="-I${THOUGHTS_MOONSHINE_RUNTIME}/Headers"
export CGO_LDFLAGS="-L${THOUGHTS_MOONSHINE_RUNTIME}"
lipo -archs "$THOUGHTS_MOONSHINE_RUNTIME/libmoonshine.a"
```

## Local Windows x86-64 setup

The official Windows bundle contains MSVC **static** libraries: `moonshine.lib`,
`bin-tokenizer.lib`, `ort-utils.lib`, and `moonshine-utils.lib`. Its
`onnxruntime.lib` is an import library for `onnxruntime.dll`. There is no upstream
Moonshine DLL in this archive. MinGW cannot directly consume its C++ objects as
if they were the Linux shared library: they depend on the MSVC C++ ABI and CRT.

Use an x64 Visual Studio developer PowerShell with the C++ toolchain installed,
plus x86-64 MinGW GCC and GNU `dlltool` on PATH. The explicit setup helper verifies
the pinned archive **before extraction**, links its libraries into a local
`thoughts-moonshine.dll` with MSVC `/MD`, and creates a MinGW import library.
It also copies the bundled ONNX DLL under `thoughts-moonshine-onnxruntime.dll`
and regenerates its import library. Windows searches System32 before PATH,
so an ordinary `onnxruntime.dll` name could select an incompatible system runtime.
See [Windows DLL search order](https://learn.microsoft.com/en-us/windows/win32/dlls/dynamic-link-library-search-order).
The helper checks that the bridge imports only the private ONNX name.
This bridge exports the existing C API, not another inference implementation.
The DLL and ONNX Runtime require the Microsoft Visual C++ x64 runtime. Use a
machine where that prerequisite is already available; the helper performs no
global installation. All generated files stay outside Git.

```powershell
$runtimeRoot = Join-Path $env:TEMP 'thoughts-moonshine-runtime'
New-Item -ItemType Directory -Force $runtimeRoot | Out-Null
$runtimeArchivePath = Join-Path $runtimeRoot 'windows.tar.gz'
Invoke-WebRequest 'https://github.com/moonshine-ai/moonshine/releases/download/v0.1.5/moonshine-voice-windows-x86_64.tar.gz' -OutFile $runtimeArchivePath
./scripts/setup-moonshine-windows.ps1 -RuntimeArchivePath $runtimeArchivePath -OutputDirectory $runtimeRoot

$env:THOUGHTS_MOONSHINE_RUNTIME = (Join-Path $runtimeRoot 'moonshine-voice-windows-x86_64').Replace('\', '/')
$env:THOUGHTS_MODELS_DIR = Join-Path $env:TEMP 'thoughts-moonshine-models'
$env:CC = (Get-Command gcc.exe).Source
$env:CGO_ENABLED = '1'
$env:CGO_CFLAGS = "-I$env:THOUGHTS_MOONSHINE_RUNTIME/include"
$env:CGO_LDFLAGS = "-L$env:THOUGHTS_MOONSHINE_RUNTIME/bridge"
$env:PATH = "$env:THOUGHTS_MOONSHINE_RUNTIME/bridge;$env:PATH"

go run ./cmd/moonshine-smoke install -models-dir $env:THOUGHTS_MODELS_DIR
go test '-tags=moonshine,moonshine_integration' -race -count=1 -v ./internal/speech/moonshine
```

Keep the bridge DLL and its copied `thoughts-moonshine-onnxruntime.dll` together on this shell's
PATH. Loading failures can occur before Go starts; a missing DLL is not always
representable as a Go `speech.ErrRuntime`. Paths containing spaces need quoted
include/library directory values inside `CGO_CFLAGS` and `CGO_LDFLAGS`.

Windows model storage uses read-only `CreateFileMapping`/`MapViewOfFile`, not Go
heap allocations. Each mapped file owns its view and mapping handle. Closing the
native transcriber precedes unmapping views and closing those handles. Unix uses
read-only `mmap`/`munmap` with the same lifetime contract.

The [pinned upstream build configuration](https://github.com/moonshine-ai/moonshine/blob/v0.1.5/core/CMakeLists.txt)
and [Windows example link settings](https://github.com/moonshine-ai/moonshine/blob/v0.1.5/examples/windows/cli-transcriber/cli-transcriber.vcxproj)
explain these library/framework requirements. The downloaded archives themselves
are authoritative for library form and architecture.

## Installation, inference and timing

Installation is an explicit network operation through modelassets. Inference
never installs assets and fails if they are missing, incomplete or corrupt.

```sh
go run ./cmd/moonshine-smoke install -models-dir "$THOUGHTS_MODELS_DIR"

go test '-tags=moonshine,moonshine_integration' -count=1 -v \
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
this adapter. Transcriber copies share one lifecycle state: closing any copy
closes them all and releases the native resources exactly once.

Caller PCM must remain unchanged until Transcribe returns. Native inference is
synchronous and cannot be interrupted. Context checks prevent entry when already
canceled and discard results when canceled before returning; control may return
only after native inference finishes. There are no detached inference goroutines.
Close waits for native work and attempts all cleanup; repeated calls return the
original close outcome without freeing twice. Transcript text is copied into
Go-owned strings inside the native adapter before crossing into portable Go
code. No-speech results are empty.

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
iOS/Android, application.Runtime speech ownership, VAD/resampling pipelines,
speakers/timestamps/confidence, Finish/final-drain semantics, automatic saves,
SQLite speech state, recommendations, embeddings/LLMs/Python, automatic updates,
and native runtime bundling/installers.
