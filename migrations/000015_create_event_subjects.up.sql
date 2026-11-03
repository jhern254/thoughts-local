-- One retained relationship per same-owner Event–Subject pair.
CREATE TABLE IF NOT EXISTS event_subjects (
    event_id   INTEGER NOT NULL,
    subject_id INTEGER NOT NULL,
    user_id    TEXT    NOT NULL,
    deleted_at INTEGER
        CHECK (deleted_at IS NULL OR typeof(deleted_at) = 'integer'),

    CONSTRAINT pk_event_subjects PRIMARY KEY (event_id, subject_id),
    CONSTRAINT fk_event_subjects_event
        FOREIGN KEY (event_id, user_id) REFERENCES events(event_id, user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE,
    CONSTRAINT fk_event_subjects_subject
        FOREIGN KEY (subject_id, user_id) REFERENCES subjects(subject_id, user_id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_event_subjects_subject_event
    ON event_subjects (subject_id, event_id);
