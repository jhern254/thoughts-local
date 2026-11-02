-- The old schema cannot represent retirement or stable numbers. Retain all
-- endpoint pairs when downgrading; retirement state requires a backup to restore.
CREATE TABLE subject_goals_legacy (
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

INSERT INTO subject_goals_legacy (subject_id, goal_id, user_id)
SELECT subject_id, goal_id, user_id FROM subject_goals;
DROP TABLE subject_goals;
ALTER TABLE subject_goals_legacy RENAME TO subject_goals;
CREATE INDEX idx_subject_goals_goal_subject
    ON subject_goals (goal_id, subject_id);
