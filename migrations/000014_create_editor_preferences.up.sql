CREATE TABLE editor_preferences (
    user_id TEXT PRIMARY KEY NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    highlight_current_line INTEGER NOT NULL DEFAULT 0
        CHECK (typeof(highlight_current_line) = 'integer' AND highlight_current_line IN (0, 1)),
    speech_pause_ms INTEGER NOT NULL DEFAULT 1500
        CHECK (typeof(speech_pause_ms) = 'integer' AND speech_pause_ms BETWEEN 0 AND 10000 AND speech_pause_ms % 500 = 0)
);
