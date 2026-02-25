-- Events
CREATE TABLE IF NOT EXISTS events (
    event_id        INTEGER PRIMARY KEY,
    user_id         TEXT    NOT NULL,
    activity_type   TEXT,
    started_at      INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)
    ended_at        INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)
    created_at      INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)

    CONSTRAINT ck_events_activity_type_len
        CHECK (activity_type IS NULL OR length(trim(activity_type)) BETWEEN 1 AND 4096),

    CONSTRAINT ck_ended_at_null_or_greater_than_started_at
        CHECK (ended_at IS NULL OR ended_at >= started_at),

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

