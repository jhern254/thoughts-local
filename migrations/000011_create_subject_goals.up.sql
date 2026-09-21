-- Classification links are retained when either endpoint is soft-deleted.
-- Linking does not enable automatic credit or create goal progress.
CREATE TABLE IF NOT EXISTS subject_goals (
    subject_id INTEGER NOT NULL,
    goal_id    INTEGER NOT NULL,
    user_id    TEXT NOT NULL,

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

CREATE INDEX IF NOT EXISTS idx_subject_goals_goal_subject
    ON subject_goals (goal_id, subject_id);
