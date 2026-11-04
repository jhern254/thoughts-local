# Goal Activity Lineage

Goal Details now includes **Activity History**: completed Events that earned
Goal time, followed by the relevant Thoughts recorded during each Event. Parent
Goals include activity earned by Goals beneath them. An Event appears once,
even when several Subjects or child Goals connect it to the requested Goal.

For example, a 90-minute Event assigned to Programming and Research may earn
time for two child Goals. Their parent shows one 90-minute Event. Expanding it
shows only Thoughts whose assigned Subject actually contributed to that parent
through either child, recorded within the Event's current time interval.

## Browsing

History starts with the newest 25 earned Events. Use arrows or `j/k` to move,
Enter to expand an Event, and Enter on a Thought preview to open the existing
Thought Details screen. That screen retains scrolling, reload, and `m` to change
Mindset. Back returns to the same selected preview and history page; its Mindset
badge reflects the change.

Tab/Shift+Tab switches between the Goal metadata viewport and history. The
metadata occupies at most one-third of the available Details body; it remains
scrollable for Goals with many Subjects. Mouse-wheel input scrolls the section
under the pointer. Clicking a history row selects and opens it.

Only one Event is expanded at a time. Its Thoughts are newest first, in pages of
50 bounded summaries. Select **Older/Newer Events** or **Older/Newer Thoughts**
when available. Escape collapses the expansion first, then returns to the Goals
list. `e` and `s` retain the existing independent property and Subject editors.
`r` refreshes history from the newest page; after a failed read it retries that
page. An expanded Event's `r` reloads its current Thought page and interval.

## Which Thoughts belong here?

The database requires both:

- `observed_at >= started_at` and `observed_at < ended_at` for the credited Event.
- A matching live Subject reference in that Event's `goal_progress`
  contributions to the requested Goal or one of its reachable descendants.

The Event start is included and its end is excluded. `thoughts.event_id` does not
need to be populated; this follows the Timeline's observation-time association.
Misc Thoughts, unrelated Subjects, deleted Thoughts, and another user's records
are excluded. A credited Event with no matching Thoughts still appears. A
zero-duration Event also appears, with zero time and no matching Thoughts.

Earned contributions remain authoritative after a Goal is deactivated or its
Subject membership is retired. Current `event_subjects` and `subject_goals` are
not used to rediscover historical eligibility. Event corrections and parent-edge
changes are reflected by subsequent reads; attribution-writing rules are unchanged.

Deleting a Subject retains its earned Event time but removes its ability to
match Thoughts here. A hard deletion detaches `goal_progress.subject_id`.
`recorded_subject_id` is historical identification only: it must never associate
Thoughts from a newly created Subject that reuses that number.

## Application and SQLite boundaries

`runtime.GoalProgress()` provides three read operations:

- `ListEarnedEvents`: a `GoalEventPageRequest` with an optional start-time/Event-ID
  cursor and Older/Newer direction returns bounded Event projections.
- `ListEventLineageThoughts`: the Event's current projection and a bounded
  `ThoughtSummaryViewRequest` page, using the existing observation-time,
  creation-time, and Thought-ID cursor ordering.
- `GetEventLineageThought`: the full Thought after rechecking its Goal, Event,
  Subject, time interval, ownership, and deletion state. Thought Details uses
  this same qualified read for reload.

A valid Goal with no earned Events returns an empty non-nil slice. A valid earned
Event with no matching Thoughts returns an empty non-nil Thought slice.
Unavailable Goals, Events, and selected Thoughts return the established not-found
error. Inactive or soft-deleted descendants can still contribute earned Events;
the requested Goal itself must be available.

Recursive SQLite CTEs follow the current parent hierarchy with `UNION`, and
Event/Thought identities remove duplicate paths. Short read transactions keep
availability checks and page reads consistent. Event projections contain at most
80 activity characters, and Thought previews reuse the existing bounded summary
reader. Full Thought bodies are fetched only on selection. No queries run during
rendering, and only the current Event page and expanded Thought page are retained.

The Event query first collects distinct earned Event IDs. Query-plan checks on
the synthetic stress database (721 Events, 20,000 Thoughts, 13,958 contribution
rows) showed that checking membership separately for each Event repeatedly
scanned contributions for empty Goals. Collecting earned IDs first avoided that
work using existing indexes; this feature adds no index or migration.

Pages reflect current data rather than a long-lived snapshot. Equal timestamps
use ID tie-breakers, but concurrent edits can move records across cursor bounds;
refresh restarts the Event history. Leaving Details, changing Goals, collapsing,
or replacing a read cancels its context and rejects late replies. Shared Thought
Details has its own existing request ownership and clears its qualified reader
when closed.

## Limits and verification

This is navigation over existing contributions, not Thought attribution. It adds
no persisted Thought–Goal relationship, progress writes, cadence calculations,
progress bars, historical backfill, or parent-management UI. Previous attribution
states cannot be reconstructed from the current contribution records.

The stress demo explicitly seeds synthetic earned contributions to make
pagination and Subject-matched Thought browsing inspectable. This fixture does
not change the application's policy against automatically backfilling old Events.

Behavior is covered by migrated SQLite tests, focused TUI tests, a root-TUI/
SQLite workflow, and the existing browser acceptance harness. Local browser
verification uses sandboxed Brave; hosted acceptance supplies Chromium and Firefox
coverage. Native and browser layouts are inspected at normal and narrow sizes.
