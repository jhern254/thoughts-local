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
“Programming in Go thought, today I learned about interfaces.” Unique voice-name
matches select the canonical Subject. The full transcript remains visible during
recording. Confirmed cleanup removes the prefix once and capitalizes the body's
first Unicode letter. Nothing is saved until Ctrl+S.

The Subject label identifies provisional suggestions and fuzzy ambiguities.
Unknown names fill the existing Subject input without creating anything; after
Stop, use Tab and “Create subject…” or choose an existing Subject. An ambiguous
fuzzy match requires selection to assign a Subject. Ordinary speech such as
“I thought about learning Go” remains unchanged and may be saved as Misc.
Existing Subject context and manual
choices take precedence. Subject and text controls remain locked while recording.
Only the initial recording into an otherwise empty draft recognizes a command;
later recordings and recordings appended to typed text are literal. Empty or
denied recording attempts do not consume that initial opportunity.

`internal/subjectdiscovery` separates a pure parser, candidate reads, and ranking.
The bounded parser considers only the first standalone case-insensitive
“thought” after at most eight words and 255 Unicode characters. Intent precedes
identity resolution: pronouns immediately before the delimiter (including after
introductory phrases), auxiliary/passive endings, pronoun/adverb constructions,
and determiner-led noun readings such as “My first thought was ...” are ordinary
speech even if the prefix happens to be a saved name or alias. Noun readings use
only a bounded lookahead at the body's first word, not body classification.

Voice spelling is compared with `strings.EqualFold` across preloaded canonical
names and aliases, counting distinct canonical Subject IDs. A unique match selects
the canonical Subject, regardless of transcription capitalization. Multiple IDs
form a collision even if one spelling matches exactly: no automatic selection,
candidate list, or prefix removal occurs, and Misc saving remains available.
Aliases differing only by case for the same Subject are not a collision.

Single-word cues and sentence-like titles are possible commands. A unique folded
name can confirm them; fuzzy scores and usage cannot establish their intent.
This preserves known names such as “The Art of Learning”, “I Ching”, “I Want to
Learn Go”, and “My Learning Journal”. An unfamiliar single word such as
“meditation” appears as an optional proposal. Saving without confirming it retains
the entire transcript as Misc. The existing Create subject flow or an explicit
picker selection confirms it and removes the prefix once, capitalizing the first
Unicode letter. Canceling creation leaves the proposal optional. Editing the body
or recording again prevents later confirmation from replacing that text; a new
recording retires the earlier cleanup permission.

Neutral multiword names such as “Machine Learning Research” retain the existing
recommendation and creation flow. If name data has not arrived by Stop, a possible
command remains literal and unassigned after a late read. Confirmed commands can
resolve late, but automatic cleanup waits until the snapshot rules out a folded
identity collision; any intervening edits remain untouched. An indefinite article
immediately before “thought” introduces ordinary
speech (“Just a thought”, “Here's another thought”). Interior indefinite articles
and comparative “more thought” phrases remain uncertain without name evidence.

Accepted revisions reevaluate intent even when the cue spelling is unchanged;
ordinary revisions withdraw automatic suggestions. Transcript revocation freezes
the final accepted command decision and byte boundary; `RecordingEnded` removes
only that authorized prefix once. Late candidate data may resolve a frozen,
confirmed cue, but cannot reopen intent classification. Starting another recording
clears the previous cue and permission; appended/subsequent recordings remain
literal regardless of candidate arrival timing. Manual selections still take
precedence. These conservative English heuristics cannot
distinguish every title from ordinary speech; names containing the delimiter or
grammatically ambiguous titles may need manual selection. Subject spelling and
persisted identity remain case-sensitive. No uncertain prefix is removed or
command-capitalized.

Candidates (Subjects, aliases, and usage) load once per draft under its context.
Live revisions reuse prepared matching features, with no database reads per
transcript. Alias/count failures degrade to the remaining data. This is not a
transactional database snapshot: concurrent changes in another session may
require reopening the draft; save/creation still use authoritative validation.

Unique folded canonical/alias matches bypass scoring. Collisions stop resolution
before fuzzy ranking or usage; database identity and `ResolveName` remain strictly
case-sensitive. For confirmed commands without a folded match,
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
