# Building AI Chat app in Go Lang, Python, SQLite 

## Database migrations

Run `make migrate/up` (optionally `DB_PATH=/path/to/thoughts.db`). The Make targets
build the standard golang-migrate CLI into `bin/migrate` from the pinned module in
`tools/migrate`. No globally installed migration CLI is required. The tool uses
modernc SQLite v1.46.1 (SQLite 3.51.2), matching the application. Older CLI builds
can lack `unhex()`, which the schema needs for exact counts after embedded NULs. Keep the
tool and application SQLite versions aligned when upgrading. `make check` also
verifies the migration tool builds; `make build` builds only the application.
The tool dependencies do not enter the application module.

`make migrate/down` rolls back one migration; `make migrate/version` reports the
version. Disposable TUI demos use the same local migration binary. Migrations
remain explicit and are never applied automatically by the executable.

`make dev/seed` and seeded demos also use the system `sqlite3` command. Use
SQLite **3.41.0 or newer**, which provides [`unhex()`](https://www.sqlite.org/releaselog/3_41_0.html),
when executing migrations or writing this schema with external SQLite tools.
Check your CLI version with `sqlite3 --version`.

The development schema was revised in migration `000004`: recreate disposable
development databases after this change. Running migrations against an already
migrated database does not retrofit the revised table definition.

Thoughts store a generated `character_count`: the number of Unicode code points
in the complete stored text, including whitespace, newlines and embedded NULs.
SQLite maintains this STORED value on every write, including SQL imports. The
1,000,000-code-point limit uses this count, including text after embedded NULs. The
existing active chronological index appends the count, letting Event statistics
read integers from the index; Event pages read the same value for visible rows.
Ordinary browsing and latest previews still select zero for their unused count.
The SQL migrations are authoritative; `mvp_schema.dbml` is a schema overview.

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
