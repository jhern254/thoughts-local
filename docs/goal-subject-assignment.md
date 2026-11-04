# Goal–Subject assignment

`subject_goals` is the only source of Goal membership. A Goal may have zero,
one, or many live Subjects. **Unassigned** means zero live Subjects; the future
Goals UI will display **Misc**. This classification is computed, not stored.
No migration, sentinel Subject, or mandatory association is needed.

Live memberships have `subject_goals.deleted_at IS NULL`, undeleted Subject and
Goal endpoints, and an available same-owner user. Names do
not determine classification: user Subjects named `Misc` or `Unassigned` work
like any other Subject.

## Application APIs

Use `runtime.GoalAssignments().Create(ctx, userID, input, subjectIDs)` to create
a Goal and all its requested links atomically. It uses the ordinary Goal
service's normalization, validation, defaults, and clock. An empty selection is
valid. Duplicate IDs produce a validation error; missing, foreign-owned, or
deleted selected Subjects produce `data.ErrRecordNotFound`. Every write rolls
back if any required step fails. Creation returns the persisted Goal at version 1.
Standalone `runtime.Goals().Create` remains available with its existing contract.

Use `runtime.GoalAssignments().ReplaceSubjects(ctx, userID, goalID,
expectedVersion, subjectIDs)` to replace the live set. All selected Subjects
are validated before links are removed. Links to soft-deleted Subjects remain
as history. An empty selection retires all live links without deleting the Goal.
The returned Goal supplies the version for the next edit.

Use `runtime.SubjectGoals().ListLiveSubjectsForGoal(ctx, userID, goalID)` for
current membership, including canonical Subject names. Results are ordered by
Subject ID. An accessible Goal with no live Subjects returns a non-nil empty
slice; a missing, foreign-owned, or soft-deleted Goal, or an unavailable owner,
returns `data.ErrRecordNotFound`. Inactive Goals remain accessible. The read uses
one SQLite snapshot for Goal visibility and membership.

`ListForGoal`/`ListForSubject` and the store's raw `ListGoalSubjects`/
`ListSubjectGoals` retain their historical-link behavior, including retired relationships and soft-deleted endpoints. Each raw record
includes `GoalNumber` and nullable `DeletedAt`. They are not current-membership
queries.

## Versions and transactions

A replacement requires the current Goal version even if it requests the same
set. A changed live set increments the version exactly once and keeps
`updated_at` nondecreasing. An identical set, regardless of order, preserves
both version and timestamp. Stale callers receive `data.ErrVersionConflict`
without changing membership.

Standalone Subject–Goal `Add` and `Remove` retain their signatures. Each
successful link change advances the Goal version once in the same transaction.
`Add` inserts a new numbered relationship or reactivates a retired pair with its
original number. An active pair returns `ErrDuplicateRecord`, retaining the
SQLite primary-key cause. `Remove` soft-deletes a live relationship. It requires
undeleted endpoints; already retired pairs and deleted endpoints return
`ErrRecordNotFound`. Failed or duplicate changes do not advance the version. Callers must reload a Goal after these
operations before editing its settings or replacing its Subjects.

SQLite transactions serialize relationship writes. Replacement acquires the
write lock with a version-conditional update before reading or comparing links,
so standalone writes cannot bypass a checked version. Contention across separate
connections can return `data.ErrDatabaseBusy`; callers should reload before
retrying. Direct SQL writers remain responsible for the application's version
contract; no new version triggers are introduced.

## Deletion and progress

Subject deletion retains Subject–Goal rows. It does not change Goals, their
versions, settings, activation, priority, progress, or parent links. Other live
Subjects remain current; a Goal whose last live Subject was deleted becomes
Unassigned. Existing Event/Thought unlinking and provenance retention stay intact.

An ordinary Misc Event has no live `event_subjects` associations. Goal discovery returns no Goals
for it, regardless of how many Unassigned Goals exist. Unassigned does not mean
associated with every NULL-Subject Event. This API does not calculate or record
automatic progress, change parent relationships, or implement a Goals screen.
Completed Event writes separately establish [earned Goal support](event-goal-attribution.md).
Later assignment changes do not retroactively rewrite that support.

## Stable Subject-local Goal references

A live membership can be displayed as `Programming 3`. Numbers identify a Goal
within a particular Subject, not its current position or priority. One Goal can
have different numbers in different Subjects. Unassigned Goals display `Misc`
without a number; internal Goal IDs remain unchanged.

`subject_goals` owns both membership state and the stable number. Its pair primary
key retains one row per Subject–Goal pair; its unique `(subject_id, goal_number)`
constraint includes retired rows. `deleted_at` uses nullable Unix seconds, just
like the existing soft-deletion convention. Retiring an association does not
release its number or change the Goal's activation.

Creation, replacement, and standalone addition run inside their existing write
transactions. A new pair receives `MAX(goal_number) + 1` over all retained rows
for its Subject. Relinking clears `deleted_at` on the existing row. Allocation
rejects integer overflow; reactivation still works at the maximum integer.
Renaming, filtering, priority, activation, and soft-deletion changes never
renumber existing pairs. Hard-deleting endpoints preserves foreign-key cascades.

Replacement compares live membership as an unordered set. Deselected pairs are
retired, selected retired pairs are reactivated, and new pairs are inserted.
Unchanged sets preserve Goal version/timestamp and every relationship timestamp.
Links to soft-deleted Subjects remain untouched. Stale versions fail even for
no-op requests. Number allocation adds no extra Goal version increment and
changes no progress/discovery behavior.

`ListLiveNumberedSubjectsForGoal` reads each number directly from the membership
row using one read snapshot, with the existing unavailable-Goal and nonnil empty
Unassigned contracts. All live readers and Goal discovery exclude retired rows.
Raw relationship APIs preserve them. This is retained pair state, not an audit
log of every transition.

## Development migrations

Migration 11 defines the complete numbered, soft-deletable `subject_goals` table.
There is no separate numbering registry, upgrade/backfill migration, or legacy
relationship schema. Its down migration drops the relationship table, matching
the existing development migration convention.

Development databases are disposable. Recreate the disposable database after
updating to this schema; editing an already applied migration does not upgrade
an existing database. Use `make tui/demo/stress` or
`make tui/browser/demo/stress` for a fresh migrated, populated demo. No running
or persistent database is reset automatically.
