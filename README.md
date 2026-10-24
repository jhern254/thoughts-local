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
Each browser server needs its own port, even when its database is temporary.
To use port 7878:

```sh
THOUGHTS_BROWSER_PORT=7878 make tui/browser/demo
```

If that port is already in use, stop the existing server with Ctrl+C in its
terminal or choose another port.

The stress fixture spans about a month, includes empty historical events, and
puts 200 thoughts in the current event. Expand events, scroll the thought picker,
browse Subjects and Misc thoughts, and resize the browser to exercise the larger
dataset. This is an interactive stress demo, with no fixed performance promise.

The application, SQLite, and filesystem remain native. The Go executable embeds
the browser terminal assets; Node/npm, a CDN, WASM, and a separate frontend server
are not needed to build or run it.

### Navigation and exit

In Subject Details, press **a** to manage aliases for the same Subject. Enter
adds or edits a name, **d** removes an alias after confirmation, and **Q/Esc**
returns. Subject names and aliases are case-sensitive.

If a new Subject name is already an alias, **d** offers to remove that alias and
create a separate empty Subject after confirmation. Existing records stay with
the original Subject.

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
**Ctrl+,** / **Cmd+,** without leaving a draft. Under Visual, choose a local JPEG/PNG image or SDR H.264 MP4/MOV video, preview it, and
apply it behind the terminal. Background darkness changes only the readability
overlay; Apply saves it. Remove background returns to the normal dark surface.
Controls are rendered in the TUI: Tab/Shift+Tab moves focus, arrows adjust
darkness, Enter activates, and Q/Esc returns without saving. Mouse buttons and
slider dragging also work. The browser supplies the device picker and the real
image inside the preview border. These controls affect browser presentation,
not terminal drafts or stored Thoughts.

Use **Background fit: Fill** to cover the browser window or **Fit** to show the whole
image with dark margins. In Fill mode, **Adjust framing** opens a larger preview:
drag or use arrow keys to move the image, and **+ / −** to zoom (100–300%).
**Done** keeps your draft framing; **Apply** saves it. **Cancel/Esc** restores the
framing from entry, and **Reset framing** centers it at 100% zoom. The preview
matches the current browser window's proportions; resizing may reveal different
edges. Framing changes preserve the imported display copy. Thoughts never modifies
the source file on your device, and background import does not archive an original
inside the app. Darkness changes one percentage point per Left/Right press;
Home/End select its limits.


Images must be non-animated, at most **16 MiB**, **8,192 pixels per side**, and
**24 million pixels** total. SVG, GIF, animated PNG, and remote URLs are unsupported.
The default darkness is 42%; the range is 0–95%.

Imported copies live in `~/Documents/Thoughts/backgrounds` (under
`%USERPROFILE%\Documents\Thoughts` on Windows), independently of the checkout
and browser profile. Each database retains its own background selection.
Back up the database and background folder together. In-memory databases do
not support persistent backgrounds.

Every newly imported background is re-encoded into a bounded display copy, including small images. Orientation is applied to pixels, unnecessary embedded metadata such as location, device details, dates, comments, and thumbnails is removed, and supported display color information is retained. JPEG output is lossy; larger images may also be resized.

Existing backgrounds and older backups are not automatically sanitized. Replace or reimport a background to apply the current policy. Local background assets are not encrypted yet.

Some JPEG/PNG color encodings are unsupported. When an image is rejected for compatibility, export a standard SDR/sRGB JPEG or PNG in an image editor and try again; all normal size and validation limits still apply.

See [Media handling and privacy](docs/media-handling-policy.md) for the distinction between background display copies and future private media attachments. Metadata removal does not anonymize what is visible in a picture.

Video imports require installed **FFmpeg and ffprobe** with H.264 encoding support.
Sources may be up to four hours, at most 8 GiB, 4,096 pixels per side, 9MP per frame,
and 60 FPS. Choose **Video quality** before preparing a loop:

| Quality | Resolution ceiling | FPS ceiling |
| --- | --- | --- |
| Efficient (default) | 1280×720 | 30 |
| High quality | 1920×1080 | 30 |
| Maximum | 1920×1080 | 60 |

These ceilings also support portrait footage and intermediate sizes/rates,
including 23.976, 29.97 and 59.94 FPS. Smaller/slower sources stay smaller/slower;
Thoughts preserves aspect ratio without upscaling or interpolating frames.
Higher quality takes more import time, storage and playback resources; see the
[T480s measurements](docs/browser-background-performance.md#video-quality-through-1080p60).
Thoughts stores a silent display copy and a still poster. Under Visual, adjust
**Playback speed** from 25–100% in five-point steps,
or **Play/Pause video**; Apply saves these preferences. Slower playback can look
less smooth. Hidden tabs, disconnected sessions and reduced-motion preferences
pause playback automatically; a deliberate Play action can override reduced
motion for the current session. Preview playback pauses the active background.
For a new upload, browse a paused local preview before transferring the source.
**Source / Start** moves the frame and proposes the next 30 seconds, clamped to
the source; shorten **End** for a smaller cut. Arrow keys adjust by 0.1 seconds,
Shift by one second, PageUp/PageDown by one minute, and Home/End reach the source
boundaries. Enter on Source / Start accepts `HH:MM:SS` with optional milliseconds.
The first **Preview loop** transfers the whole source once to temporary local
staging, validates it, and prepares only the selected 1–30 seconds. Later previews
reuse that staged source until the editor closes. A fading braille wave above Options shows activity during preparation; press
Esc or Tab then Enter to cancel the upload. **Done** and **Apply** use the last successful preview,
even if you subsequently change the cut points or quality. The editor shows the
previewed quality and actual dimensions/FPS; preview again to accept a new quality.
Apply saves those exact previewed bytes and their quality preference. With no
pending import, changing quality only changes the preference for future imports.
Speed changes playback duration,
not the selected footage. Saved videos cannot be re-trimmed; reimport the source.
Some MP4/MOV timestamp layouts cannot be previewed consistently across browsers;
export with timestamps starting at zero if requested. HDR/HEVC and AI animation
remain separate work.

Background assets remain local and are not cloud-uploaded.

### Local trust and maintenance

Browser mode binds only `127.0.0.1`, validates the exact Host and Origin, and
serves embedded client files and the selected application-owned background assets. Use the printed numeric address, not a hostname or
reverse proxy. This is a local single-user application: processes running as
your user can access it. There is no LAN mode, authentication, or shell endpoint.
Disable `TEA_TRACE` before launching browser mode; terminal traffic recording is
refused. Logs use the same fixed-message policy as native mode.

The adapter uses xterm.js and selected ideas/options from sip. See
[the upstream and asset record](internal/browserterm/UPSTREAM.md) for versions,
licenses, update steps, limits, and optional browser tests.
