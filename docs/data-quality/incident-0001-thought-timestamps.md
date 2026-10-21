# Data quality incident 1: thought observation times recorded at save

Discovered October 20, 2026. Fix implemented in [PR #81](https://github.com/jhern254/thoughts-local/pull/81),
commit `427fdc9780e51dd8e6a3291e9a1a1e2655fbd238`. Deployment was unverified at
the time of this report. Historical timestamps have not been changed.

## What is wrong with the data

A thought's `observed_at` should represent when the user opens its draft through
T, F8, or Create thought. Previously, the UI passed an empty observation time
when saving, and the [thought service](../../internal/thought/service.go)
substituted its current time. This was a Go defaulting issue, not SQL `COALESCE`.
The database already had separate fields for observation and persistence times.

Affected thoughts therefore appear later than intended by the time spent typing,
recording, choosing a subject, or retrying a save. They may fall into the wrong
event interval or calendar day, affecting timeline views and time-based counts.
The note text and stored relationships remain intact. This is a historical data
quality issue; SQLite's structural integrity check passed.

At the October 21, 2026, 06:34:09 UTC review, the local database contained
**38 thoughts, IDs 1–38**, all with equal observation and creation timestamps.
All 38 are in the review population. Equality alone does not prove that a row
is wrong: a valid same-second capture or an explicit API timestamp can also
produce equal values. Other rows created by the old application may be affected
if it continues running after this review.

## Fix

The draft now captures `observed_at` when it opens and preserves it through
recording restarts, subject selection, and save retries. Canceling and opening
a new draft captures a new time. The record is still written when the user saves:
`created_at` and `updated_at` remain persistence timestamps.

Event start/end forms already captured their default times when opened. API
clients can supply an explicit `observed_at`; clients that omit it still receive
the service-call time and must supply an earlier capture time themselves.

The fix passed `make ci`, targeted regressions, and a manual delayed-save check.
It takes effect when the fixed application is built and restarted, not when the
PR is merely merged.

## Reviewing potentially affected thoughts

Use the approved [read-only SQL](incident-0001-thought-timestamps.sql). It returns
full thought records and readable UTC timestamps, including deleted thoughts,
every user, and thoughts edited after their original creation.

Leave `:fixed_writers_started_at_unix` NULL to review all records. If the database
contains later saves and the actual deployment time is known, supply that time
as a positive integer Unix timestamp in **seconds**. The query then includes
`created_at <= cutoff`, retaining the boundary second. Use the time when all old
application processes had stopped and the fixed application took over, rather
than the merge date or the time the query is run.

The query assumes valid parameters and integer creation timestamps. Check those
before applying a numeric cutoff. If deployment history, timestamp values, or
clock changes make that boundary uncertain, leave the cutoff NULL. Do not use
`observed_at = created_at` as the filter: it cannot reliably identify impact.
The [SQLite regression tests](../../integration/data_quality_incident_workflow_test.go)
cover these selection rules, including deleted records, later edits, and the
cutoff second.

## Historical data and lessons

The original draft-opening times were not retained, and the investigation found
no independent evidence to recover them exactly. Keep existing timestamps unless
reliable evidence supports an individual correction. Do not subtract an assumed
recording duration or replace a thought's time with an event's start time.
For now, the historical uncertainty is documented rather than reconstructed.
Further application development does not depend on retaining database copies
or reconstructing these historical timestamps.

For future features:

- Define what each timestamp means and which user action captures it.
- Keep observation time separate from database creation and update times.
- Test delayed saves, retries, and cancellation so persistence cannot silently
  replace the original capture time.
- Document known data limitations; make corrections from evidence rather than
  guesses, preserving original creation times.

## Records reviewed

These are all 38 records reviewed at the time above. Each listed value was shared
by `observed_at`, `created_at`, and `updated_at`. Their original opening times and
individual error durations are unknown. The SQL can be rerun for a current list;
this inventory is a record of the review, not a permanent limit on the incident.

| Thought ID | Stored epoch seconds | Stored time (UTC) |
| --- | --- | --- |
| 1 | 1791153460 | 2026-10-04T22:37:40Z |
| 2 | 1791155489 | 2026-10-04T23:11:29Z |
| 3 | 1791295766 | 2026-10-06T14:09:26Z |
| 4 | 1791335918 | 2026-10-07T01:18:38Z |
| 5 | 1791339534 | 2026-10-07T02:18:54Z |
| 6 | 1791340777 | 2026-10-07T02:39:37Z |
| 7 | 1791347050 | 2026-10-07T04:24:10Z |
| 8 | 1791358810 | 2026-10-07T07:40:10Z |
| 9 | 1791358916 | 2026-10-07T07:41:56Z |
| 10 | 1791359044 | 2026-10-07T07:44:04Z |
| 11 | 1791359292 | 2026-10-07T07:48:12Z |
| 12 | 1791360261 | 2026-10-07T08:04:21Z |
| 13 | 1791400672 | 2026-10-07T19:17:52Z |
| 14 | 1791437544 | 2026-10-08T05:32:24Z |
| 15 | 1791438489 | 2026-10-08T05:48:09Z |
| 16 | 1791454637 | 2026-10-08T10:17:17Z |
| 17 | 1791519274 | 2026-10-09T04:14:34Z |
| 18 | 1791522259 | 2026-10-09T05:04:19Z |
| 19 | 1791523262 | 2026-10-09T05:21:02Z |
| 20 | 1791523306 | 2026-10-09T05:21:46Z |
| 21 | 1791523373 | 2026-10-09T05:22:53Z |
| 22 | 1791523429 | 2026-10-09T05:23:49Z |
| 23 | 1791523650 | 2026-10-09T05:27:30Z |
| 24 | 1791524425 | 2026-10-09T05:40:25Z |
| 25 | 1791526059 | 2026-10-09T06:07:39Z |
| 26 | 1791592565 | 2026-10-10T00:36:05Z |
| 27 | 1791602382 | 2026-10-10T03:19:42Z |
| 28 | 1791610710 | 2026-10-10T05:38:30Z |
| 29 | 1791614793 | 2026-10-10T06:46:33Z |
| 30 | 1791616284 | 2026-10-10T07:11:24Z |
| 31 | 1791619492 | 2026-10-10T08:04:52Z |
| 32 | 1791620544 | 2026-10-10T08:22:24Z |
| 33 | 1791870781 | 2026-10-13T05:53:01Z |
| 34 | 1792235939 | 2026-10-17T11:18:59Z |
| 35 | 1792559548 | 2026-10-21T05:12:28Z |
| 36 | 1792561031 | 2026-10-21T05:37:11Z |
| 37 | 1792561610 | 2026-10-21T05:46:50Z |
| 38 | 1792564358 | 2026-10-21T06:32:38Z |
