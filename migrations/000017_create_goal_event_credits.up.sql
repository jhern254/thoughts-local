-- Attribution snapshots are appended atomically by the Event transaction.
CREATE TABLE goal_event_credits (
    event_id INTEGER NOT NULL,
    event_version INTEGER NOT NULL,
    goal_id INTEGER NOT NULL,
    user_id TEXT NOT NULL,
    PRIMARY KEY (event_id, event_version, goal_id),
    UNIQUE (event_id, event_version, goal_id, user_id),
    FOREIGN KEY (event_id, event_version, user_id) REFERENCES event_credit_revisions (event_id, event_version, user_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (goal_id, user_id) REFERENCES goals (goal_id, user_id) ON DELETE CASCADE ON UPDATE CASCADE
);

-- Identity maintenance is allowed only after the old referenced key disappears.
CREATE TRIGGER goal_event_credits_immutable_update
BEFORE UPDATE ON goal_event_credits
WHEN (NEW.event_version IS NOT OLD.event_version)
OR (
    EXISTS (
        SELECT 1
        FROM users
        WHERE user_id = OLD.user_id
    )
    AND (
        EXISTS (
            SELECT 1
            FROM event_credit_revisions
            WHERE event_id = OLD.event_id
                AND event_version = OLD.event_version
                AND user_id = OLD.user_id
        )
        AND EXISTS (
            SELECT 1
            FROM goals
            WHERE goal_id = OLD.goal_id
                AND user_id = OLD.user_id
        )
    )
)
BEGIN
    SELECT RAISE(ABORT, 'immutable attribution history');
END;

CREATE TRIGGER goal_event_credits_immutable_delete
BEFORE DELETE ON goal_event_credits
WHEN EXISTS (
    SELECT 1
    FROM users
    WHERE user_id = OLD.user_id
)
AND (
    EXISTS (
        SELECT 1
        FROM event_credit_revisions
        WHERE event_id = OLD.event_id
            AND event_version = OLD.event_version
            AND user_id = OLD.user_id
    )
    AND EXISTS (
        SELECT 1
        FROM goals
        WHERE goal_id = OLD.goal_id
            AND user_id = OLD.user_id
    )
)
BEGIN
    SELECT RAISE(ABORT, 'immutable attribution history');
END;

CREATE INDEX idx_goal_event_credits_goal ON goal_event_credits (user_id, goal_id, event_id, event_version);
