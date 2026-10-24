package data

import (
	"context"
	"database/sql"
	"errors"
)

type SQLiteEditorPreferencesStore struct{ db *sql.DB }

func NewSQLiteEditorPreferencesStore(db *sql.DB) *SQLiteEditorPreferencesStore {
	return &SQLiteEditorPreferencesStore{db: db}
}
func (s *SQLiteEditorPreferencesStore) Load(ctx context.Context, userID string) (EditorPreferences, error) {
	preferences := DefaultEditorPreferences()
	err := s.db.QueryRowContext(ctx, `SELECT highlight_current_line,speech_pause_ms FROM editor_preferences WHERE user_id=?`, userID).Scan(&preferences.HighlightCurrentLine, &preferences.SpeechPauseMS)
	if errors.Is(err, sql.ErrNoRows) {
		return preferences, nil
	}
	return preferences, TranslateSQLiteError(err)
}
func (s *SQLiteEditorPreferencesStore) Save(ctx context.Context, userID string, preferences EditorPreferences) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO editor_preferences(user_id,highlight_current_line,speech_pause_ms) VALUES (?,?,?)
 ON CONFLICT(user_id) DO UPDATE SET highlight_current_line=excluded.highlight_current_line,speech_pause_ms=excluded.speech_pause_ms`, userID, preferences.HighlightCurrentLine, preferences.SpeechPauseMS)
	return TranslateSQLiteError(err)
}
