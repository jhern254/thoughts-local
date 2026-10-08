package data

import (
	"context"
	"database/sql"
	"errors"
)

type SQLiteVisualStore struct{ db *sql.DB }

func NewSQLiteVisualStore(db *sql.DB) *SQLiteVisualStore {
	return &SQLiteVisualStore{db: db}
}
func (s *SQLiteVisualStore) Load(ctx context.Context, userID string) (Visual, error) {
	settings := Visual{Darkness: DefaultBackgroundDarkness}
	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(background_asset, ''), darkness FROM browser_appearance WHERE user_id=?", userID).Scan(&settings.BackgroundAsset, &settings.Darkness)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	return settings, TranslateSQLiteError(err)
}
func (s *SQLiteVisualStore) Save(ctx context.Context, userID string, settings Visual) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO browser_appearance(user_id,background_asset,darkness) VALUES (?,NULLIF(?,''),?)
 ON CONFLICT(user_id) DO UPDATE SET background_asset=excluded.background_asset,darkness=excluded.darkness`, userID, settings.BackgroundAsset, settings.Darkness)
	return TranslateSQLiteError(err)
}
