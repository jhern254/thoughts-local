# Automatic Goal progress from Events

The Event owns time. `goal_progress` owns earned Goal contributions, with one row
per Event × Goal × contributing Subject. It does not copy timestamps or durations.
Current `events.ended_at - events.started_at` determines earned UTC seconds.
Multiple contribution rows for the same Event and Goal still count its time once.

## Example: one work session, several Goals

Suppose you spend 90 minutes building the app and assign the Event to Programming
and Research. Practice Go belongs to both Subjects; Study AI belongs to Research.
Both Goals sit beneath a larger parent Goal, Become a Better Engineer.

- Practice Go earns 90 minutes, even though two Subjects connect it to the Event.
- Study AI also earns 90 minutes. The session's time is not divided between Goals.
- Become a Better Engineer earns 90 minutes from that session, not 180. It is the
  same Event reaching the parent through two children.

This is what a parent "rollup" means here: collect the Events earned by the
parent and all Goals beneath it, then count each Event once. Include the full
chain of children, not just immediate children. Even if the parent also earns
that Event directly, the total from this session is still 90 minutes.

If you correct the session to 60 minutes, each of these totals becomes 60 minutes.
The calculation reads the corrected Event; there are no separate duration values
to repair. Adding another independent 60-minute Event would increase each
applicable total by 60 minutes, even though both Events have the same duration.

A contribution row records one Subject connection between an Event and a Goal.
It is not another block of time to add. Removing Programming from the example
Event leaves Practice Go's Research connection, so it still earns the full time.
Removing Research as well removes its final connection and retracts that Event's
time from Practice Go. Deleting a Subject elsewhere in the app is different:
that preserves time already earned, as described below.

## Establishing and correcting contributions

Completing an Event through End, starting a successor, or Create Past Event
establishes contributions inside the Event's SQLite transaction. Each live Event
Subject establishes contributions to every active, undeleted same-owner Goal
reached through a live Subject–Goal link. Priority and Goal date bounds do not
filter eligibility. Several Subjects may establish contributions to the same
Goal; several Goals may earn the full Event duration. An unassigned/Misc Event
earns no Goal credit. Ongoing Events receive no contributions; zero-duration
completions establish contributions with zero time.

Event corrections have deliberately narrow effects:

- Label and interval edits leave contribution rows and their `progress_id` identities
  untouched. Corrected time is immediately reflected in summaries.
- Explicit additions to the live Subject set discover currently eligible Goals
  through those added Subjects only.
- Explicit removals delete contribution rows through the removed Subjects only.
  Remaining contributions retain the Goal's full Event duration.
- Unchanged Subjects preserve earned contributions, including after Goal deactivation
  or retirement of a Subject–Goal link.

Adding a Goal or linking it to a Subject does not retroactively credit completed
Events. Existing completed Events are not backfilled. Any failed contribution write
rolls back the entire Event operation; Goal versions are unchanged.

## Deletion and historical identity

Soft-deleted Events cease contributing time while their contribution rows remain.
Hard-deleting an Event or Goal removes dependent contributions. Deleting an owner
cascades normally. Same-owner foreign keys prevent ownership transfers from
silently moving earned contributions to another user.

Soft-deleting a Subject retires its Event assignments without erasing earned
contributions. Hard deletion clears the optional live `subject_id` reference, while
`recorded_subject_id` retains the original identity. A new Subject reusing that
numeric ID creates new contribution rows; it never reattaches detached contributions.
Uniqueness applies to live Event–Goal–Subject contributions; detached rows can coexist
with contributions earned through a later Subject using the same recorded ID.

A normal complete-live-set Event update cannot retract contributions from a Subject
already deleted and absent from the previous live set. Neither nil nor empty
Subject lists imply that historical intent. Label/time edits and ordinary live
classification edits preserve those contributions. An explicit historical-contribution
correction operation would be a separate future feature.

## Lifetime summaries and parent Goals

`runtime.GoalProgress().GetSummary(ctx, userID, goalID)` returns
`LifetimeEarnedSeconds`. An existing Goal with no qualifying Events returns zero;
an unavailable or foreign-owned requested Goal returns not found.

A parent Goal includes Events earned by Goals beneath it, including children
that later became inactive or were soft-deleted. One child can belong to more
than one parent, and several chains of children can lead to the same Goal.
Those multiple paths never multiply an Event's time.

The SQL query follows the current parent relationships, gathers distinct Event
IDs, and adds their current durations. It excludes ongoing and soft-deleted
Events. Adding or removing a parent relationship changes which Events the
parent includes; no copied progress records need updating. Invalid intervals
and totals too large for a signed 64-bit integer return an error rather than an
inaccurate or partial total.

This is a lifetime metric, with no progress UI or cadence engine. Future period
calculations must use the requested Goal's own timezone/cadence boundaries and
clip each unique Event interval once, rather than summing child-period totals.

## MVP tradeoff and development databases

There is one authoritative progress table, no attribution revision ledger, and
no stored duplicate of Event time. Previous attribution assignments are
intentionally not retained or replayable: the guarantee is correct current
progress, not reconstruction of earlier assignments. Retained Subject identity
is provenance for a contribution, not an audit log. Manual Goal-time entry is not a
supported workflow; Create Past Event records missing activity instead.

Migration 8 is amended for the final development schema; the unmerged ledger
migrations 16–18 are removed. Existing disposable development databases must be
recreated, including databases already at migration 15 with the former manual
progress table. Normal `migrate up` cannot convert those amended schemas. No
application database is automatically reset. Preserve/export any meaningful
external data before resetting it; this PR supplies no production-data converter.
