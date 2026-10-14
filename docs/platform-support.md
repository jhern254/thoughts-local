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

## Browser video background processor

Video import requires user-installed FFmpeg/ffprobe with `libx264`, tested on
Linux amd64 with FFmpeg 6.1.1. They are not bundled, auto-installed, or required
by ordinary app builds/image backgrounds. Keep the tools patched using the
platform's maintained distribution. Processing uses Go subprocesses without cgo;
this does not establish an OS sandbox. Chromium and Firefox Linux playback were
executed with synthetic SDR clips; codec support does not prove hardware decoding.
macOS/Windows video import/playback remain unverified. See the
[background measurements](browser-background-performance.md) for CPU limits and
missed targets. Native speech statuses above are unchanged.
