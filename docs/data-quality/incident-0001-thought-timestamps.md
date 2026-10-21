# Data quality incident 1: thought observation times recorded at save

## Status and scope

- Discovered: October 20, 2026, America/Los_Angeles.
- Evidence snapshot: October 21, 2026 at 06:34:09 UTC, from local `data/thoughts.db`.
- Code remediation: implemented in commit `427fdc9780e51dd8e6a3291e9a1a1e2655fbd238`, [PR #81](https://github.com/jhern254/thoughts-local/pull/81).
- Deployment: **unverified; PR #81 was open and draft at investigation time.** A commit, merge, or report date is not the deployment cutoff.
- Historical remediation: open. No historical records have been rewritten.
- Review population: **all 38 retained thoughts in the evidence snapshot**, including every user and assignment. This is the complete review population for that snapshot, not a sample and not proof that every timestamp is wrong.

New saves from an old application remain potentially affected. Rerun the extract
at actual cutover; the 38-record inventory below is not a permanent upper bound.
The issue's introduction date has not been established, so there is no lower
creation-date bound. Deleted or missing records outside this database cannot be
recovered by a query against it; examine retained backups separately if needed.

## What happened

The intended meaning of `observed_at` is when the user begins capturing a
thought: opening its draft through T, F8, or Create thought. The application
instead supplied an empty Go `time.Time` when saving. The shared service then
used its current clock for both the missing observation time and creation time:

```go
now := time.Now().UTC()
if observedAt.IsZero() {
    observedAt = now
}
```

This is a defaulting error at the UI/service boundary, similar in intent to a
fallback such as `COALESCE`, but **not a SQL COALESCE bug**. See
[thought creation](../../internal/thought/service.go) and the
[explicit timestamp insert](../../internal/data/sqlite_thought_store.go).
The schema's `DEFAULT (unixepoch('now'))` does not supply these values for this
application path: the insert supplies `observed_at` and `created_at` explicitly.

The save transaction is still the appropriate time to persist the record.
The [schema](../../migrations/000004_create_thoughts_table.up.sql) already separates
observation time from persistence timestamps. No schema change is required.
The investigation does not establish that save-time observation was an
intentional original product decision.

## Impact and evidence

Thoughts can appear later on the timeline by the time spent recording, typing,
choosing a subject, and retrying saves. That can move a thought across an event
or calendar-day boundary and change time-based counts and distributions.
The stored text and relationship IDs are not changed by this timestamp bug.
SQLite `PRAGMA quick_check` returned `ok`; that checks database structure, not
whether timestamps reflect the user's intended meaning.

The snapshot contains 38 rows, 38 distinct thought IDs, no soft-deleted thoughts,
and 38 rows with `observed_at = created_at`. All are version 1 and have
`updated_at = created_at`. Creation dates range from 2026-10-04T22:37:40Z through
2026-10-21T06:32:38Z. The query deliberately supports deleted and edited records
although this particular snapshot has none.

The three inspected SQLite backups under
`data/backups/master-data-upgrade-20261017T104812Z/` each contain 33 thoughts,
all with equal observation and creation timestamps. The operational log records
thought saves, not draft openings. Audio and recording start times are held in
memory, not retained as a recovery ledger. No independent exact opening time was
found. Recording start would also not necessarily equal draft opening for a
previously typed draft.

Snapshot SHA-256:
`a32c8ce14fbc496708b541c6378f2ee835d6dcc4a38adff076259058a0b56830`.
The private `snapshot.sqlite3`, full-text `review.csv`, and `manifest.json`
(recording capture time, hashes, count, and NULL cutoff) are retained locally
under `data/incidents/incident-0001/` in the primary checkout and excluded from Git. Archive those files in
private durable storage if this workspace is removed. The report commits only
the complete ID/timestamp inventory; authored text is available in the CSV and
through the query below. The initial 37-record snapshot and export are retained
as `initial-snapshot.sqlite3` and `initial-review.csv`; this report supersedes that
inventory after the data owner added thought 38. No thought timestamps changed.

### Complete snapshot inventory

Every row below has the same value for `observed_at`, `created_at`, and
`updated_at`; the UTC column renders that shared value. All 38 remain under
review, with original draft-opening time and error duration unknown.

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

## SQL for a complete review

Run [incident-0001-thought-timestamps.sql](incident-0001-thought-timestamps.sql).
It returns **every column of every matching thought, including its full text**,
plus UTC renderings of its four timestamp fields. The filename identifies the
incident; retain the chosen cutoff with the private export manifest.
It is a read-only SELECT. Run it on a consistent SQLite backup so concurrent
writes cannot change the population between export and reconciliation.

From a repository checkout, substitute the actual source database path:

```sh
mkdir -p /tmp/incident-0001-review
sqlite3 -readonly /absolute/path/to/data/thoughts.db \
  ".backup '/tmp/incident-0001-review/snapshot.sqlite3'"
sqlite3 -readonly -header -csv /tmp/incident-0001-review/snapshot.sqlite3 \
  < docs/data-quality/incident-0001-thought-timestamps.sql \
  > /tmp/incident-0001-review/thoughts.csv
sqlite3 -readonly /tmp/incident-0001-review/snapshot.sqlite3 \
  'SELECT COUNT(*) AS retained_thoughts, COUNT(DISTINCT thought_id) AS distinct_ids FROM thoughts;'
```

With no bound cutoff, all snapshot rows are returned. Reconcile the CSV's parsed
record count and unique thought IDs against the counts above; do not count CSV
lines, because authored thoughts may contain newlines. The captured incident
extract reconciles to **38 records and 38 distinct IDs**. Keep full-text exports
local rather than adding them to operational logs or the Git repository.

### After a verified deployment

This procedure produces a review file containing thoughts that may have been
saved by the old application. It does not repair their timestamps. Use a
snapshot of the database so the records being reviewed cannot change while you
validate and export them.

Here, **cutover** means the transition from the old application to the fixed
application for one particular database. A **writer** is any running process
that can save to that database, such as the TUI, browser server, API, or an import
command. Merging the PR does not stop those processes or replace their running
code. Opening another browser tab also does not restart the server behind it.

There is no verified cutover timestamp recorded for this incident yet. Until
that evidence is available, use a NULL cutoff and review the complete snapshot.

#### 1. Choose the review scope

| Database snapshot available | Cutoff to use | What the export contains |
| --- | --- | --- |
| Final snapshot taken after all old writers stopped, before fixed writers began saving | `NULL` | Every retained thought at the end of the old application's operation. This is the preferred incident inventory. |
| Later snapshot containing both old and new saves, with a documented cutover | Verified integer Unix timestamp | Thoughts whose `created_at` is at or before the cutover second. |
| Snapshot with uncertain deployment history, overlapping writers, imports with preserved creation times, or clock changes | `NULL` | Every retained thought for manual review; a time filter cannot reliably separate the two populations. |

A Unix timestamp is the number of seconds since 1970-01-01 00:00:00 UTC. This
query expects **seconds**, not milliseconds or a formatted date string. The
cutoff describes the deployment boundary; it is not the time of the oldest or
newest thought and is not the time you happen to run the review.

#### 2. Establish and record the deployment boundary

If you are carrying out the deployment, perform these actions in order:

1. Identify the exact database file and every process that can write to it.
   Resolve the configured database path; do not assume two processes started
   from different directories use the same `data/thoughts.db`.
2. Stop accepting new entries through the old application. Let saves already
   submitted finish, and confirm all old writer processes have exited. Pending
   background work must finish before the database connection closes. An
   unsaved draft is not a database record; let its owner save or explicitly
   discard it before shutdown. Do not assume a browser draft survives a restart.
3. While all writers are stopped, make the final snapshot using step 3 below.
   Exporting every row of this snapshot with a NULL cutoff gives the complete
   retained population without relying on historical clock values.
4. Rebuild from a revision containing the fix and record the revision and
   executable being deployed. Immediately before starting the fixed writers,
   record the UTC time and its Unix value. Confirm that every restarted writer
   uses the intended executable and database. This recorded boundary is usable
   only if no old writer can resume saving afterward.
5. Verify the running application with a deliberate delayed-save check: open a
   test draft, wait several seconds, and save it. Its observation time should
   reflect opening, while its creation time should reflect saving. Identify the
   test record in the deployment notes so it is not mistaken for historical data.

On the deployment host, this terminal command records both time representations
from one clock reading. Run it at the boundary described above, not later when
preparing the report:

```sh
sqlite3 -header -column :memory: \
  "SELECT unixepoch('now') AS cutover_unix, datetime('now') AS cutover_utc;"
```

Retain the output alongside the absolute database path, source revision,
executable path and hash, confirmation that old writers stopped, snapshot path
and hash, and the delayed-save result. Add that deployment evidence to this
report. A valid date conversion alone cannot establish which software was
running. If the transition already happened without adequate records, do not
invent a cutoff from the merge date or the current time; use the full extract.

#### 3. Create a private snapshot and open it read-only

The following commands run in your normal terminal on Linux, from the repository
root. Replace the source path with the database identified in step 2. `mktemp`
creates a new directory for this review so an earlier snapshot is not replaced.
For the preferred final snapshot, run these while all writers are stopped. For
a later review, the same SQLite backup command gives a consistent copy, but it
will also contain records saved since deployment.

```sh
incident_source_db="/absolute/path/to/data/thoughts.db"
incident_review_dir="$(mktemp -d /tmp/incident-0001-review.XXXXXX)"
sqlite3 -readonly "$incident_source_db" \
  ".backup '$incident_review_dir/snapshot.sqlite3'"
sha256sum "$incident_review_dir/snapshot.sqlite3"
printf '%s\n' "$incident_review_dir"
sqlite3 -readonly "$incident_review_dir/snapshot.sqlite3"
```

Keep the printed directory path and hash. `/tmp` is temporary storage; archive
the evidence in private durable storage before it is cleaned up. Use SQLite's
backup command rather than copying a live `.db` file alone: recent committed
changes may still be in a separate write-ahead log.

The last command opens the SQLite shell and displays a `sqlite>` prompt. The
commands in steps 4 and 5 belong at that prompt. Commands beginning with a dot
are SQLite shell commands; SQL statements end with a semicolon. Stay in this
same session so the parameter you validate is the one used for export.

#### 4. Set the cutoff and validate it before exporting

Start with the complete-population setting:

```text
.parameter init
.parameter set :fixed_writers_started_at_unix NULL
.headers on
.mode column
```

For the complete snapshot, leave `NULL` as written. For a later snapshot with a
verified cutover, replace only `NULL` in the `.parameter set` command with the
positive integer from the deployment record, then execute that command. Do not
paste a date string, a millisecond value, or an illustrative timestamp.

Inspect the value SQLite will actually use:

```sql
SELECT
    typeof(:fixed_writers_started_at_unix) AS cutoff_type,
    :fixed_writers_started_at_unix AS cutoff_value,
    datetime(:fixed_writers_started_at_unix, 'unixepoch') AS cutoff_utc;
```

For a complete extract, `cutoff_type` must be `null`; the other two values will
be NULL, normally displayed as empty cells. For an extract limited by cutover,
require all three conditions:

- `cutoff_type` is `integer`.
- `cutoff_value` is positive and matches the recorded Unix seconds.
- `cutoff_utc` is nonempty and matches the recorded UTC deployment time.

If any condition fails, stop and correct the parameter before continuing. The
extraction query does not validate its input. In particular, a text value can
produce misleading comparisons instead of an error.

Next, check that stored creation times can be compared to that cutoff:

```sql
SELECT thought_id, created_at
FROM thoughts
WHERE typeof(created_at) <> 'integer'
   OR datetime(created_at, 'unixepoch') IS NULL
ORDER BY thought_id;
```

**Zero rows is the required result before using a numeric cutoff.** No output
means this check found no malformed creation times. If rows appear, retain their
IDs for investigation and reset the parameter to NULL for a complete extract:

```text
.parameter set :fixed_writers_started_at_unix NULL
```

Rerun the parameter inspection after changing it. The incident snapshot passed
this creation-time check with zero rows. Passing establishes usable types and
date representations; it does not prove the historical clocks were correct.

#### 5. Export and reconcile the review file

Only after the checks above, run these commands in the same SQLite session.
Replace the `.once` destination with a file named `thoughts.csv` inside the
private directory printed in step 3. The `.read` path is relative to the
repository root where you launched SQLite.

```text
.headers on
.mode csv
.once /absolute/path/to/private-review/thoughts.csv
.read docs/data-quality/incident-0001-thought-timestamps.sql
```

`.once` sends the next command's output to that file. Keep it immediately before
`.read`; running a validation query between them would put the validation output
in the CSV instead of the thoughts. The export includes authored text, so retain
it with the private evidence rather than committing it to Git.

After the export, check the expected population using the same cutoff:

```text
.mode column
```

```sql
SELECT
    COUNT(*) AS expected_records,
    COUNT(DISTINCT thought_id) AS expected_distinct_ids
FROM thoughts
WHERE :fixed_writers_started_at_unix IS NULL
   OR created_at <= :fixed_writers_started_at_unix;
```

Use a CSV-aware reader to compare the exported record count and unique thought
IDs with these results. A line count is insufficient because a thought can span
multiple lines. Record the chosen cutoff (including NULL), snapshot hash,
export hash, and reconciled counts with the evidence. The captured incident
snapshot reconciles to 38 records and 38 distinct IDs; a later snapshot may have
more. Type `.quit` to leave SQLite when finished.

The query includes `created_at <= cutoff`, so saves in the cutover second remain
in scope. SQLite stores these timestamps at whole-second precision; a fixed
writer's first save in that same second may therefore be included for review.
`created_at` is the service's save-time clock value, not a database commit
sequence. If an old writer saved after the recorded boundary, or the clock or
import history makes that comparison unreliable, use the complete extract and
review the deployment evidence again. Inclusion identifies a review candidate,
not a confirmed incorrect timestamp.

### Why the query does not miss the usual historical cases

| Case | Review behavior |
| --- | --- |
| Equal observation/creation timestamps | Included if in scope; same-second valid captures and API defaults can also be equal. |
| Different observation/creation timestamps | Included if in scope; equality is never a selection predicate. Explicit API/import times need provenance review. |
| Observation time before or after the cutoff | Scope follows creation time, not observation time. |
| Thought edited or deleted after cutover | Included based on original creation time, regardless of `updated_at` or `deleted_at`. |
| Assigned, unassigned, or event-linked thought | Included; no relationship filter or join can remove it. |
| Another user or soft-deleted owner | Included; this is an incident inventory across the database, not the application's active-user list. |
| Creation in the cutoff second | Included conservatively. |
| Malformed creation timestamp | Preflight must return no cases before bounding. Otherwise use the full extract and review the anomalies. |
| NULL cutoff | All rows retained. |
| Invalid cutoff | Reject during preflight; the extraction query does not validate it. |
| Post-cutover creation with a backdated observation | Excluded only when the cutoff is verified; assess later API/import problems separately. |

Neither `observed_at = created_at`, a maximum ID, nor the PR merge time proves
which writer created a record. The database does not store capture source or
application build per thought. Consequently SQL can capture the full review
population, but cannot identify the exact harmed subset or reconstruct its
missing times. Do not describe every candidate as confirmed incorrect.

The [SQLite regression test](../../integration/data_quality_incident_workflow_test.go)
executes this exact SQL against real migrations. It asserts the complete ID set
for NULL and valid integer cutoffs, including equal/different timestamps,
other/deleted users, deleted thoughts, linked thoughts, later edits, boundary
seconds, and post-cutover backdating. Invalid input is outside the extraction
query's contract and must be rejected by the preflight checks above.

## Applied code fix and validation

The draft now captures its observation time synchronously on entry. Saving
passes that stored value to the service. Recording restarts, subject selection
or creation, and failed-save retries retain it. Canceling and opening a new
draft captures a new time; queued saves retain their own draft's time.
`created_at` and `updated_at` remain persistence timestamps. The API continues
to accept an explicit RFC3339 `observed_at`; omission still defaults to service
call time, so clients capturing earlier must supply it.

Event start/end forms already captured their default time on entry and permit
manual correction. Regression coverage was added for delayed event-end saves.
For the fix commit, `make quick`, `make ci`, and the required pre-push checks
passed. A synthetic native TUI walkthrough at 100×30 opened a thought at
2026-10-21T05:55:44Z and saved at 05:55:56Z: both SQLite and the detail screen
retained 05:55:44Z as observation time. Physical microphone capture was not
manually exercised; voice lifecycle and root dispatch have automated coverage.
These results validate the code fix, not deployment or historical repair.

## Historical disposition and data quality governance

Keep original historical values unless independent evidence supports a
correction. Do not subtract an assumed recording duration, copy an event start,
or use `COALESCE(observed_at, created_at)` as a repair: none recovers the missing
opening time. A known start time can support a correction; a user estimate must
remain explicitly labeled as estimated rather than becoming apparent fact.

For any approved correction, first preserve a consistent backup and a private
change manifest recording database identity, incident ID, thought ID, old/new
observation times, evidence, confidence, reviewer, and correction time. Apply
reviewed changes atomically with expected-version/old-value checks, advance the
version and keep `updated_at` nondecreasing, and preserve `created_at`. Reconcile
changed IDs and rerun affected timeline reports. This report does not execute
or prescribe an unreviewed bulk UPDATE.

| Action | Responsible role | Completion evidence / current state |
| --- | --- | --- |
| Define each domain time and its capture action separately from persistence time. | Feature author and reviewer | Draft-entry semantics documented here and in the browser audio guide. Require this decision for future capture features. |
| Test time meaning across UI, service, and SQLite boundaries. | Feature author and reviewer | Current fix covers entry paths, retries, restart, and cancellation. Keep delayed-save and boundary tests when adding surfaces. |
| Record actual cutover and stop old writers. | Repository maintainer / app operator | Pending: deployed build, UTC boundary, final snapshot/hash, and post-restart smoke test. |
| Preserve and reconcile the complete incident population. | Incident reviewer | 38/38 snapshot records exported and inventoried; rerun at cutover and retain the private artifacts. |
| Classify historical cases as unknown, independently confirmed, corrected, or estimated. | Data owner and incident reviewer | Pending individual review. Preserve uncertainty when evidence is absent. |
| Review clients that omit observation time and future import contracts. | API/client maintainer | Explicit input is already supported. Defaulting behavior must be an intentional documented choice at each capture boundary. |
| Consider persistent capture provenance if exact audits become a requirement. | Repository maintainer | Proposed separate scoped change: source/build and evidence requirements, retention, and privacy. Do not add authored content or audio to operational logs. |
| Close code remediation separately from historical data remediation. | Incident reviewer | Deployment is unverified; historical accuracy remains unresolved. Closure must state which uncertainty is accepted. |

Structural database checks and a high rate of equal timestamps cannot measure
semantic timestamp accuracy on their own. Future quality reviews should check
capture contracts and provenance, distinguish confirmed defects from review
candidates, and keep incident evidence reproducible without silently rewriting
history.
