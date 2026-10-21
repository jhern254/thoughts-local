# Mindset thoughts

Press **m** on a saved thought in a list or its detail screen to mark or unmark
it as a mindset. A white **M** badge indicates that it is currently marked.
The shortcut does not intercept typing in the editor or search filter.

The **mindset** panel sits above Events. Tab and Shift+Tab move between Mindset,
Events, and the entity strip. Enter expands the panel, then opens the selected
thought. Arrow keys and Page Up/Down navigate; Home/End select the first/last
thought. Left collapses. Q/Esc collapses first, then returns focus to Events.
Returning from a thought restores the panel's selection and scroll position.

Collapsed, the panel cycles through previews every 15 seconds. Rotation pauses
while focused, expanded, or offscreen. Expanded, it shows up to five text rows,
with fewer rows on short terminals to leave room for Events. Open a thought to
read its complete text. Press **r** to refresh.

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
