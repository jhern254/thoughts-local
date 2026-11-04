-- Events own time; these rows retain the Subjects that established earned Goal contributions.
CREATE TABLE goal_progress (
    progress_id INTEGER PRIMARY KEY,
    event_id INTEGER NOT NULL,
    goal_id INTEGER NOT NULL,
    user_id TEXT NOT NULL,
    recorded_subject_id INTEGER NOT NULL CHECK (typeof(recorded_subject_id) = 'integer'),
    subject_id INTEGER,
    UNIQUE (event_id, goal_id, subject_id),
    FOREIGN KEY (user_id) REFERENCES users (user_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (event_id) REFERENCES events (event_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (goal_id) REFERENCES goals (goal_id) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (subject_id) REFERENCES subjects (subject_id) ON DELETE SET NULL ON UPDATE CASCADE,
    -- Separate ownership checks prevent endpoint owner changes from transferring contributions.
    FOREIGN KEY (event_id, user_id) REFERENCES events (event_id, user_id),
    FOREIGN KEY (goal_id, user_id) REFERENCES goals (goal_id, user_id),
    FOREIGN KEY (subject_id, user_id) REFERENCES subjects (subject_id, user_id)
);

CREATE INDEX idx_goal_progress_goal ON goal_progress (user_id, goal_id, event_id);
CREATE INDEX idx_goal_progress_subject ON goal_progress (subject_id, user_id);
