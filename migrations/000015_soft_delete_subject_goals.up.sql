-- Upgrade the original relationship table without changing endpoint records.
CREATE TABLE subject_goals_next (
    subject_id INTEGER NOT NULL,
    goal_id    INTEGER NOT NULL,
    user_id    TEXT NOT NULL,
    goal_number INTEGER NOT NULL
        CHECK (typeof(goal_number) = 'integer' AND goal_number > 0),
    deleted_at INTEGER
        CHECK (deleted_at IS NULL OR typeof(deleted_at) = 'integer'),

    CONSTRAINT uq_subject_goals_number UNIQUE (subject_id, goal_number),

    CONSTRAINT pk_subject_goals PRIMARY KEY (subject_id, goal_id),
    CONSTRAINT fk_subject_goals_subject
        FOREIGN KEY (subject_id, user_id) REFERENCES subjects(subject_id, user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE,
    CONSTRAINT fk_subject_goals_goal
        FOREIGN KEY (goal_id, user_id) REFERENCES goals(goal_id, user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);

INSERT INTO subject_goals_next (subject_id, goal_id, user_id, goal_number)
SELECT
    subject_id,
    goal_id,
    user_id,
    ROW_NUMBER() OVER (PARTITION BY subject_id ORDER BY goal_id)
FROM subject_goals;

DROP TABLE subject_goals;
ALTER TABLE subject_goals_next RENAME TO subject_goals;

CREATE INDEX idx_subject_goals_goal_subject
    ON subject_goals (goal_id, subject_id);
