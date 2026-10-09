# Platform support

This is the authoritative status of Thoughts on each platform. **Verified**
means actual runtime execution; **Build-only** means compiled without runtime
proof; **Upstream-supported** means Moonshine supports it but Thoughts has not
verified it; **Planned** means not implemented. STT requires explicit native
setup and model installation; ordinary builds contain no native runtime.

| Platform | Core app | Browser TUI | Moonshine STT | Native STT integration test | Notes |
|---|---|---|---|---|---|
| Linux amd64 | Verified | Verified | Verified | Verified locally and in CI | ThinkPad T480s and [Linux runner](https://github.com/jhern254/thoughts-local/pull/69/checks). |
| macOS arm64 | Build-only | Build-only | Verified | Verified in CI | [Apple Silicon execution](https://github.com/jhern254/thoughts-local/pull/69/checks). |
| macOS amd64 | Build-only | Build-only | Verified | Verified in CI | [Intel execution](https://github.com/jhern254/thoughts-local/pull/69/checks). |
| Windows amd64 | Build-only | Build-only | Verified | Verified in CI | [Windows execution](https://github.com/jhern254/thoughts-local/pull/69/checks); private ONNX DLL prevents system-runtime collisions. |
| iOS arm64 | Planned | Planned | Planned | Not implemented | Desktop cgo support does not imply iOS support. |

CI evidence links follow this PR's current head; inspect the matching
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
