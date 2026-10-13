# Building AI Chat app in Go Lang, Python, SQLite 

## Database migrations

Run `make migrate/up` (optionally `DB_PATH=/path/to/thoughts.db`). The migration
tool builds locally; no global installation is required. `make migrate/down`
rolls back one migration, and `make migrate/version` reports the version.
The executable does not apply migrations automatically.

Seeding commands and demos require the system `sqlite3` CLI, version **3.41.0
or newer**.

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

### Navigation and exit

- Q and Esc go back through lists, details, expanded events, and curve controls.
- In forms and filter input, Q remains text; Esc cancels or goes back.
- At top-level Events or the focused entity selector, Esc or Ctrl+C opens an
  exit confirmation. No is selected by default. Use Left/Right or Tab to select,
  then Enter to accept; Y exits immediately, and N/Q/Esc cancels.
- Ctrl+C does nothing in nested views and forms. Q on the entity selector
  focuses Events; Q on top-level Events does nothing.

### Browser behavior

- One active tab. A second tab reports that the first is busy and offers Reconnect.
  Opening/reloading retries admission for up to one second while an old session
  finishes closing.
- Confirming exit ends that session. Ctrl+C with a browser text selection
  copies it; without a selection it follows the exit rules above. Ctrl+C in
  the launching terminal stops the server.
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

### Browser background

Select **Options** alongside Thoughts and Subjects (Tab, arrows, Enter), or use
**Ctrl+,** / **Cmd+,** without leaving a draft. Under Visual, choose a local JPEG or PNG, preview it, and
apply it behind the terminal. Background darkness changes only the readability
overlay; Apply saves it. Remove background returns to the normal dark surface.
Controls are rendered in the TUI: Tab/Shift+Tab moves focus, arrows adjust
darkness, Enter activates, and Q/Esc returns without saving. Mouse buttons and
slider dragging also work. The browser supplies the device picker and the real
image inside the preview border. These controls affect browser presentation,
not terminal drafts or stored Thoughts.

Use **Image fit: Fill** to cover the browser window or **Fit** to show the whole
image with dark margins. In Fill mode, **Adjust framing** opens a larger preview:
drag or use arrow keys to move the image, and **+ / −** to zoom (100–300%).
**Done** keeps your draft framing; **Apply** saves it. **Cancel/Esc** restores the
framing from entry, and **Reset framing** centers it at 100% zoom. The preview
matches the current browser window's proportions; resizing may reveal different
edges. The original image is retained. Darkness changes one percentage point per
Left/Right press; Home/End select its limits.


Images must be non-animated, at most **16 MiB**, **8,192 pixels per side**, and
**24 million pixels** total. SVG, GIF, animated PNG, and remote URLs are unsupported.
Large imports are resized to a lightweight application-owned copy; the image on
your device is unchanged. Reimport an existing background to optimize it.
The default darkness is 70%; the range is 0–95%.

Imported copies live in
`<database filename>.assets/appearance/` (normally
`data/thoughts.db.assets/appearance/`), independently of the browser profile.
Back up the database and its asset directory together. In-memory databases do
not support persistent backgrounds.

Background assets are local application data, are not cloud-uploaded, and are
**not encrypted**. Future encrypted-vault work must include these personal media
assets. Video backgrounds and AI animation are separate future work.

### Local trust and maintenance

Browser mode binds only `127.0.0.1`, validates the exact Host and Origin, and
serves embedded client files and the selected application-owned background image. Use the printed numeric address, not a hostname or
reverse proxy. This is a local single-user application: processes running as
your user can access it. There is no LAN mode, authentication, or shell endpoint.
Disable `TEA_TRACE` before launching browser mode; terminal traffic recording is
refused. Logs use the same fixed-message policy as native mode.

The adapter uses xterm.js and selected ideas/options from sip. See
[the upstream and asset record](internal/browserterm/UPSTREAM.md) for versions,
licenses, update steps, limits, and optional browser tests.
