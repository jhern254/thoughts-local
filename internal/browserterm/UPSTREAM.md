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
never restores drafts or replays unsaved input. Tracking supports ordinary
`tea.Cmd` and `tea.BatchMsg`; `tea.Sequence`, detached goroutines, or other
scheduling behavior requires reviewing session cleanup again.

Completing a network write does not mean xterm has rendered its queued output.
The current text MVP bounds socket writes without acknowledging browser rendering.
High-frequency effects or image/video-like output needs a dedicated renderer
flow-control review. The stalled-output regression gates an accepted TCP socket's
write after a real WebSocket session starts, exercising the actual write deadline
and cleanup without relying on OS buffer sizes or a large output payload. It
does not measure xterm render-queue pressure.

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

Set `THOUGHTS_BROWSER_URL` for a different port. These development tests use
synthetic clipboard contents. Inspect normal/narrow screenshots as well as test
results. Run `make quick` before commits and `make ci` for full Go verification
(includes `make check`); run `npm audit` and `govulncheck ./...` for dependencies.
Interactive platform evidence belongs in the PR; cross-compilation alone does
not establish Windows/macOS support or Safari/iPhone acceptance.

## Future voice/media boundary

A later recording control belongs in this page's HTML/client script and sends
recorded bytes to an explicit native HTTP upload handler. That work must define
session/draft ownership, limits, and the existing origin checks for the endpoint.
Native transcription should return a preview; explicit insertion can then use a
validated paste/draft operation. Audio bytes and transcripts must not be encoded
as terminal keystrokes or submitted by synthesizing Enter. Browser recording uses
the browser device's microphone; no offline claim is made for browser speech APIs.
Browser playback can supplement the terminal later. No speculative endpoints or
media interfaces are included here.
