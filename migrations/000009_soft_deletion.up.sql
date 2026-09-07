-- Retain deleted entities and their historical relationships. NULL means undeleted.
-- Timestamps use UTC epoch seconds, as do created_at and updated_at.
ALTER TABLE users ADD COLUMN deleted_at INTEGER
    CONSTRAINT ck_users_deleted_at
    CHECK (deleted_at IS NULL OR
        (typeof(deleted_at) = 'integer' AND created_at <= deleted_at AND deleted_at <= updated_at));

ALTER TABLE subjects ADD COLUMN deleted_at INTEGER
    CONSTRAINT ck_subjects_deleted_at
    CHECK (deleted_at IS NULL OR
        (typeof(deleted_at) = 'integer' AND created_at <= deleted_at AND deleted_at <= updated_at));

ALTER TABLE events ADD COLUMN deleted_at INTEGER
    CONSTRAINT ck_events_deleted_at
    CHECK (deleted_at IS NULL OR
        (typeof(deleted_at) = 'integer' AND created_at <= deleted_at AND deleted_at <= updated_at));

ALTER TABLE thoughts ADD COLUMN deleted_at INTEGER
    CONSTRAINT ck_thoughts_deleted_at
    CHECK (deleted_at IS NULL OR
        (typeof(deleted_at) = 'integer' AND created_at <= deleted_at AND deleted_at <= updated_at));

ALTER TABLE tags ADD COLUMN deleted_at INTEGER
    CONSTRAINT ck_tags_deleted_at
    CHECK (deleted_at IS NULL OR
        (typeof(deleted_at) = 'integer' AND created_at <= deleted_at AND deleted_at <= updated_at));

ALTER TABLE goals ADD COLUMN deleted_at INTEGER
    CONSTRAINT ck_goals_deleted_at
    CHECK (deleted_at IS NULL OR
        (typeof(deleted_at) = 'integer' AND created_at <= deleted_at AND deleted_at <= updated_at));

-- Names can be reused without changing retained entity IDs or ownership keys.
DROP INDEX uq_subjects_user_name;
CREATE UNIQUE INDEX uq_subjects_user_name
    ON subjects (user_id, subject_name) WHERE deleted_at IS NULL;

DROP INDEX uq_tags_user_name;
CREATE UNIQUE INDEX uq_tags_user_name
    ON tags (user_id, tag_name) WHERE deleted_at IS NULL;

DROP INDEX uq_goals_user_name;
CREATE UNIQUE INDEX uq_goals_user_name
    ON goals (user_id, goal_name) WHERE deleted_at IS NULL;

