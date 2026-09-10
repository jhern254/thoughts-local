-- Mindset designation history, not snapshots of thought text.
-- Marking opens a period, unmarking closes it, and reactivation adds a new row.
-- An open period supplies current status; no separate boolean is needed.
-- Soft deletion leaves history unchanged. Active-screen queries must join
-- thoughts and users and exclude their soft-deleted records.
-- Periods record actions, not scheduled/backdated intervals. Future application
-- code must keep transitions chronological: these constraints do not prevent
-- overlap between historical periods.
CREATE TABLE IF NOT EXISTS thought_mindset_periods (
    mindset_period_id INTEGER PRIMARY KEY,
    thought_id        INTEGER NOT NULL,
    started_at        INTEGER NOT NULL DEFAULT (unixepoch('now')), -- UTC epoch seconds
    ended_at          INTEGER, -- NULL means open; UTC epoch seconds otherwise

    CONSTRAINT ck_mindset_periods_started_at
        CHECK (typeof(started_at) = 'integer'),

    -- Equal timestamps permit marking and unmarking within the same second.
    CONSTRAINT ck_mindset_periods_ended_at
        CHECK (ended_at IS NULL OR
            (typeof(ended_at) = 'integer' AND ended_at >= started_at)),

    CONSTRAINT fk_mindset_periods_thought
        FOREIGN KEY (thought_id) REFERENCES thoughts(thought_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);

-- Many thoughts can be mindsets, but each has at most one open period.
CREATE UNIQUE INDEX IF NOT EXISTS uq_mindset_periods_open_thought
    ON thought_mindset_periods (thought_id) WHERE ended_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mindset_periods_thought_started_at
    ON thought_mindset_periods (thought_id, started_at);
