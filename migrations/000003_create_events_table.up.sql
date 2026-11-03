-- Events table.
CREATE TABLE IF NOT EXISTS events (
    event_id        INTEGER PRIMARY KEY,
    user_id         TEXT    NOT NULL,
    activity_type   TEXT,
    started_at      INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)
    ended_at        INTEGER, -- NULL means ongoing; UTC epoch seconds otherwise
    created_at      INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)
    deleted_at      INTEGER, -- NULL means undeleted; UTC epoch seconds otherwise
    version         INTEGER NOT NULL DEFAULT 1 CHECK (version > 0), -- optimistic-lock version

    CONSTRAINT ck_events_activity_type_len
        CHECK (activity_type IS NULL OR length(trim(activity_type)) BETWEEN 1 AND 4096),

    CONSTRAINT ck_events_started_at
        CHECK (typeof(started_at) = 'integer'),

    CONSTRAINT ck_ended_at_null_or_greater_than_started_at
        CHECK (ended_at IS NULL OR
            (typeof(ended_at) = 'integer' AND ended_at >= started_at)),

    CONSTRAINT ck_events_deleted_at
        CHECK (deleted_at IS NULL OR
            (typeof(deleted_at) = 'integer' AND created_at <= deleted_at AND deleted_at <= updated_at)),

    CONSTRAINT ck_events_time_order
        CHECK (created_at <= updated_at),

    -- Foreign key
    CONSTRAINT fk_events_user
        FOREIGN KEY (user_id) REFERENCES users(user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_events_user_started_at
    ON events (user_id, started_at);

-- One ongoing, undeleted event per user. Historical overlap checks belong to
-- transactional event operations; this index does not prevent those overlaps.
CREATE UNIQUE INDEX IF NOT EXISTS uq_events_ongoing_user
    ON events (user_id) WHERE ended_at IS NULL AND deleted_at IS NULL;

-- Parent key for enforcing ownership of progress provenance.
CREATE UNIQUE INDEX IF NOT EXISTS uq_events_id_user
    ON events (event_id, user_id);

-- Retained identity for each same-owner Event–Subject pair.
CREATE TABLE IF NOT EXISTS event_subjects (
    event_id INTEGER NOT NULL,
    subject_id INTEGER NOT NULL,
    user_id TEXT NOT NULL,
    deleted_at INTEGER CHECK (deleted_at IS NULL OR typeof(deleted_at) = 'integer'),
    PRIMARY KEY (event_id, subject_id),
    FOREIGN KEY (event_id, user_id) REFERENCES events(event_id, user_id)
        ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (subject_id, user_id) REFERENCES subjects(subject_id, user_id)
        ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_event_subjects_subject_event
    ON event_subjects (subject_id, event_id);
