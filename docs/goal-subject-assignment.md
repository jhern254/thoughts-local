# Goal–Subject assignment

`subject_goals` is the only source of Goal membership. A Goal may have zero,
one, or many live Subjects. **Unassigned** means zero live Subjects; the future
Goals UI will display **Misc**. This classification is computed, not stored.
No migration, sentinel Subject, or mandatory association is needed.

Live Subjects belong to the same user and have `deleted_at IS NULL`. Names do
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
as history. An empty selection removes all live links without deleting the Goal.
The returned Goal supplies the version for the next edit.

Use `runtime.SubjectGoals().ListLiveSubjectsForGoal(ctx, userID, goalID)` for
current membership, including canonical Subject names. Results are ordered by
Subject ID. An accessible Goal with no live Subjects returns a non-nil empty
slice; a missing, foreign-owned, or soft-deleted Goal, or an unavailable owner,
returns `data.ErrRecordNotFound`. Inactive Goals remain accessible. The read uses
one SQLite snapshot for Goal visibility and membership.

`ListForGoal`/`ListForSubject` and the store's raw `ListGoalSubjects`/
`ListSubjectGoals` retain their historical-link behavior, including soft-deleted
endpoints. They are not current-membership queries.

## Versions and transactions

A replacement requires the current Goal version even if it requests the same
set. A changed live set increments the version exactly once and keeps
`updated_at` nondecreasing. An identical set, regardless of order, preserves
both version and timestamp. Stale callers receive `data.ErrVersionConflict`
without changing membership.

Standalone Subject–Goal `Add` and `Remove` retain their signatures. Each
successful link change now advances the Goal version once in the same
transaction, including explicit removal of retained historical links. Failed
or duplicate changes do not advance it. Callers must reload a Goal after these
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

An ordinary Misc Event has `subject_id = NULL`. Goal discovery returns no Goals
for it, regardless of how many Unassigned Goals exist. Unassigned does not mean
associated with every NULL-Subject Event. This API does not calculate or record
automatic progress, change parent relationships, or implement a Goals screen.
