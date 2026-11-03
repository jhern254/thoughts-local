-- Attribution snapshots are appended atomically by the Event transaction.
CREATE TABLE goal_event_credit_subjects (
    support_id INTEGER PRIMARY KEY,
    event_id INTEGER NOT NULL,
    event_version INTEGER NOT NULL,
    goal_id INTEGER NOT NULL,
    user_id TEXT NOT NULL,
    recorded_subject_id INTEGER NOT NULL CHECK (typeof(recorded_subject_id) = 'integer'),
    subject_id INTEGER,
    attributed_at INTEGER NOT NULL CHECK (typeof(attributed_at) = 'integer'),
    UNIQUE (event_id, event_version, goal_id, subject_id),
    FOREIGN KEY (event_id, event_version, goal_id, user_id) REFERENCES goal_event_credits (event_id, event_version, goal_id, user_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (subject_id) REFERENCES subjects (subject_id) ON DELETE SET NULL ON UPDATE CASCADE,
    FOREIGN KEY (subject_id, user_id) REFERENCES subjects (subject_id, user_id)
);

-- Identity maintenance is allowed only after the old referenced key disappears.
CREATE TRIGGER goal_event_credit_subjects_immutable_update
BEFORE UPDATE ON goal_event_credit_subjects
WHEN (
    NEW.support_id IS NOT OLD.support_id
    OR NEW.event_version IS NOT OLD.event_version
    OR NEW.recorded_subject_id IS NOT OLD.recorded_subject_id
    OR NEW.attributed_at IS NOT OLD.attributed_at
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
            FROM goal_event_credits
            WHERE event_id = OLD.event_id
                AND event_version = OLD.event_version
                AND goal_id = OLD.goal_id
                AND user_id = OLD.user_id
        )
    )
    AND NOT (
        OLD.subject_id IS NOT NULL
        AND NEW.subject_id IS NOT OLD.subject_id
        AND NOT EXISTS (
            SELECT 1
            FROM subjects
            WHERE subject_id = OLD.subject_id
        )
        AND NEW.event_id IS OLD.event_id
        AND NEW.goal_id IS OLD.goal_id
        AND NEW.user_id IS OLD.user_id
    )
)
BEGIN
    SELECT RAISE(ABORT, 'immutable attribution history');
END;

CREATE TRIGGER goal_event_credit_subjects_immutable_delete
BEFORE DELETE ON goal_event_credit_subjects
WHEN EXISTS (
    SELECT 1
    FROM users
    WHERE user_id = OLD.user_id
)
AND (
    EXISTS (
        SELECT 1
        FROM goal_event_credits
        WHERE event_id = OLD.event_id
            AND event_version = OLD.event_version
            AND goal_id = OLD.goal_id
            AND user_id = OLD.user_id
    )
)
BEGIN
    SELECT RAISE(ABORT, 'immutable attribution history');
END;
