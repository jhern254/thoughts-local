-- Thoughts
CREATE TABLE IF NOT EXISTS thoughts (
    thought_id   INTEGER PRIMARY KEY,                          -- int64
    user_id      TEXT    NOT NULL,                             -- FK -> users.user_id
    subject_id   INTEGER,                                      -- nullable FK -> subjects.subject_id
    event_id     INTEGER,                                      -- nullable FK -> events.event_id
    thought      TEXT    NOT NULL,                             -- validated via CHECK below
    version      INTEGER NOT NULL DEFAULT 1 CHECK (version > 0), -- optimistic-lock version
    observed_at  INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)
    created_at   INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)
    updated_at   INTEGER NOT NULL DEFAULT (unixepoch('now')),  -- epoch seconds (UTC)

    -- Validation checks
    -- Non-empty after trim; cap size to ~1M characters (~1MB). (Fix 'body' -> 'thought'.)
    CONSTRAINT ck_thoughts_text_len
        CHECK (length(trim(thought)) > 0 AND length(thought) <= 1000000),

    -- Timestamps must be logical
    CONSTRAINT ck_thoughts_time_order
        CHECK (created_at <= updated_at),

    -- Foreign keys
    CONSTRAINT fk_thoughts_user
        FOREIGN KEY (user_id)    REFERENCES users(user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE,

    CONSTRAINT fk_thoughts_subject
        FOREIGN KEY (subject_id) REFERENCES subjects(subject_id)
        ON DELETE SET NULL
        ON UPDATE CASCADE,

    CONSTRAINT fk_thoughts_event
        FOREIGN KEY (event_id)   REFERENCES events(event_id)
        ON DELETE SET NULL
        ON UPDATE CASCADE
);

-- Indexes
-- time grouping
CREATE INDEX IF NOT EXISTS idx_thoughts_user_observed_at
    ON thoughts (user_id, observed_at);

CREATE INDEX IF NOT EXISTS idx_thoughts_event
    ON thoughts (event_id);

CREATE INDEX IF NOT EXISTS idx_thoughts_subject
    ON thoughts (subject_id);
