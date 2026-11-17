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

History opens at the newest earned Events. Use arrows or `j/k` to move,
Enter to expand an Event, and Enter on a Thought preview to open the existing
Thought Details screen. That screen retains scrolling, reload, and `m` to change
Mindset. Back returns to the same selected preview and history page; its Mindset
badge reflects the change.

Activity History shows Event and Thought times as `HH:MM AM/PM` in the same
Pacific display timezone as Events. Event rows retain their date; seconds remain
stored but are omitted from these history labels.

Tab/Shift+Tab switches between the Goal metadata viewport and history. The
metadata occupies at most one-third of the available Details body; it remains
scrollable for Goals with many Subjects. Mouse-wheel input scrolls the section
under the pointer. Clicking a history row selects and opens it.

A shared-style divider separates metadata from history and follows the border
weight preference. It turns purple while Activity History is focused. A Goals block appears above the Goal name. Within metadata,
Up/Down or j/k selects the Goal settings/dates group or one Subject at a time;
only that selection gets the bright highlight. The wheel and mouse can also
select Subjects, and wrapped selections scroll into view. Home/End reaches the
first/last item. Tab remembers the selected item when switching panels, with a
muted selection while history is focused. Headers and Created/Updated timestamps
are not selectable; the Goals and Subjects headings retain their block styles.

Only one Event is expanded at a time. Its Thoughts are newest first, in pages of
50 bounded summaries. **Older/Newer Thoughts** loads another Thought batch.
Event paging is automatic: arrows, Page Up/Down, and the mouse wheel continue
through adjacent Event windows without selectable loading rows. Each window
loads up to ten screenfuls of Event summaries, capped at 250 Events. The dots
show at most ten pages at a time; continuing forward resets to the first dot,
and returning backward selects the last page of the newer window. Expansion
and resizing can change the number of display pages without loading more data.
Left (or `h`) pages toward newer Events. At the newest page, a fresh Left acts
like Escape: collapse an expanded Event first, then return to the Goals list.
With metadata focused, Left also acts as Back. Held Left stops at this boundary;
release and press again to go back. Escape remains available from any page.
`e` and `s` retain the existing independent property and Subject editors.
`r` refreshes history from the newest page; after a failed read it retries that
page. An expanded Event's `r` reloads its current Thought page and interval.

Loading feedback appears only if a read is still pending after 150 ms, matching
Events. Reads start immediately and completed results appear immediately. The
reserved status row stays in place, and existing Events remain visible while
paging. Errors show retry guidance immediately. An expanded Event with no
matching Thoughts stays visible without an additional empty-Thought message.

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
  cursor, Older/Newer direction, and optional limit returns bounded Event
  projections. The default is 25; larger display windows are capped at 250.
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

The Event query first restricts contributions to the requested Goal and its
reachable descendants, then matches Events against those IDs with `IN`.
This uses the existing `(user_id, goal_id, event_id)` index instead of scanning
every contribution for the user before filtering Goals. The ID membership is
computed once, so older/newer cursor paging does not repeatedly join and scan
it for each Event. Matching IDs also keeps an Event unique across Subjects and
parent paths. This feature adds no index or migration.

Thought reads collect the Event's contributing Subject IDs once, then use
membership in that set to filter the time-bounded Thought query. A correlated
contribution check previously scanned a Subject's contributions again for each
Thought. The set query keeps the same live-identity and ownership rules without
that repeated work. Preview and full Thought reads use the same condition.

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

## Reproducing performance measurements

Use a disposable migrated database loaded with `scripts/demo_stress.sql`.
Compile the opt-in benchmarks separately, then run without other builds or tests:

```sh
go test -c -tags=integration -o /tmp/goal-activity-bench.test ./internal/tui/goals
THOUGHTS_STRESS_DSN=file:/tmp/your-stress-demo.db /tmp/goal-activity-bench.test \
  -test.run '^$' -test.bench '^BenchmarkGoalActivity' -test.benchtime=30x -test.count=3
```

The read benchmarks cover initial, older, newer, maximum-sized, and empty Event windows,
and Thought expansion. Presentation benchmarks measure rendering, local movement,
and expansion at 100×35 and 48×24 terminal cells. The benchmark completes data
commands directly; loading-feedback timing is tested separately. Database setup,
fixture selection, and compilation are outside the timed loops.

## Measured comparison — November 16, 2026

Reference machine: ThinkPad T480s, Intel i5-8250U, Linux. Database fixtures were
synthetic and disposable: 721 Events / 20,000 Thoughts / 13,958 contributions,
and a larger copy with 7,201 Events / 200,000 Thoughts / 139,580 contributions.
Six parent edges formed a diamond for hierarchy checks. The larger fixture
repeated completed history at separate older intervals; it kept one ongoing
Event. Neither fixture changed personal app data.

Compiled Go benchmarks ran sequentially with no other builds or tests. Each
case used three runs of 30 warm operations; the table reports the median of the
three run means. These are complete service reads, including transactions and
ownership checks, rather than isolated SQL statements.

| Service read | Stress before → after | Larger before → after |
|---|---:|---:|
| First 80 Events | 3.40 → 1.10 ms | 29.94 → 5.06 ms |
| Older 80 Events | 3.06 → 1.11 ms | 29.46 → 5.16 ms |
| Maximum 250 Events | 3.93 → 1.41 ms | 30.64 → 5.93 ms |
| Empty Goal | 3.19 → 0.68 ms | 25.47 → 1.84 ms |
| Thought expansion | 42.72 → 1.65 ms | 43.31 → 5.79 ms |

Newer-window reads also measured 1.11 ms and 5.04 ms after the changes. The
benchmarks caught and rejected an intermediate join-based rewrite: it improved
initial reads but made older cursor reads slower. The final Event-ID membership
query improved both directions using the existing indexes.

Browser measurements used installed sandboxed Brave with a clean context and
static backgrounds. Test-only instrumentation observed keydown, terminal-data
arrival, and xterm's matching render callback. Each action had 30 samples per
viewport; local movement had 60. Values below are median / p95 key-to-render
time, not physical display or GPU presentation latency.

| Browser action | 1200×850 before → after | 480×420 before → after |
|---|---:|---:|
| Local movement | 12.3 / 26.5 → 14.2 / 29.8 ms | 15.5 / 41.3 → 13.1 / 42.0 ms |
| Expand Event | 78.7 / 89.1 → 31.2 / 46.9 ms | 82.8 / 95.1 → 23.5 / 61.2 ms |
| Open Details | 29.4 / 30.9 → 17.3 / 47.8 ms | 18.7 / 50.2 → 29.6 / 32.4 ms |
| Return to Goals | 31.3 / 57.5 → 32.5 / 64.4 ms | 34.9 / 65.6 → 55.9 / 64.3 ms |

Crossing Event windows in either direction measured 17–18 ms median afterward,
versus about 20 ms before. Tail timings varied: the normal-size older-window
p95 increased from 27.7 to 45.0 ms, while narrow-size older-window p95 fell from
50.4 to 42.9 ms. Unrelated list-return and rendering work was not optimized;
these samples do not establish an across-the-board navigation speedup.

Before the change, loading feedback appeared in all 60 expansion samples and
34 of 60 Details openings. Afterward it appeared in none of those samples or
the 120 window-crossing samples. Each browser measurement session retained one
WebSocket connection. Go rendering/local movement stayed around 1–2 ms with
broadly unchanged allocation counts. The measured gains are less SQL work and
less transient feedback; no browser-memory or whole-app CPU reduction is claimed.

The read/presentation baseline includes the feedback cleanup but precedes both
query changes. The browser baseline precedes all three changes. Warm caches,
headless rendering, frame scheduling, and normal desktop activity limit these
comparisons; these are local observations, not guarantees for every device or
video background.
