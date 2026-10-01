# Building AI Chat app in Go Lang, Python, SQLite 

## Thoughts TUI

Build with `make tui/build`. Apply migrations explicitly with `make migrate/up`
before using a new database. The executable never applies migrations at startup.

```sh
./bin/thoughts-tui                       # native terminal (default)
./bin/thoughts-tui --browser             # http://127.0.0.1:7681
./bin/thoughts-tui --browser --browser-port 8123
```

Both modes accept `--db-dsn` / `THOUGHTS_DB_DSN`. Browser mode accepts
`THOUGHTS_BROWSER_PORT`; the flag takes precedence. Open the printed address.
`make tui/browser` migrates and launches the persistent database;
`make tui/browser/demo` launches the existing seeded disposable demo.
For the stress fixture: `make tui/demo/stress TUI_ARGS=--browser`.

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
  to return focus. Microphone, attachments, and mobile packaging are deferred.

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
