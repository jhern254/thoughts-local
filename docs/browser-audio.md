# Browser recording in Thoughts

Speech should be another way to fill in a thought draft. The user should be able
to record, stop, review the words, and save through the same editor they already
use for typing.

With the approved model installed and the native runtime configured, browser
recording uses local Moonshine speech recognition. On the Events timeline, F8
opens a thought and immediately starts recording. T opens a draft for typing.
Inside the draft, F8 starts or stops recording. Review and edit the words before
saving.

Without speech configured, Thoughts still starts and supports typed drafts.
Trying to record explains that speech is unavailable without requesting the
microphone. See [Moonshine setup](moonshine.md#browser-speech) to enable it.

## Keep recording part of the existing draft

A recording belongs to the draft open when the user presses F8. If permission
is denied or capture fails, that draft remains available. When a recognizer
supplies text, it updates the unsaved draft while preserving what the user typed
before recording. Saving remains the user's decision.

A thought's observation time is captured when its draft opens, whether through
Create thought, T, or F8. Recording restarts, subject selection or creation, and
save retries retain that time. Canceling and opening another draft captures a
new time. Database creation and update timestamps still describe persistence.
Event start and end forms likewise capture their default time when opened and
allow the user to edit it. API clients must supply `observed_at` when capturing
before submission; omitting it defaults to the service call time.

The responsibilities follow a small path:

```text
Browser captures sound
    → Go receives it through a separate audio connection
    → a small memory queue feeds the consumer
    → returned text is checked against the current recording
    → the editor updates the unsaved draft
```

The browser process loads the model once. Each recording gets its own Moonshine
stream, so sound can arrive a small piece at a time without keeping the whole
recording in Go memory. Returned words replace that recording's contribution to
the unsaved draft as recognition improves. Tests can still use a fake consumer
to check capture and draft ownership independently of the speech engine.

## Spoken Subject prefixes

Start an empty voice draft with a name followed by “thought”, for example
“Programming in Go thought, today I learned about interfaces.” Exact canonical
names and aliases select the canonical Subject. The full transcript remains
visible during recording. After Stop, the command is removed and the body's
first Unicode letter is capitalized. Nothing is saved until Ctrl+S.

The Subject label identifies provisional suggestions and ambiguous matches.
Unknown names fill the existing Subject input without creating anything; after
Stop, use Tab and “Create subject…” or choose an existing Subject. An ambiguous
match requires selection. Ordinary speech such as “I thought about learning Go”
remains unchanged and may be saved as Misc. Existing Subject context and manual
choices take precedence. Subject and text controls remain locked while recording.
Only the initial recording into an otherwise empty draft recognizes a command;
later recordings and recordings appended to typed text are literal. Empty or
denied recording attempts do not consume that initial opportunity.

`internal/subjectdiscovery` separates a pure parser, candidate reads, and ranking.
The conservative English command grammar accepts a standalone case-insensitive
“thought” after at most eight words and 255 Unicode characters. Articles and
pronouns may start names such as “The Art of Learning”, “I Ching”, or “I Want to
Learn Go”. Bare conversational words and personal-pronoun fragments ending in
common auxiliaries, modifiers, or indefinite articles are declined (for example,
“I thought”, “I never thought”, and “I had a thought”). These conservative
English heuristics cannot distinguish every title from ordinary speech; this is
a command convention, not full natural-language understanding. Subject spelling
and persisted identity remain case-sensitive.

Candidates (Subjects, aliases, and usage) load once per draft under its context.
Live revisions reuse prepared matching features, with no database reads per
transcript. Alias/count failures degrade to the remaining data. This is not a
transactional database snapshot: concurrent changes in another session may
require reopening the draft; save/creation still use authoritative validation.

Exact case-sensitive canonical names and aliases bypass scoring. Otherwise,
ranking takes the larger Dice overlap of significant word sets and Unicode
character trigrams, using the best canonical/alias spelling per Subject. A 0.75
minimum and 0.10 winner margin govern selection. Close relevant ties may use
Thought counts only when all counts are known and one Subject has at least three
Thoughts, at least three more than each competitor, and at least twice their
usage. Other ties remain ambiguous. These are tunable heuristics, not calibrated
probabilities. No extra model, persistence, or inference is involved.

Recommendations share transcript authority with editor mutation. Stop revokes
that authority immediately; `RecordingEnded` cleans only the last accepted
initial contribution, once. Prefix-only input is retained for editing. Candidate
replies carry draft, request, session and component identity, and cannot override
manual selection/creation or a retired draft. No transcript or cue is logged.

## Give sound its own connection

Typing and recording have different needs. Sound arrives continuously and
needs its own limits and cleanup. `/voice/audio` carries sound; `/ws` continues
to carry typing and recording controls. Sound never becomes terminal input.

Starting a recording gives the browser a temporary, single-use permission
token. Go binds it to that exact recording, and the browser presents it when
opening the audio connection. The token expires and Stop revokes it. It stays
out of URLs, logs, and the database.

Browsers capture sound at different rates. The capture code converts it to one
agreed format and sends small, uniform chunks. This keeps those device
differences out of the server and recognizer.

## Stop a backlog before it grows

Processing may fall behind capture. Go holds at most two seconds of waiting
audio, with one more chunk being processed. If that queue fills, recording
stops. It does not keep growing or spill sound into a file.

The server also limits both recording time and the amount of sound received to
20 minutes. These checks apply even if the browser misbehaves. The browser has
its own sending bound and stops capture if it cannot keep up; an abandoned
connection times out.

The worklet permits ten unacknowledged 100 ms frames (32,000 bytes), giving the
browser about one second to recover from a brief main-thread stall. The browser
socket remains bounded at 48,000 queued bytes, below Go's twenty-frame waiting
queue (64,000 bytes). A recovered worklet burst therefore fits the downstream
ceilings. These limits do not batch or delay normal audio: each frame is sent
immediately. Sustained overload still ends capture instead of silently dropping
sound or accumulating an unbounded recording. Stop clears late frames as before.

## Make Stop freeze the draft

As soon as Go processes Stop, that recording loses permission to change the
draft. Cleanup then stops capture, discards waiting sound, and waits for
started processing to return. The editor waits for cleanup before allowing
another recording.

A text update may already be on its way to the editor when recording A stops.
If B starts on the same draft, that old update must not become part of B. The
editor checks which browser session, draft, and recording each update belongs
to, and accepts only a newer update from the current active recording. A late
result from A cannot change B's text. No final result is accepted after Stop.

Canceling, leaving the draft, or disconnecting also ends the recording. Reloading
creates a new session, so results from the old page cannot affect the new one.

## Keep sound temporary

Audio stays in recording-owned memory. Thoughts does not put it in SQLite,
logs, temporary files, or diagnostic attachments. Stopping releases the capture
resources and clears queued sound.

Processing code must stop when canceled and must not retain the sound it
receives. Go waits for that work before completing cleanup. Recognized words
are ordinary unsaved draft text; they reach the database only through the normal
explicit Thought save.

## Working on this boundary

Run `make tui/browser` for typed drafts, or enable speech using the
[explicit native setup](moonshine.md#browser-speech).
The [browser maintenance guide](../internal/browserterm/UPSTREAM.md#browser-acceptance)
has developer test commands. Chromium and Firefox tests use synthetic sound;
they check capture and recording behavior, not physical microphones or
recognition accuracy.

The exact message format lives in
[audio_protocol.go](../internal/browserterm/audio_protocol.go). Browser conversion
lives in [pcm-worklet.js](../internal/browserterm/static/pcm-worklet.js), and
[recording.go](../internal/voice/recording.go) defines permission to apply
transcript updates.
