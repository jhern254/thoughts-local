-- Subjects table.
CREATE TABLE IF NOT EXISTS subjects (
    subject_id    INTEGER PRIMARY KEY,                         -- maps to int64
    user_id       TEXT    NOT NULL,                            -- FK -> users.user_id
    subject_name  TEXT    NOT NULL,                            -- validated via CHECK below
    created_at    INTEGER NOT NULL DEFAULT (unixepoch('now')), -- epoch seconds (UTC)
    updated_at    INTEGER NOT NULL DEFAULT (unixepoch('now')), -- epoch seconds (UTC)

    -- Validation checks
    CONSTRAINT ck_subjects_name_len
        CHECK (length(trim(subject_name)) BETWEEN 1 AND 255),  -- non-empty, reasonable max; trims spaces
    CONSTRAINT ck_subjects_time_order
        CHECK (created_at <= updated_at),                      -- created_at must not be after updated_at

    -- Foreign key
    CONSTRAINT fk_subjects_user
        FOREIGN KEY (user_id) REFERENCES users(user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);

-- Uniqueness: per-user subject names must be unique
CREATE UNIQUE INDEX IF NOT EXISTS uq_subjects_user_name
    ON subjects (user_id, subject_name);

-- Composite parent key for enforcing thought/subject ownership.
CREATE UNIQUE INDEX IF NOT EXISTS uq_subjects_id_user
    ON subjects (subject_id, user_id);

CREATE INDEX IF NOT EXISTS idx_subjects_user
    ON subjects (user_id);
