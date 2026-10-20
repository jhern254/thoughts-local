-- Aliases name a canonical Subject; they do not own other entity relationships.
CREATE TABLE subject_aliases (
    alias_id INTEGER PRIMARY KEY,
    subject_id INTEGER NOT NULL,
    user_id TEXT NOT NULL,
    alias_name TEXT NOT NULL COLLATE BINARY,
    created_at INTEGER NOT NULL DEFAULT (unixepoch('now')),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch('now')),
    deleted_at INTEGER,

    CONSTRAINT ck_subject_aliases_name_len
        CHECK (length(trim(alias_name)) BETWEEN 1 AND 255),
    CONSTRAINT ck_subject_aliases_time_order CHECK (created_at <= updated_at),
    CONSTRAINT ck_subject_aliases_deleted_at CHECK (deleted_at IS NULL OR
        (typeof(deleted_at) = 'integer' AND created_at <= deleted_at AND deleted_at <= updated_at)),
    CONSTRAINT fk_subject_aliases_subject FOREIGN KEY (subject_id, user_id)
        REFERENCES subjects(subject_id, user_id) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE UNIQUE INDEX uq_subject_aliases_user_name
    ON subject_aliases(user_id, alias_name) WHERE deleted_at IS NULL;
CREATE INDEX idx_subject_aliases_subject
    ON subject_aliases(user_id, subject_id, alias_id) WHERE deleted_at IS NULL;

-- A unique index cannot span two tables. These guards protect the shared exact
-- name namespace in both write directions, including writers outside Go. SQLite
-- serializes writes, so competing canonical/alias writes cannot both commit.
-- Fixed trigger messages are persistence sentinels, never submitted names.
CREATE TRIGGER subject_aliases_name_insert
BEFORE INSERT ON subject_aliases WHEN NEW.deleted_at IS NULL
BEGIN
    SELECT RAISE(ABORT, 'subject name conflict') WHERE EXISTS (
        SELECT 1 FROM subjects WHERE user_id=NEW.user_id
        AND subject_name=NEW.alias_name AND deleted_at IS NULL
    );
END;

CREATE TRIGGER subject_aliases_name_update
BEFORE UPDATE OF alias_name, user_id, deleted_at ON subject_aliases
WHEN NEW.deleted_at IS NULL
BEGIN
    SELECT RAISE(ABORT, 'subject name conflict') WHERE EXISTS (
        SELECT 1 FROM subjects WHERE user_id=NEW.user_id
        AND subject_name=NEW.alias_name AND deleted_at IS NULL
    );
END;

CREATE TRIGGER subjects_alias_name_insert
BEFORE INSERT ON subjects WHEN NEW.deleted_at IS NULL
BEGIN
    SELECT RAISE(ABORT, 'subject name conflict') WHERE EXISTS (
        SELECT 1 FROM subject_aliases WHERE user_id=NEW.user_id
        AND alias_name=NEW.subject_name AND deleted_at IS NULL
    );
END;

CREATE TRIGGER subjects_alias_name_update
BEFORE UPDATE OF subject_name, user_id, deleted_at ON subjects
WHEN NEW.deleted_at IS NULL
BEGIN
    SELECT RAISE(ABORT, 'subject name conflict') WHERE EXISTS (
        SELECT 1 FROM subject_aliases WHERE user_id=NEW.user_id
        AND alias_name=NEW.subject_name AND deleted_at IS NULL
    );
END;
