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

## Stable Subject-local Goal references

A live membership can be displayed as `Programming #3`. Numbers identify a Goal
within a particular Subject, not its current position or priority. One Goal can
have different numbers in different Subjects. Unassigned Goals display `Misc`
without a number; internal Goal IDs remain unchanged.

`subject_goal_numbers` stores positive, unique reservations per Subject. It is
not membership: `subject_goals` remains the sole membership authority. Creation,
replacement, and standalone addition reserve a number inside the existing write
transaction. Allocation uses the highest reserved number plus one. Unlinking,
soft deletion, renaming, filtering, and activation changes never renumber Goals.
Re-linking a Goal restores its previous reservation. Hard-deleting an endpoint
cascades its reservations; permanent reservation after destructive purging is not
guaranteed.

Migration 15 backfills every retained link in Goal-ID order within each Subject,
including deleted endpoints, without modifying Goals or their versions. Links
removed before this migration cannot be reconstructed. It applies forward to
existing databases without resetting them.

`ListLiveNumberedSubjectsForGoal` returns Subject records with their numbers using
one read snapshot, with the same unavailable-Goal and empty-Unassigned contracts
as the existing live query. A membership missing its reservation is an error,
not Misc. Direct SQL importers must populate reservations transactionally; normal
application callers use the assignment or link services. Raw relationship and
unnumbered Subject APIs retain their existing contracts. Number allocation adds
no extra Goal version increment and changes no progress/discovery behavior.
