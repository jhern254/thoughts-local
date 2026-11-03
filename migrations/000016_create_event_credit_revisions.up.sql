-- Attribution snapshots are appended atomically by the Event transaction.
CREATE TABLE event_credit_revisions (
    event_id INTEGER NOT NULL,
    event_version INTEGER NOT NULL CHECK (typeof(event_version) = 'integer' AND event_version > 0),
    user_id TEXT NOT NULL,
    credited_start_at INTEGER NOT NULL CHECK (typeof(credited_start_at) = 'integer'),
    credited_end_at INTEGER NOT NULL CHECK (
        typeof(credited_end_at) = 'integer'
        AND credited_end_at >= credited_start_at
        AND typeof(credited_end_at - credited_start_at) = 'integer'
    ),
    recorded_at INTEGER NOT NULL DEFAULT (unixepoch('now')) CHECK (typeof(recorded_at) = 'integer'),
    reason TEXT NOT NULL CHECK (reason IN ('completion', 'interval_correction', 'classification_correction')),
    PRIMARY KEY (event_id, event_version),
    UNIQUE (event_id, event_version, user_id),
    FOREIGN KEY (user_id) REFERENCES users (user_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (event_id, user_id) REFERENCES events (event_id, user_id) ON DELETE CASCADE ON UPDATE CASCADE
);

-- Identity maintenance is allowed only after the old referenced key disappears.
CREATE TRIGGER event_credit_revisions_immutable_update
BEFORE UPDATE ON event_credit_revisions
WHEN (
    NEW.event_version IS NOT OLD.event_version
    OR NEW.credited_start_at IS NOT OLD.credited_start_at
    OR NEW.credited_end_at IS NOT OLD.credited_end_at
    OR NEW.recorded_at IS NOT OLD.recorded_at
    OR NEW.reason IS NOT OLD.reason
)
OR (
    EXISTS (
        SELECT 1
        FROM users
        WHERE user_id = OLD.user_id
    )
    AND (
        EXISTS (
            SELECT 1
            FROM events
            WHERE event_id = OLD.event_id
                AND user_id = OLD.user_id
        )
    )
)
BEGIN
    SELECT RAISE(ABORT, 'immutable attribution history');
END;

CREATE TRIGGER event_credit_revisions_immutable_delete
BEFORE DELETE ON event_credit_revisions
WHEN EXISTS (
    SELECT 1
    FROM users
    WHERE user_id = OLD.user_id
)
AND (
    EXISTS (
        SELECT 1
        FROM events
        WHERE event_id = OLD.event_id
            AND user_id = OLD.user_id
    )
)
BEGIN
    SELECT RAISE(ABORT, 'immutable attribution history');
END;

CREATE INDEX idx_event_credit_revisions_user_event
    ON event_credit_revisions (user_id, event_id, event_version);
