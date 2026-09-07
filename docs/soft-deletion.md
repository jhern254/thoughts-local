# Soft deletion

The database retains deleted entities and their historical relationships. Migration
000009 adds nullable `deleted_at` to users, subjects, thoughts, events, tags, and
goals. A non-NULL value identifies explicit deletion; NULL means undeleted.
Timestamps are integer UTC epoch seconds, with
`created_at <= deleted_at <= updated_at`. A deletion writes `updated_at` and
`deleted_at` together. Versioned entities should also increment their version when
a future application operation deletes them; this change adds no such operations.

| Table | Purpose and relationship behavior |
| --- | --- |
| users | Retain ownership. A deleted owner makes its content unavailable without changing each child row. Handles and email addresses remain reserved. |
| subjects | Retain subject identity and thought associations. Existing subject deletion now sets timestamps. |
| thoughts | Retain authored content, tag links, and goal-progress provenance. |
| events | Retain provenance for thoughts and progress; deleting an event does not delete either. |
| tags | Retain tag identity and historical thought associations. |
| goals | Retain the goal and progress ledger. `goal_is_active` is separate from deletion. |
| thought_tags | No deletion marker: this association has no independent deletion lifecycle here. A normal view must require both endpoints and their owners to be undeleted. |
| goal_progress | No deletion marker: retain the append-only ledger. A normal view must require its goal and owner to be undeleted. Deleting source events or thoughts does not invalidate recorded contributions. |

Subject, tag, and goal names are unique per user among undeleted records. A reused
name receives a new ID; historical links still reference the original ID. User
identity uniqueness and all primary/foreign keys remain unchanged. Setting a
deletion timestamp does not invoke `ON DELETE` actions. Explicit SQL hard deletes
still use the existing cascading and unlinking behavior.

## Existing application behavior

Only subject deletion currently has an application operation. Its get, list,
update, and delete operations exclude deleted subjects and deleted owners. Creating
subjects or thoughts requires an undeleted owner. Thought creation also requires
any supplied subject or event and its owner to be undeleted, checked within the
insert statement.
Existing thought/subject ownership constraints remain in force.

Thought reads exclude deleted thoughts and owners. They return NULL subject/event
IDs for deleted referenced entities (or deleted event owners) while retaining those
IDs in SQLite. Deleted
subjects and events do not hide otherwise available thoughts.

Local-user bootstrap does not revive or replace a deleted local user. The reserved
handle causes the existing duplicate-record error. There is no account restoration
interface in this change.

Other entity deletion services, trash views, restore and purge operations are not
implemented. The table above specifies visibility for future readers; it does not
create those readers. SQL foreign keys guarantee existence, not availability, so
new application operations must check deletion and ownership explicitly.

## Inspecting retained data

```sql
SELECT subject_id, subject_name, deleted_at
FROM subjects
WHERE deleted_at IS NOT NULL;

-- An undeleted child can be unavailable because its owner is deleted.
SELECT s.subject_id, s.deleted_at, u.deleted_at AS owner_deleted_at
FROM subjects AS s
JOIN users AS u ON u.user_id = s.user_id
WHERE s.deleted_at IS NOT NULL OR u.deleted_at IS NOT NULL;
```

This preserves data and makes deletion easy to identify without adding another
user-facing lifecycle. It also requires filtering in application queries, retains
storage indefinitely, and is neither a backup nor a complete audit log.

## Migration and rollback

Apply with `make migrate/up`; application startup does not run migrations.
Migration 000009 is additive except for the three name uniqueness indexes. Existing
rows begin undeleted, with their IDs and relationships intact.

The down migration refuses to run if any entity has a deletion timestamp. It must
not silently resurrect retained rows or discard deletion history. Before deletion
has been used, rollback restores full name uniqueness and removes the new columns.
Golang-migrate wraps the SQL in a transaction; the temporary constraint guard makes
refusal happen before persistent schema changes. A refused migration can leave
migration metadata dirty even though schema changes were rolled back: inspect the
schema and migration error before repairing that metadata. Do not clear deletion
markers merely to force a rollback.
