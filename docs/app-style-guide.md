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

Place the indicator above the **Options** line and align its horizontal bounds
with the video preview box. Keep its height compact while leaving enough room
for distinct peaks and troughs. Show a plain status label such as
“Preparing video…” alongside the visual activity cue.

The curve repeats independently of encoding progress. Resizing changes its
display width, not the meaning of the animation. If actual completion progress
becomes available, present it explicitly rather than implying that the moving
dot measures work completed.

The browser renders the braille curve once for the fading trail and bright head.
CSS moves clipped light windows over that fixed text in about 12 steps per
second. These are implementation choices, not a required rendering API or a
performance guarantee. The browser reserves space above the terminal while
preparation is pending; resizing uses the existing terminal layout and transport.
Animation frames remain local and do not rewrite text or send terminal or HTTP
updates. See the [measurement notes](browser-background-performance.md#video-preparation-activity-wave)
for the cost of the integrated indicator.

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

## Day activity overview

The [day activity navigator](day-activity-navigator.md) uses a dim Total curve
with independently toggled Event and Thought traces above the Events divider.
Its samples represent daily activity (or daily averages over wider ranges), not
probability. Keep its accepted-date marker, focus order and compact-height
behavior aligned with the daily timeline.
