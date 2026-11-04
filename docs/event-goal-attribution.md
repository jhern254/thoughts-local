# Automatic Event attribution

Completed Events earn their full elapsed UTC seconds for every distinct eligible
Goal reached through their live Subjects. Matching a Goal through several
Subjects does not multiply its credit. Eligibility requires a live owner, an
active undeleted Goal, and live Event–Subject and Subject–Goal links. Priority
and Goal date bounds do not filter attribution. Ongoing Events earn no committed
time. Zero-duration completions freeze eligible recipients with zero seconds.

## Immutable history and effective progress

Three normalized tables form the ledger:

- `event_credit_revisions` records one credited interval and reason per Event
  version that changes attribution.
- `event_goal_credits` identifies the unique recipient Goals in that revision.
- `event_goal_credit_subjects` records each recipient's contributing Subjects.

Each revision is a complete snapshot, written atomically with the Event. An
empty snapshot supersedes earlier positive attribution. Effective progress uses
only the latest recorded revision for each Event, never the latest row per Goal.
Label-only edits can advance the Event version without creating a revision.

Ordinary corrections do not update or delete ledger rows. Duration/boundary
corrections append the corrected interval while retaining earned recipients and
support. Subject corrections compare the persisted live set before and after
Update: additions discover current eligible support; removals retract only their
support; unchanged contributing Subjects retain earned support even after Goal
deactivation or Subject–Goal unlinking. A recipient disappears from the new
snapshot only when all its contributing support is explicitly removed.

Subject soft deletion automatically retires Event links but preserves attribution.
Later label/time edits preserve earned recipients. An ordinary complete-live-set
Update cannot express removal of support from a Subject already deleted and
absent from that set. Neither nil nor empty slices imply such historical intent.
A future explicit historical correction operation must append a revision; this
PR provides no such operation or UI.

Legacy completed Events are not backfilled by migration or by label/time edits.
Only newly added Subjects can establish credit during a legacy Event correction.
An Event completed without eligible Goals does not gain credit merely because a
Goal is created or linked later.

## Maintenance and deletion

The app uses soft deletion. A soft-deleted Event keeps its ledger but stops
contributing automatic time. Soft-deleted/inactive Goals keep earned history,
which remains available through live ancestors.

Database maintenance remains possible. Hard-deleting Events, Goals, or Users
cascades to dependent ledger records. Hard-deleting a Subject instead clears its
optional live reference: recorded Subject identity and original attribution time
remain immutable, and earned credit continues counting. Detached support cannot
be reattached or confused with a new Subject reusing the numeric ID. Legitimate
key renames may update foreign-key references; credited intervals and historical
payload remain immutable. Individual ownership transfers cannot break same-owner
constraints. These are narrow referential-maintenance exceptions, not application
correction workflows.

## Lifetime summaries

`runtime.GoalProgress().GetSummary(ctx, userID, goalID)` returns direct and
effective automatic seconds, plus direct and effective explicit/manual seconds.
Unavailable requested Goals return not found; existing empty Goals return zeros.

The current parent DAG determines reachable descendants, including retained
inactive/deleted descendants. Automatic totals deduplicate by Event ID across
all recipients and paths, including a parent's own direct credit. Two independent
Events of equal duration both count. Manual rows remain independent contributions
and are deduplicated only by progress-record identity, never duration or optional
Event provenance. Automatic and manual categories remain separate.

Future period calculations must use the requested Goal's own timezone/cadence
boundaries and clip each unique Event's credited interval once. They must not
sum child-period totals. This implementation provides lifetime summaries only;
integer overflow returns an error rather than rounded or partial totals.

The migrations are additive and do not rewrite meaningful existing data or manual
`goal_progress`. There is no original-data reset, automatic backfill, inherited
credit materialization, background worker, or progress UI.
