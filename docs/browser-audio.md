# Ephemeral browser audio transport

The F8 recording controls now send PCM to a dedicated `/voice/audio` WebSocket.
Ordinary browser sessions consume and discard it: **no speech recognizer is
connected yet**. Only injected test consumers produce synthetic transcript text.
Moonshine, model installation, SQLite speech state, and application Runtime
wiring are outside this change.

## Two paths with different responsibilities

`/ws` remains the terminal/control connection. Its bounded `v` frames contain
recording controls and server state, including a one-use capability. Audio and
transcript uploads are rejected; transcript results are never keyboard events.

```
browser microphone → AudioWorklet → binary /voice/audio
                                         ↓
                            bounded recording-owned PCM queue
                                         ↓
                            injected consumer (default: discard)
                                         ↓
                     Go-only transcript update with recording identity
                                         ↓
                       authority check at the draft mutation
```

The terminal session owns an unpredictable 128-bit session identity. Each Start
has a fresh recording number within the current draft. Reload/reconnect creates
a new session. The complete `(session, draft, recording)` key identifies work
independently of any speech engine or native handle.

Start creates a 256-bit random capability bound to that key. It expires after
60 seconds if unused, authorizes one connection, and is revoked on Stop, cancel,
replacement, failure, or disconnect. Knowing numeric IDs is insufficient. Both
connections require the existing exact loopback Host and Origin checks. Audio
URLs cannot contain queries; the capability belongs only in the binary handshake.
This is local recording authorization, not a general user authentication system.

## PCM format and browser conversion

PCM represents the waveform as regularly spaced amplitude samples, without an
encoded container. The wire format is **one channel (mono), 16,000 samples per
second, signed 16-bit integers, little-endian**. Each sample occupies two bytes;
100 ms contains 1,600 samples / 3,200 PCM bytes.

Little-endian means the least significant byte comes first. For example:

| Bytes | Signed sample |
| --- | ---: |
| `01 00` | 1 |
| `ff 7f` | 32,767 |
| `00 80` | −32,768 |
| `ff ff` | −1 |

The browser supplies float amplitudes, ordinarily in `[-1, 1]`. The worklet
explicitly requests mono input, rejects non-finite values, clamps amplitudes,
and converts them to the signed integer range. `DataView.setInt16(..., true)`
selects little-endian explicitly; Go decodes with `binary.LittleEndian`.

The AudioContext uses the browser's device processing rate, often 44.1 or 48 kHz.
The worklet averages the source waveform over each 16 kHz output interval and
carries fractional intervals across processing blocks. Relabeling bytes with a
new rate would change their interpreted duration and pitch. This simple bounded
conversion proves transport; recognition quality and higher-quality filtering
remain future work. The node's output stays silent to avoid microphone playback.
No MediaRecorder container, WAV parser, or server codec is involved. The older
MediaRecorder discard helper remains separate and is not started by this path.

## Binary protocol version 1

All audio WebSocket messages are binary, uncompressed, and start with four bytes:
`54 41 01 <type>` (`TA`, version 1, frame type). Multi-byte integers below are
unsigned little-endian. No authored text is accepted on this uplink.

### Initial handshake: type 1, exactly 76 bytes

| Offset | Bytes | Meaning |
| --- | ---: | --- |
| 0 | 4 | Common header |
| 4 | 16 | Raw session identity (hex in control metadata) |
| 20 | 8 | Draft ID |
| 28 | 8 | Recording ID |
| 36 | 32 | Raw one-use capability (hex in control metadata) |
| 68 | 4 | Sample rate, exactly 16,000 |
| 72 | 1 | Channels, exactly 1 |
| 73 | 1 | Encoding, 1 = signed PCM16LE |
| 74 | 2 | Reserved, both zero |

Wait for Ready before sending PCM. IDs supplied to the browser stay within
JavaScript's exact integer range; the wire uses uint64.

### PCM: type 2, 12-byte header followed by actual sample bytes

| Offset | Bytes | Meaning |
| --- | ---: | --- |
| 0 | 4 | Common header |
| 4 | 4 | Sequence, starts at zero and increases by one |
| 8 | 4 | Declared sample count, must match actual bytes |
| 12 | count × 2 | PCM16LE |

### Server status: type 3, exactly 5 bytes

The byte after the common header is: Ready `0`, Stopped `1`, Denied `2`, Invalid
`3`, Limit `4`, Busy `5`, or Failed `6`. Status frames contain fixed codes only.
On failure the connection closes; a status is best effort when a peer/read has
already failed. Stop is a control action on `/ws`, not an audio frame. Closing
the audio connection also revokes the recording.

## Independent limits

| Boundary | Server limit |
| --- | --- |
| Whole WebSocket message, including header | 8,192 bytes |
| PCM chunk | 3,200 samples / 6,400 bytes / 200 ms |
| Total audio bytes | 38,400,000 |
| Total samples | 19,200,000 |
| Wall time since accepted Start | 20 minutes |
| Outstanding audio, including consumer's current chunk | 32,000 samples / 2 seconds |
| Pending chunks | 20 |
| Sample ingress token bucket | 16,000/s, 32,000 burst |
| Message ingress token bucket | 50/s, 20 burst |
| Initial audio handshake | 5 seconds |
| Authorized read inactivity | 10 seconds |
| Capability attachment | 60 seconds |
| Active recording / pending handshake per session | 1 / 1 |

A fixed 8,193-byte read buffer bounds fragmented messages too. Sample count and
byte checks precede PCM allocation. The total recording allowance is a counter,
not a preallocated buffer. Tiny frames cannot evade queue or message-rate limits;
a fast consumer cannot evade ingress-rate limits. Overflow stops the recording
rather than dropping PCM, spilling to disk, or waiting indefinitely.

Browser limits are separate: four outstanding worklet transfers, 100 ms chunks,
and at most 48,000 buffered socket bytes. Exceeding either bound stops capture.
The server trusts neither those limits nor client-declared lengths.

## Authority, ownership, and privacy

Stop first revokes shared `TranscriptAuthority`. Revocation and synchronous draft
mutation share one lock, so queued results cannot cross that boundary. Then the
server cancels recording work, clears queued PCM, closes transport, and joins the
consumer. The editor remains stopping until the Go-only `RecordingEnded` message;
a browser acknowledgement cannot unlock it early. No final transcript is
accepted after Stop.

The draft checks the full current identity, active authority, and increasing
revision at mutation time. Updates describe the current recording's cumulative
contribution: they replace that contribution while preserving text typed before
Start. A later recording bases its contribution on the newly edited draft.
Transcript text is bounded to 16 KiB per update and checked against the editor's
plain-text contract. One pending update slot coalesces revisions; PCM is never
coalesced or dropped. Old results remain inert across Stop/restart, cancel,
draft replacement, disconnect, and a new session.

The session owns delivery; each recording owns its deadline, consumer, queue,
and connection. `PCMConsumer` borrows samples only until returning, must honor
context, and must retain none. Consumer errors and panic values are discarded;
only fixed status codes escape. Shutdown joins recording and session work before
shared application resources close. A consumer that ignores cancellation can
block shutdown; detached work is not permitted by this injection contract.

PCM has no store, logger, editor, or diagnostic dependency. Receive storage,
queued samples, and completed consumer chunks are explicitly cleared. The browser
stops tracks, closes its context/socket, and clears transferred/partial frames.
There are no audio temp files, database writes, audio history, debug WAVs, or
application-authored PCM diagnostics. `TEA_TRACE` remains refused in browser mode.
This does not promise secure erasure of browser/OS buffers, swap, or externally
configured process dumps. Transcript text is unsaved draft content until the
normal explicit Thought save.

## Tests and developer use

Normal `make tui/browser` exercises capture and discard, with fixed status text
making the absent recognizer explicit. No model or runtime downloads occur.

Portable Go tests use injected consumers and controlled clocks/channels. The
SQLite integration verifies no commits anywhere during capture/unsaved updates,
no audio temp files or private logs, and persistence only through explicit save.

For browser acceptance, install development dependencies, then let the opt-in Go
host own a disposable migrated, seeded app and fake consumer:

```sh
(cd internal/browserterm/client && npm ci --ignore-scripts && npx playwright install chromium firefox)
node --test internal/browserterm/client/microphone.test.mjs internal/browserterm/client/pcm*.test.mjs
THOUGHTS_BROWSER_AUDIO_TEST=1 go test -tags=integration -count=1 -timeout 12m -run '^TestBrowserAudioWorkflow_Playwright$' ./integration
```

Optional `THOUGHTS_BROWSER_TEST_FILTER` selects Playwright test names;
`THOUGHTS_BROWSER_ARTIFACTS=/tmp/thoughts-browser-review` retains synthetic
screenshots and test artifacts outside the source tree. Ordinary Go checks skip
this browser host. `npm test` against a running normal demo retains the existing
browser/control coverage and skips cases requiring the injected fake.

Chromium acceptance uses generated tones and silent Web Audio tracks. Firefox's
PCM cases use its synthetic microphone device (configured only in Playwright);
its permission/control cases retain silent generated tracks. Explicit permission
replies and the real worklet/socket path are exercised in both browsers. It proves
transport/lifecycle, not recognition accuracy or physical-microphone behavior.
No macOS/Windows microphone interaction or iOS support is claimed here.
