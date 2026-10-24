# App style guide

## Clean distributions and waves

Clean distribution curves are a preferred part of Thoughts' visual language.
Use them where they help communicate a pattern or ongoing activity. Keep the
shape readable, the surrounding space quiet, and the relationship to nearby
content explicit.

- Prefer a thin, clear trace with restrained purple accents. Braille characters
  suit the terminal aesthetic, including focused browser enhancements.
- Use a subtle axis when it clarifies the shape. Align the graphic with the
  content it belongs to and adapt its width when that content resizes.
- Keep motion deliberate and predictable. A bright leading dot and a fading
  tail can communicate activity without a heavy glow or layered effects.
- Preserve text labels, focus styling, and the space needed for controls.
  Animation should support the task rather than compete with it.

A data distribution and a decorative activity wave have different meanings.
Data graphics must reflect actual values, with appropriate scales and labels;
do not reshape them into an attractive curve. An activity wave is a repeating
visual cue, not a measurement or a completion percentage.

## Approved video-preparation indicator

The reviewed design uses a fading braille trail along a fixed sine wave, with a
brighter leading dot. Its full path spans the video preview box and completes
two cycles. Both endpoints meet the horizontal zero axis at the left and right
edges; the axis runs through the vertical center. Positive and negative portions
remain visible, so the wave crosses the line without flattening against it.

Place the indicator between **Playback speed** and **Preview loop**, aligned
horizontally with the video preview box. Reserve its space while hidden so
loading never moves the controls. Keep its height compact while leaving enough
room for distinct peaks and troughs. Show a plain status label such as
“Preparing video…” alongside the visual activity cue.

The curve repeats independently of encoding progress. Resizing changes its
display width, not the meaning of the animation. If actual completion progress
becomes available, present it explicitly rather than implying that the moving
dot measures work completed.

The browser renders the braille curve once for the fading trail and bright head.
CSS moves clipped light windows over that fixed text in about 12 steps per
second. These are implementation choices, not a required rendering API or a
performance guarantee. The TUI reserves seven terminal rows for the browser
indicator throughout cut selection, preserving its original responsive height.
Resizing uses the existing terminal layout and transport. Animation frames remain local and do not rewrite text or send terminal or HTTP
updates. See the [measurement notes](browser-background-performance.md#video-preparation-activity-wave)
for the cost of the integrated indicator.

## Movie workflow layout

Validation, cut selection, rendering and prepared-loop review share preview,
control and footer positions. Use placeholders for unverified source bounds;
keep unavailable actions visible but disabled. Reserve the same loading-wave
space when idle. Keep Cancel upload on its own row after a blank separator and
leave a blank row between prepared-loop details and the Preview ready status.

## Small-window movie editing

Keep the movie preview at its normal 12-terminal-row height. Scroll the controls
beneath it as keyboard or wheel focus moves, while keeping accepted-loop details,
status, and help visible. Resizing must preserve source position, cut points,
accepted bytes, and editor/session ownership. Clip the loading indicator to its
visible reserved control rows; it must never cover the preview or footer.

Below 48 columns or 30 rows, show a resize notice instead of a clipped editor.
Preserve the staged source and cut, block hidden edits, and retain explicit
cancellation (including Stop rendering while preparation is active). Restore the
same editor when the window becomes usable again.

## Motion, ownership, and verification

Keep animation local to the renderer that owns the display, following
[presentation ownership and rendering efficiency](../AGENTS.md#presentation-ownership-and-rendering-efficiency).
This applies to the current browser interface and a future Go renderer.

Use a small, bounded amount of rendering work. Stop updates when the operation
finishes, its view closes, or the display becomes hidden. Respect reduced motion
with a static visual and a readable status label. Decorative dots should not
generate repeated screen-reader announcements or interfere with keyboard input
and cancellation.

Review the actual appearance at normal and narrow sizes, including resizing,
both endpoints, the fading tail, and reduced motion. Before claiming efficiency,
measure the integrated indicator during real video preparation on reference
hardware: animation frame timing, CPU and memory use, and input responsiveness.
Check repeated operations for lingering timers or resource growth. Browser
rendering and braille glyphs alone do not establish low cost.

## Thought draft editor

Use the shared Bubble Tea textarea for text and cursor movement. New drafts
start in Insert; show INSERT in a teal block or NORMAL in the shared purple
highlight beside the title, with mode-specific help
below the text. Use a steady, contrasting block cursor. Current-line background
highlighting is a separate saved preference, off by default; it must not replace
or obscure the cursor. Modal letter commands apply only to focused thought text,
not the subject picker. In Normal mode, v controls recording and u restores the
last automatic voice-prefix cleanup. These letters remain text in Insert and
the Subject field; while recording locks editing, v can also stop capture.

Esc preserves the draft and enters Normal, stopping recording first if needed.
Ctrl+C in Insert mode behaves like Esc; browser selections still copy.
Ctrl+Q explicitly cancels; a nonempty draft requires confirmation with Keep
editing selected initially. Dictation appends at the end, with editing locked
until capture cleanup completes. Reserve the actual wrapped help/confirmation
rows before sizing the textarea so narrow layouts retain visible actions and a
visible cursor. Browser enhancements do not own this editing workflow.

### Browser readability

| Capability | Coverage |
| --- | --- |
| Structural outlines and panel dividers | Normal, Heavy |
| Text weight | Independent Heavier text toggle |
| Text size | Standard, Larger |
| Contrast over wallpaper/video | Optional stronger text contrast and effective darkness |
| Visible keyboard focus | Existing highlighting retained; browser contrast mode strengthens focus outlines |
| Preview, cancel, save, reload | Supported for all readability preferences |
| Reduced motion | Existing OS preference handling retained |
| Screen readers | No new verified support; requires separate work |

Options → Accessibility offers independent border weight, text weight, text size,
and contrast preferences. Defaults are Normal borders, ordinary text weight,
Standard size (14px), and High contrast off. Larger uses 16px;
retain monospace typography and the existing line spacing.

Structural borders share a single Go glyph mapping: rounded Normal and square
Heavy. Each edge stays one terminal cell wide. Apply the same weight to card outlines, section dividers, dialogs, and reserved media frames.
Timeline rails, distribution curves, sliders, and activity waves encode other
information and keep their existing graphics. Browser media placement must check
the matching frame glyphs before publishing a preview over terminal cells.

Text-size previews refit the existing terminal and use its normal resize channel;
wait for matching terminal geometry before showing media overlays. Settings must
remain keyboard accessible at narrow widths and short heights, scrolling controls
to keep Apply and Back reachable. Back/Esc restores saved presentation, and failed
saves retain the draft for retry.

High contrast uses xterm's minimum contrast adjustment with a target of 7 and an
effective background darkness of at least 85%, without overwriting saved Visual
darkness or pausing the visible video. Browser status and focus outlines remain
visible. This is a readability aid, not a claim of verified WCAG conformance over
arbitrary images, video, or all terminal color combinations. Screen-reader support
requires a separately tested design; these controls do not establish it.

Larger is capped at 16px. At a 960×450 browser-content viewport on a 1920×1080
display, Chromium and Firefox checks cover Events navigation, multiline thought
creation, and Options controls without resizing. The short timeline keeps a
scrolling body of at least six rows rather than hiding it behind a resize notice.
The movie editor also fits this reference viewport: below 30 terminal rows it
compacts the heading and footer, retaining the full 12-row media preview and
scrolling controls. The minimum is 48 columns by 20 rows. Full-size layouts retain
their existing presentation. During preparation, essential status and Stop/Cancel
remain available even when there is no room for the optional activity wave.

## Day activity overview

The [day activity navigator](day-activity-navigator.md) uses a dim Total curve
with independently toggled Event and Thought layers above the Events heading.
Its Braille baseline meets the filled slopes; a quiet row below holds the
accepted-day block in the same purple as the Events highlights.
Keep its canvas bounded: zoom and styling must not push the timeline downward.
Its samples represent daily activity (or daily averages over wider ranges), not
probability. Keep its accepted-date marker, focus order and compact-height
behavior aligned with the daily timeline.
