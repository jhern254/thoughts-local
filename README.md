# Building AI Chat app in Go Lang, Python, SQLite 

## Thoughts TUI

Build with `make tui/build`. Apply migrations explicitly with `make migrate/up`
before using a new database. The executable never applies migrations at startup.

```sh
./bin/thoughts-tui                       # native terminal (default)
./bin/thoughts-tui --browser             # http://127.0.0.1:7777
./bin/thoughts-tui --browser --browser-port 8123
```

Both modes accept `--db-dsn` / `THOUGHTS_DB_DSN`. Browser mode accepts
`THOUGHTS_BROWSER_PORT`; the flag takes precedence. Browser mode automatically
opens your operating system's default browser (including Brave when configured
as the default). The address is also printed for manual opening if needed.
Use `--browser-open=false` or `THOUGHTS_BROWSER_OPEN=false` to serve without
opening a tab.
`make tui/browser` migrates and launches the persistent database.

For disposable browser demos:

| Command | Synthetic data |
| --- | --- |
| `make tui/browser/demo/seeded` | 2 subjects, 3 events, 12 thoughts |
| `make tui/browser/demo/stress` | 4 subjects, 721 events, 20,000 thoughts |

Both build the same executable, migrate a fresh temporary database, and open on
Events. Your default browser opens automatically; stop the server with Ctrl+C to
delete the temporary database. Each launch starts fresh and leaves your usual database
untouched. `make tui/browser/demo` remains an alias for the seeded version.
Set `THOUGHTS_BROWSER_PORT=8123` if the default port is occupied.

The stress fixture spans about a month, includes empty historical events, and
puts 200 thoughts in the current event. Expand events, scroll the thought picker,
browse Subjects and Misc thoughts, and resize the browser to exercise the larger
dataset. This is an interactive stress demo, with no fixed performance promise.

The application, SQLite, and filesystem remain native. The Go executable embeds
the browser terminal assets; Node/npm, a CDN, WASM, and a separate frontend server
are not needed to build or run it.

### Browser behavior

- One active tab. A second tab reports that the first is busy and offers Reconnect.
  Opening/reloading retries admission for up to one second while an old session
  finishes closing.
- Quit (`q` where available, or Ctrl+C without a browser selection) ends that
  session. Ctrl+C in the launching terminal stops the server.
- Reload/disconnect discards unsaved UI state. Reconnect starts on Events and
  reloads saved data; input is never replayed and nothing is automatically saved.
- Use the browser's normal paste command (Ctrl+V, Cmd+V, Shift+Insert, or its
  context menu). It reads the browser device's clipboard. Each paste is limited
  to 1 MiB; the existing editor validates the complete text before insertion.
  Unsupported tabs, CR/CRLF, controls, and U+FFFD retain the existing rejection
  behavior in Thoughts/Events. Browser text selection uses the normal copy path.
- Application shortcuts work while the terminal has focus. Browser-reserved
  shortcuts such as Ctrl+L and Ctrl+W remain browser actions. Click the terminal
  to return focus. Attachments and mobile packaging are deferred.

### Voice input

In browser mode, press **t** from browsing to open the regular Thoughts editor,
with the same blue Thoughts heading used by normal creation. **F8** toggles
**Record / Stop**; the TUI displays only the current action. The browser requests
microphone permission when recording starts. Each recording can last twenty
minutes; Stop restores editing, and recording again keeps the draft. Editing,
subject changes, and saving are disabled during recording.

While stopped, use **Tab** to switch between the text and optional subject search.
The first choice is **Create subject…**, followed by matching subjects, as in
Events. An empty field means **Misc**; clear it to return to Misc. Creating or
cancelling returns to the preserved draft. **Ctrl+S** saves and **Esc** cancels.
Plain **v** remains text; Vim-style normal/insert modes are deferred.

This first stage captures microphone audio without transcription. Audio chunks
are discarded immediately; there is no playback, audio upload, or recording
storage. Type or paste draft text while stopped. Reloading or disconnecting ends
capture and discards unsaved state. Live native transcription follows separately.

### Local trust and maintenance

Browser mode binds only `127.0.0.1`, validates the exact Host and Origin, and
serves only embedded files. Use the printed numeric address, not a hostname or
reverse proxy. This is a local single-user application: processes running as
your user can access it. There is no LAN mode, authentication, or shell endpoint.
Disable `TEA_TRACE` before launching browser mode; terminal traffic recording is
refused. Logs use the same fixed-message policy as native mode.

The adapter uses xterm.js and selected ideas/options from sip. See
[the upstream and asset record](internal/browserterm/UPSTREAM.md) for versions,
licenses, update steps, limits, and optional browser tests.
