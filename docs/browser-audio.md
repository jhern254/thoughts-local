# Browser recording in Thoughts

Speech should be another way to fill in a thought draft. The user should be able
to record, stop, review the words, and save through the same editor they already
use for typing.

**Today, F8 captures and sends sound, but the app discards it. Speech recognition
is not connected yet.** Tests use a fake recognizer to check how returned text
would reach the draft. This guide explains the recording boundary that a real
recognizer will use later.

## Keep recording part of the existing draft

A recording belongs to the draft open when the user presses F8. If permission
is denied or capture fails, that draft remains available. When a recognizer
supplies text, it updates the unsaved draft while preserving what the user typed
before recording. Saving remains the user's decision.

The responsibilities follow a small path:

```text
Browser captures sound
    → Go receives it through a separate audio connection
    → a small memory queue feeds the consumer
    → returned text is checked against the current recording
    → the editor updates the unsaved draft
```

The consumer is the replaceable part that processes sound. The app currently
uses a consumer that discards it; tests supply one that returns known text.
Capture and draft ownership can therefore be checked without a speech engine.

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
differences out of the server and the eventual recognizer.

## Stop a backlog before it grows

Processing may fall behind capture. Go holds at most two seconds of waiting
audio, with one more chunk being processed. If that queue fills, recording
stops. It does not keep growing or spill sound into a file.

The server also limits both recording time and the amount of sound received to
20 minutes. These checks apply even if the browser misbehaves. The browser has
its own sending bound and stops capture if it cannot keep up; an abandoned
connection times out.

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

Run `make tui/browser` to try capture and Stop with the discard consumer.
The [browser maintenance guide](../internal/browserterm/UPSTREAM.md#browser-acceptance)
has developer test commands. Chromium and Firefox tests use synthetic sound;
they check capture and recording behavior, not physical microphones or
recognition accuracy.

The exact message format lives in
[audio_protocol.go](../internal/browserterm/audio_protocol.go). Browser conversion
lives in [pcm-worklet.js](../internal/browserterm/static/pcm-worklet.js), and
[recording.go](../internal/voice/recording.go) defines permission to apply
transcript updates.
