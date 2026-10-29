# Platform support

This is the authoritative status of Thoughts on each platform. **Verified**
means actual runtime execution; **Build-only** means compiled without runtime
proof; **Upstream-supported** means Moonshine supports it but Thoughts has not
verified it; **Planned** means not implemented. Prebuilt desktop bundles include
the speech engine; users only install the model through onboarding.
`make tui/browser` automatically prepares a missing engine for source builds.
`make tui/package` builds the app and engine together; the existing native matrix
checks these bundles after relocation on all four desktop targets. Browser model onboarding stores
weights in the visible per-user `Documents/Thoughts/models` folder, shared across databases;
normal builds without the engine explain its absence and offer no download.
Ordinary Go builds contain no
native runtime. The developer setup helper is checked by the existing four-target
native workflow; interactive microphone statuses below remain unchanged.

| Platform | Core app | Browser TUI | Moonshine STT | Native STT integration test | Notes |
|---|---|---|---|---|---|
| Linux amd64 | Verified | Verified | Verified | Verified locally and in CI | ThinkPad T480s and [Linux runner](https://github.com/jhern254/thoughts-local/pull/69/checks). |
| macOS arm64 | Build-only | Build-only | Verified | Verified in CI | [Apple Silicon execution](https://github.com/jhern254/thoughts-local/pull/69/checks). |
| macOS amd64 | Build-only | Build-only | Verified | Verified in CI | [Intel execution](https://github.com/jhern254/thoughts-local/pull/69/checks). |
| Windows amd64 | Build-only | Build-only | Verified | Verified in CI | [Windows execution](https://github.com/jhern254/thoughts-local/pull/69/checks); private ONNX DLL prevents system-runtime collisions. |
| iOS arm64 | Planned | Planned | Planned | Not implemented | Desktop cgo support does not imply iOS support. |

The table links record desktop batch-inference verification. The
[browser streaming checks](https://github.com/jhern254/thoughts-local/actions/workflows/modelassets.yml?query=branch%3Afeature%2Fbrowser-moonshine)
exercise streaming on those same four targets. Inspect the matching
`Native transcription (<GOOS>/<GOARCH>)` job for each platform.

Core app and Browser TUI statuses describe running/hosting the Go application,
not accessing a Linux-hosted browser TUI from another device. Desktop builds
alone do not prove interactive browser or terminal behavior.

All desktop STT targets keep Moonshine Voice **v0.1.5**, C API **30000**, and
Small Streaming English **quantized_26_08_21**. macOS uses the desktop universal static
library from the official v0.1.5 XCFramework; Windows uses a locally generated DLL from the verified MSVC libraries;
Linux uses the official shared libraries. See [native setup](moonshine.md).

All four native jobs executed the licensed speech phrase checks, silence, copied
transcript lifetime, resource cleanup, and unexpected-file checks under the race
detector. Symlink confinement tests passed on all four runners, including Windows.
Portable jobs build the complete application without cgo; macOS/Windows core app
and Browser TUI have not been interactively verified.

The speech workflow records GOOS, GOARCH, actual runner architecture, archive
identity, and successful native transcription. Update this table only from
those results; cross-compilation does not establish runtime support.

Browser recordings now use one native stream per recording when explicitly
configured. Linux streaming was executed locally with the licensed fixture in
100 ms chunks, including a Chromium worklet-to-draft walkthrough with the real
recognizer. Interactive macOS/Windows microphones remain unverified; the
Browser TUI statuses above are unchanged. iOS remains outside this desktop work.

Explicit model repair uses shared/exclusive installation-use locks on the
existing desktop targets: `flock` on Linux/macOS and `LockFileEx` on Windows.
Loaded transcribers hold shared access until native resources close; repair
requires exclusive access and the existing installation mutation lock.
The portable package jobs exercise cross-process contention and process-exit
release; the native jobs check that a loaded model blocks repair. See the
[repair desktop checks](https://github.com/jhern254/thoughts-local/actions/workflows/modelassets.yml?query=branch%3Afeature%2Fmodel-repair)
for execution results on each target.
This does not add platform or microphone support.

Browser startup recognizes Unix `EADDRINUSE` and Windows `WSAEADDRINUSE`
(10048) when reporting an occupied port. Bind failures retain the original
error through successful cleanup; public messages contain the configured port
and recovery guidance rather than raw OS errors. A real occupied-port launch
was exercised on Linux amd64. Windows error-code coverage uses controlled
fixtures; it does not change the platform verification statuses above.

## Terminal renderer resize correction

The pure-Go Ultraviolet dependency is pinned to `d38ea0f8aa5c` for its resize-cache
correction; its existing MIT license and no-cgo boundary are unchanged. The module
requires `golang.org/x/sync` v0.22.0. Linux amd64 Chromium/Firefox tests exercise
native renderer output, repeated terminal resizes and ordinary editing. Other
platform statuses above are unchanged; hosted portable/runtime checks provide
separate evidence and do not establish interactive browser verification there.
See [browser maintenance](../internal/browserterm/UPSTREAM.md#native-renderer-resize-cache).

## Browser video background processor

Video import requires user-installed FFmpeg/ffprobe with `libx264`, tested on
Linux amd64 with FFmpeg 6.1.1. New-upload trimming, including late cuts and
fractional frame rates, is verified on that processor with Chromium/Firefox
Linux previews. Native source-relative trimming also tests nonzero source
timestamps; source previews now fall back to bounded native stills where browser
seeking differs. The tools are not bundled, auto-installed, or required
by ordinary app builds/image backgrounds. Keep the tools patched using the
platform's maintained distribution. Processing uses Go subprocesses without cgo;
this does not establish an OS sandbox. Chromium and Firefox Linux playback were
executed with synthetic SDR clips; codec support does not prove hardware decoding.
macOS/Windows video import/playback remain unverified. See the
[background measurements](browser-background-performance.md) for CPU limits and
missed targets. Native speech statuses above are unchanged.

The quality extension has runtime coverage on Linux amd64 / FFmpeg 6.1.1 for
Efficient (720p30), High quality (1080p30), and Maximum (1080p60), including
portrait output, 23.976/29.97/59.94 FPS and padded-macroblock level selection.
Chromium and Firefox integration tests decode and persist actual 1080p60
prepared streams. These Linux results do not upgrade other platforms or imply
that every browser/device supports Constrained Baseline Level 4.2 playback.

Long-source imports (up to four hours / 8 GiB) retain these platform statuses.
Linux amd64 exercised small synthetic two- and four-hour sources in Chromium/Firefox and
a real 2.64 GiB source in Brave on the T480s. The exact 8 GiB boundary uses
controlled streaming readers rather than a huge routine test fixture. Windows
video-write feedback recognizes Win32 disk-full codes 112 and 39; macOS/Windows
large-file hosting, cleanup permissions and browser runtime behavior remain
unverified. Build-only checks do not establish large-source runtime support.

Matroska import/source-frame fallback has Linux amd64 runtime coverage on the
T480s with FFmpeg 6.1.1: H.264 8-bit, HEVC Main 8-bit/Main10 SDR, VP9 8-bit
and AV1 8-bit (libdav1d decoder). Chromium and Firefox exercise scene browsing,
forced native fallback, rapid scrubbing and exact prepared-asset Apply. Main10
requires explicit Rec.709 signaling; HDR and VP9/AV1 10-bit remain unsupported.
These results do not verify macOS/Windows processors or promise that a browser
can decode the original container. Output quality/platform statuses above remain
unchanged; updated measurements are in the background performance guide.

### Phrase timing for editor newlines

The Moonshine v0.1.5 adapter now copies phrase identity, start/duration, and
completion fields from the same pinned C API. It retains the existing native
runtime gate and copies no audio data. Linux amd64 has runtime evidence from the
licensed speech → four-second silence → speech integration fixture, including
one generated newline and no trailing break. This does not establish new runtime
verification for macOS or Windows; their existing support classifications remain
unchanged. Word timestamps and speaker identification remain disabled.
