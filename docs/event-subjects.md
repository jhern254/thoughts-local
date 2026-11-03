# Event Subjects

An Event is one focused interval with zero or more live Subject assignments.
`event_subjects` is the assignment source of truth. Each same-owner pair is
retained once; removing it sets `deleted_at`, and relinking clears that value.
No live assignments means Misc. Names such as Misc remain ordinary Subject names.

Create, CreatePast, and Update accept an unordered Subject-ID set. Duplicate IDs
are rejected. Nil and empty sets explicitly clear assignments on Update. A caller
correcting only an activity label or timestamps must supply the current set.
Event and relationship writes share one SQLite transaction; stale versions and
invalid selections cannot partially change the interval or its assignments.
Ending an Event and closing its predecessor retain their Subject sets.

Subject deletion retires its Event links and advances affected Event versions
once, including retained deleted Events, without changing their content, times,
Thought provenance, or other Subject assignments. Thought assignments retain
their existing single-Subject deletion behavior.

Subject-time totals overlap: a 90-minute Event assigned to two Subjects contributes
90 minutes to each. This still represents only 90 minutes of distinct activity.
Never sum Subject totals to calculate unique activity time. Existing day/range
clipping and ongoing cutoffs apply independently to each live association.

Goal discovery returns the deduplicated union of eligible active Goals across
live Event Subjects. An unassigned Event has no candidates. This does not record
progress or introduce direct Event–Goal assignments.

Future automatic attribution must deduplicate by Event ID at every Goal,
including parent Goals reached through several child paths. Matching the same
Goal through two Subjects must not credit it twice; summing child credits must
not multiply a parent's elapsed time.

Development databases are disposable. Migration 3 defines Events without the
old Subject column; migration 15 separately creates `event_subjects`. There is
no compatibility column or automatic conversion of older databases.
Recreate disposable databases using the normal migration/demo commands. Startup
does not run migrations implicitly. SQL migrations remain authoritative.

## Start Event picker

The Subjects action opens a picker within the existing Events component. Arrows
move through Subjects, Space toggles a check, and `/` enters search mode so spaces
can be typed in a query. Enter leaves search mode, confirms the checked set, or
opens Create subject… when that action owns focus. Escape restores the selection
from before the picker visit. Ctrl+S saves the Event with the current checked set.
Search text is never an assignment. Subject creation returns to this picker with
prior checks preserved and the new Subject checked. End Event requires no Subject
selection. Existing Event editing and historical creation remain service APIs.

Collapsed cards keep a single compact Subject preview alongside the Thought
count, showing up to three names and `...` for additional Subjects or clipped
names. Expansion initially shows the same three-name preview on one width-bounded row.
Up from the first Thought focuses that row; Left/Right or h/l browses every
assigned Subject without additional reads. The selected Subject stays visible
at narrow widths. Down returns to Thoughts; Esc/Q collapses the card. Left still
collapses when Thoughts own focus. Additional names are indicated with `...`.
The preview reuses the existing row to preserve card height and the timestamp rail.
Subject display reads use a single SQLite statement per day, rather than a
Subject lookup for each Event. Expanded cards revalidate their live Subjects
within the existing owned expansion command.
