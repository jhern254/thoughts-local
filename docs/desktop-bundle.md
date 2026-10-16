# Thoughts with local speech

This folder contains the Thoughts browser app and its speech engine. Keep the
folder together when moving it. You do not need Go, a compiler, or a separate
Moonshine installation.

Run `./thoughts-tui --browser` on Linux/macOS, or
`.\thoughts-tui.exe --browser` in Windows PowerShell. The app uses your existing
Thoughts database; `--db-dsn` can select another migrated database.

When Models appears, choose **Download and enable**. That downloads the speech
model with progress and enables F8 without a restart. The engine is already in
this app folder. **Not now** lets you keep writing. **Options → Models** returns
to setup later.

Downloaded models live in `~/Documents/Thoughts/models` (Linux/macOS) or
`%USERPROFILE%\Documents\Thoughts\models` (Windows). They remain available after updating
or moving this app folder. Audio is kept in memory; transcripts stay unsaved
until you save the Thought normally.

The engine is Moonshine Voice v0.1.5, with the approved Small Streaming English
model. Dependency notices are in `licenses/`. This is the app release payload,
not a separate engine installer. CI produces unsigned release candidates;
code signing and database installation are separate work.
