# Browser terminal sources and maintenance

Imported/adapted on 2026-10-01 against Thoughts `d8b9bdb` (default branch: master).

## Scope

This is a Thoughts-specific adapter, not a copy of sip's server. The browser runs
xterm.js; Go runs the existing Bubble Tea model and services. The only new Go
module is `github.com/coder/websocket v1.8.14` (Go 1.23 minimum). Thoughts keeps
Go 1.25.14, Bubble Tea 2.0.9, Bubbles 2.2.1, and Lipgloss 2.0.5.

Reference: [sip v0.8.3, 2124762778023a5e466c88649c3fa0f3cd83c47f](https://github.com/Gaurav-Gosain/sip/tree/2124762778023a5e466c88649c3fa0f3cd83c47f).
`session.go:newProgram` adapts `sip.go:MakeOptions` (explicit dimensions, color
profile, terminal environment, suspend handling). Binary message identifiers
`0` input, `1` output, `2` resize, and `7` close follow sip's framing convention.
The MIT notice is embedded in `static/SIP-LICENSE.txt`.

No sip server, session implementation, PTY, middleware, webterm bundle, shell/CLI,
WASM, WebTransport, QUIC, certificate store, mobile controls, font collection,
or kitty/media transcoder was imported. The client uses published xterm artifacts
directly, avoiding an opaque webterm bundle and an unnecessary fork of that code.
There is no compatibility promise with sip's protocol or page extension APIs.

## Local implementation

- `cmd/thoughts-tui/browser.go`: defaults to port 7777 and opens the OS default
  browser using `xdg-open`, macOS `open`, or Windows `rundll32`. Only the generated
  loopback URL is passed; subprocess diagnostics are discarded. Launching is
  bounded and joined on shutdown. `--browser-open=false` suppresses launching;
  a failed launcher leaves the server available at the printed address.
- `server.go`: exact loopback authority/origin checks, embedded files, WebSocket
  admission, bounded frames, one model per session, and explicit cleanup. No
  static directory on disk, forwarded-header trust, origin wildcard, or shell.
- `input.go`: uses Charm's existing `uv.EventDecoder`; it is not a terminal
  parser. Each xterm onData callback is one bounded complete frame. A separate
  `p` frame becomes an unmodified `tea.PasteMsg`, avoiding xterm's LF-to-CR paste
  conversion and the terminal reader's replacement-character removal. Messages
  enter the program in wire order. Host clipboard key bindings are suppressed.
- `session.go`: public Bubble Tea APIs with explicit input messages/output and
  dimensions; no operating-system terminal is involved. Cancels and joins started
  commands before releasing the connection slot or runtime. Queued commands
  cannot start after cancellation. The current TUI/widgets use ordinary commands
  and `tea.BatchMsg`; adding `tea.Sequence` or detached work requires revisiting
  this lifecycle boundary. Model/command panics are contained without printing
  panic values. Bubble Tea's process-wide `TEA_TRACE` option is refused.
- `static/client.js`: fitting, browser clipboard capture, fixed status messages,
  and explicit reconnect. `convertEol` supplies the output translation normally
  performed by a PTY. OSC 52 is consumed without clipboard side effects; links
  do not open automatically. Initial admission retries a busy slot for up to one
  second to allow a reloaded page's previous session to finish closing. Established
  connections do not automatically reconnect. One pending resize retains only the
  latest dimensions and retries every 50ms while backpressured. It is cleared on
  socket change/close; keypresses and pastes are never retained or replayed.

Limits: 1 MiB per whole paste (+ one framing byte), 4 KiB per key event, 512×128
cells, 16 KiB HTTP headers, 10s initial resize, 5s per output write. HTTP shutdown
has a 10s timeout; WebSocket close bounds its write and acknowledgement at 5s
each. Session service calls use cancellation; shutdown joins started commands
instead of closing their database underneath them. There is no idle-session
timeout. Do not add commands that ignore cancellation and block indefinitely.

The HTTP logger emits only a fixed approved failure event. Failed WebSocket
handshakes retain their status but replace dependency error bodies with fixed
text. Application close messages are fixed identifiers; the browser never prints
socket errors or peer close reasons. The WebSocket library's protocol-level close
responses are not forwarded to diagnostics. No terminal traffic is recorded.

## Data path and lifecycle contracts

Browser event → binary WebSocket frame → `inputMessages` → Bubble Tea message →
existing TUI. Bubble Tea output → `terminalWriter` → WebSocket output frame →
xterm.js. Malformed input cancels application work before awaiting peer closure.

The server owns the shared native Runtime; each session owns its model/context.
Started commands must respect cancellation. Shutdown joins tracked work before
the command closes Runtime resources. Reconnect creates a new UI session and
never restores drafts or replays unsaved input. Both launch modes reuse the
command guard in `internal/tui/session.go`. Tracking supports ordinary
`tea.Cmd` and `tea.BatchMsg`; `tea.Sequence`, detached goroutines, or other
scheduling behavior requires reviewing session cleanup again.

Completing a network write does not mean xterm has rendered its queued output.
The current text MVP bounds socket writes without acknowledging browser rendering.
High-frequency effects or image/video-like output needs a dedicated renderer
flow-control review. The stalled-output regression gates an accepted TCP socket's
write after a real WebSocket session starts, exercising the actual write deadline
and cleanup without relying on OS buffer sizes or a large output payload. It
does not measure xterm render-queue pressure.

Native clipboard reads do not expose cancellation; commands check before/after
the call and join any read already running. Pinned Bubbles cursor timers and
synchronous filters are also joined rather than abandoned. These dependency
limits require separate review before promising immediately interruptible work.

## Terminal artifacts

All three packages come from xterm.js commit
`f447274f430fd22513f6adbf9862d19524471c04`:

| Package | Version | Retained artifact |
| --- | --- | --- |
| `@xterm/xterm` | 6.0.0 | terminal JS, CSS |
| `@xterm/addon-fit` | 0.11.0 | fit addon JS |
| `@xterm/addon-unicode-graphemes` | 0.4.0 | Unicode 15 grapheme addon JS |

`client/package-lock.json` pins registry URLs and SHA-512 package integrity.
`ASSETS.sha256` records the generated bytes. Full upstream MIT notices are in
`static/LICENSES.txt`, embedded and available at `/static/LICENSES.txt`.
The bundled VS Code utilities retain Microsoft's MIT notice in
`static/VSCODE-LICENSE.txt`; the terminal's older attribution is also included.

The published bundles are transformed by pinned esbuild 0.28.2 with
`drop: ['console']` to remove diagnostic calls in vendored utilities that bypass
xterm's supported logger. The logger itself also receives only fixed-message
callbacks. Identifiers are **not** minified again; parser/rendering code is not
hand-edited. CSS is copied verbatim. No source maps or external URLs are loaded.

To regenerate with development-only Node/npm:

```sh
cd internal/browserterm/client
npm ci --ignore-scripts
npm run build
cd ../static
sha256sum -c ../ASSETS.sha256
```

For upgrades, review upstream changes/advisories, edit exact package versions,
refresh the lockfile, rebuild, inspect hashes/licenses, and run both Go and browser
regressions. Rebuild the Go executable after changing embedded assets. Copied or
generated assets require this explicit audit; Go vulnerability scanning alone
does not cover them. Internal code ownership is not a security guarantee.

## Browser acceptance

Start `THOUGHTS_BROWSER_OPEN=false make tui/browser/demo` so the automated tests
can own the single browser session, then in `client/`:

```sh
npm ci --ignore-scripts
npx playwright install chromium firefox
npm test
```

For the complete synthetic PCM/fake-transcript path, run from the repository
root after installing the dependencies above. The opt-in Go host owns a
disposable migrated, seeded app and fake consumer:

```sh
node --test internal/browserterm/client/microphone.test.mjs internal/browserterm/client/pcm*.test.mjs
THOUGHTS_BROWSER_AUDIO_TEST=1 go test -tags=integration -count=1 -timeout 12m -run '^TestBrowserAudioWorkflow_Playwright$' ./integration
```

Optional `THOUGHTS_BROWSER_TEST_FILTER` selects Playwright test names;
`THOUGHTS_BROWSER_ARTIFACTS=/tmp/thoughts-browser-review` retains synthetic
screenshots and test artifacts outside the source tree. Ordinary Go checks skip
this browser host. `npm test` against a running normal demo skips cases requiring
the injected fake.

Chromium uses generated tones; Firefox's PCM cases use its synthetic microphone
device. These tests exercise the real worklet/socket path, not physical
microphones or recognition accuracy. Headless Linux needs a PulseAudio-compatible
backend for Firefox's Web Audio clock; CI starts a null output sink without
physical hardware or audio storage. Desktop sessions can use their existing
backend. No macOS/Windows microphone interaction or iOS support is claimed.

Set `THOUGHTS_BROWSER_URL` for a different port. These development tests use
synthetic clipboard contents. Inspect normal/narrow screenshots as well as test
results. Run `make quick` before commits and `make ci` for full Go verification
(includes `make check`); run `npm audit` and `govulncheck ./...` for dependencies.
Interactive platform evidence belongs in the PR; cross-compilation alone does
not establish Windows/macOS support or Safari/iPhone acceptance.

## Microphone capture

The browser now uses `pcm-capture.js` and `pcm-worklet.js` to send bounded,
mono PCM16LE at 16 kHz to the dedicated `/voice/audio` socket. The standard
consumer discards PCM; real recognition is intentionally not connected.
[`docs/browser-audio.md`](../../docs/browser-audio.md) explains how recording fits
the draft, why sound has its own connection, and how Stop prevents late results
from changing text. The exact wire contract lives in
[`audio_protocol.go`](audio_protocol.go).
The older `microphone.js` MediaRecorder discard helper remains separate for its
existing focused tests; the client does not start both capture paths.

`BrowserRecordingEnabledMsg` tells the root TUI that this session uses browser
recording controls. `VoiceState.BrowserRecordingEnabled` reports that capability;
it does not mean microphone permission was granted or a device is available.
`CanOpenThoughtDraft` separately reports whether the current screen permits quick
entry without interrupting a form or filter.

`DraftID` identifies the current draft, and `RecordingID` identifies an attempt
within it. `RecordingStatus` describes its phase. These names are also used in
the JSON frames exchanged with the embedded client.

Recording is controlled by the shared Thoughts editor, using F8 for now:

1. The TUI key changes the draft's recording state through `updateVoiceAction`.
2. `voiceBridgeModel` sends a `VoiceState` snapshot to `client.js`, which starts
   or stops `ThoughtsPCMRecording` on the browser's device.
3. Browser callbacks send control acknowledgements through `inputMessages`
   back to the TUI. Editing and saving stay locked until recording-owned server work joins.

Binary `v` frames carry bounded recording controls/state and the one-use audio capability; never PCM or transcript uploads. `inputMessages`
validates controls; the Thoughts draft checks its draft/recording identities.
`voiceBridgeModel` publishes authoritative state after model updates through the
same bounded socket-write path as terminal output. Native programs do not enable
this capability. The session/draft owns capture; late permission grants release
their tracks, and navigation/disconnect cancels capture without replay.

The subject picker follows Events' explicit create-and-return behavior. Draft
reads have cancellable child contexts; submitted writes remain session-owned.
No audio, authored text, or raw microphone errors enter operational diagnostics.

## Future voice/media boundary

Real Moonshine streaming inference remains separate work. It must consume this
recording-owned queue and publish identity-fenced Go updates, without automatic
model downloads or audio persistence. Browser PCM never belongs in terminal
input. Finish/finalize behavior, VAD, live partial transcript UI, and product
speech wiring are intentionally deferred.

## Visual settings and local backgrounds

The `browser_appearance` table and `.assets/appearance/` directory retain their
original names so existing background selections remain usable.

Visual metadata belongs to SQLite; the concrete visual service owns
immutable image files beside that database. Import validates bounded JPEG/PNG
content before writing. It commits the selection before deleting the old asset;
failure before that commit preserves the working selection. A crash or failed
cleanup may leave an unreferenced file; there is no background cleanup worker.

The browser changes only the image layer, black overlay, and xterm theme
background. `allowTransparency` must be set before `Terminal.open`; the
application CSS also clears the outer viewport's black fallback. Terminal
foreground opacity, input transport, and microphone ownership remain unchanged.
The image layer can later be replaced with a video element without changing
the overlay or terminal layers; no playback infrastructure is implemented.

Only exact visual routes access native images. Mutations require the same
exact local Host and Origin as the terminal boundary; no directory is served.
Imported bytes, filenames, paths, and decoder errors must never enter logs or
public diagnostics.

The root entity strip includes Options. Bubble Tea renders browser Options as
an overlay without replacing the underlying screen or draft. Ctrl+, / Cmd+,
opens the same page during a draft or recording. Its bridge carries bounded
`o` control/result frames and a terminal-cell preview rectangle; image bytes
remain on HTTP. Operation IDs reject stale replies, including across page
close/reopen. The browser also guards results across connection changes.

The browser draws only the image inside that rectangle and retains the device
file picker. Before displaying it, public xterm buffer reads verify that the
reserved border and blank cells have actually rendered; this avoids covering
old text when metadata arrives ahead of terminal output. Cell geometry uses the
pinned xterm screen element and its public row/column counts. No private xterm
APIs or alternate-screen-incompatible marker decorations are used. Mouse mode
is enabled only on Options. Native Options retains its browser-only explanation.

### Background framing

Migration 000013 adds bounded Fill/Fit, zoom, and normalized overflow positions
without rewriting background assets. Bubble Tea owns draft settings and framing
navigation. The browser owns pixel geometry and sends bounded, operation-scoped
drag updates. One geometry calculation paints both the viewport and its scaled
preview, including after resize; thumbnail dimensions do not determine the crop.
