# Mindset thoughts

Press **m** on a saved thought in a list or its detail screen to mark or unmark
it as a mindset. A white **M** badge indicates that it is currently marked.
The shortcut does not intercept typing in the editor or search filter.

The **Mindset** panel sits above Events, with a white title block, black text,
and a separator line. The complete selected thought appears beside the title;
an empty day shows `...` in that position, with purple picker styling when focused. Tab
and Shift+Tab move between Mindset, Events, and the entity strip. Mindset
always expands its scrolling list to the right of the title while focused;
moving focus away shows only the selected thought, starting at its beginning.
Enter opens the selected thought. Up/Down reads through a tall thought before
selecting the next or previous thought; Page Up/Down performs a page of those
steps. Navigation wraps around at either end. Home/End select the beginning of
the first thought or the end of the last thought. Left and Q leave the picker
and its cursor unchanged.
Esc opens the app exit prompt; canceling it preserves the picker. Returning
from a thought restores the panel's selection and scroll position.

The divider above Events or the entity strip turns purple when that section
has focus. Mindset uses its picker highlight to show focus.

When focus leaves Mindset, the selected thought stays in the panel
for 15 seconds before cycling to the next thought. Rotation pauses
while focused or offscreen.

Thoughts wrap with a small right margin and retain their original paragraph
breaks. The panel grows to fit up to twelve text lines, with fewer lines on short
terminals to leave room for Events and the tabs. Longer thoughts remain fully
readable by scrolling while focused; there is no preview-length limit. A blank
line separates Mindset from the Events divider. Focus changes and returning
from a thought preserve the picker's reading position. Press **r** to refresh.

## What a historical day means

The panel includes each thought that was a mindset at any time during the
selected Events day, using the application's Pacific timezone. It orders thoughts
by the start of their latest designation period overlapping that day, oldest
first. A thought marked again later that day moves later in the list.

Unmarking removes the current M badge but does not remove the thought from that
day's history. Historical panels display the current thought text; designation
history does not preserve past versions of the text. Deleted thoughts and owners
are hidden, while their designation history remains stored.

Marking and unmarking record the time the user presses m, even while browsing a
past day. These actions do not change the thought's observation time or content
revision. The existing `thought_mindset_periods` table stores each marking as a
new period and each unmarking as its end; there is no additional stored flag.
Repeated requests for the same state do nothing. Actions preceding the latest
transition are rejected rather than silently moving timestamps.

Day ranges exclude the following midnight; a period ending at midnight belongs
to the preceding day. A mark and unmark within the same stored second still
appear on their action day. Where periods begin in the same second, their IDs
provide a stable ordering.
