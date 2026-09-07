-- golang-migrate wraps this migration in a transaction. Refuse to erase any
-- deletion markers: doing so could resurrect records or introduce duplicate names.
CREATE TEMP TABLE soft_deletion_rollback_guard (
    deleted_count INTEGER CONSTRAINT cannot_rollback_retained_deletions CHECK (deleted_count = 0)
);
INSERT INTO soft_deletion_rollback_guard (deleted_count)
SELECT (SELECT count(*) FROM users WHERE deleted_at IS NOT NULL)
     + (SELECT count(*) FROM subjects WHERE deleted_at IS NOT NULL)
     + (SELECT count(*) FROM events WHERE deleted_at IS NOT NULL)
     + (SELECT count(*) FROM thoughts WHERE deleted_at IS NOT NULL)
     + (SELECT count(*) FROM tags WHERE deleted_at IS NOT NULL)
     + (SELECT count(*) FROM goals WHERE deleted_at IS NOT NULL);
DROP TABLE soft_deletion_rollback_guard;

DROP INDEX uq_subjects_user_name;
CREATE UNIQUE INDEX uq_subjects_user_name ON subjects (user_id, subject_name);

DROP INDEX uq_tags_user_name;
CREATE UNIQUE INDEX uq_tags_user_name ON tags (user_id, tag_name);

DROP INDEX uq_goals_user_name;
CREATE UNIQUE INDEX uq_goals_user_name ON goals (user_id, goal_name);

ALTER TABLE goals DROP COLUMN deleted_at;
ALTER TABLE tags DROP COLUMN deleted_at;
ALTER TABLE thoughts DROP COLUMN deleted_at;
ALTER TABLE events DROP COLUMN deleted_at;
ALTER TABLE subjects DROP COLUMN deleted_at;
ALTER TABLE users DROP COLUMN deleted_at;
