-- Goal progress (append-only)
CREATE TABLE IF NOT EXISTS goal_progress (
    progress_id     INTEGER PRIMARY KEY,
    goal_id         INTEGER NOT NULL,
    user_id         TEXT    NOT NULL,
    occurred_at     INTEGER NOT NULL DEFAULT (unixepoch('now')), -- epoch seconds (UTC)
    time_spent_sec  INTEGER NOT NULL,                         -- materialized contribution
    event_id        INTEGER,                                  -- optional provenance
    thought_id      INTEGER,                                  -- optional provenance
    progress_note            TEXT,
    created_at      INTEGER NOT NULL DEFAULT (unixepoch('now')),

    -- Validation checks
    CONSTRAINT ck_goal_progress_time_spent_pos
        CHECK (time_spent_sec > 0),

    CONSTRAINT ck_goal_progress_note_len
        CHECK (progress_note IS NULL OR length(progress_note) <= 1000000),  -- 1 MB

    -- Foreign keys
    CONSTRAINT fk_goal_progress_user
        FOREIGN KEY (user_id) REFERENCES users(user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE,

    CONSTRAINT fk_goal_progress_goal
        FOREIGN KEY (goal_id) REFERENCES goals(goal_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE,

    CONSTRAINT fk_goal_progress_event
        FOREIGN KEY (event_id) REFERENCES events(event_id)
        ON DELETE SET NULL
        ON UPDATE CASCADE,

    CONSTRAINT fk_goal_progress_thought
        FOREIGN KEY (thought_id) REFERENCES thoughts(thought_id)
        ON DELETE SET NULL
        ON UPDATE CASCADE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_progress_goal_time
    ON goal_progress (goal_id, occurred_at);

CREATE INDEX IF NOT EXISTS idx_progress_user_time
    ON goal_progress (user_id, occurred_at);

CREATE INDEX IF NOT EXISTS idx_progress_event
    ON goal_progress (event_id);

CREATE INDEX IF NOT EXISTS idx_progress_thought
    ON goal_progress (thought_id);

-- TODO: Check if more needed indexes
