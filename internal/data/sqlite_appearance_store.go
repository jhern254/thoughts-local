package data

import (
	"context"
	"database/sql"
	"errors"
)

type SQLiteAppearanceStore struct{ db *sql.DB }

func NewSQLiteAppearanceStore(db *sql.DB) *SQLiteAppearanceStore {
	return &SQLiteAppearanceStore{db: db}
}
func (s *SQLiteAppearanceStore) Load(ctx context.Context, userID string) (Appearance, error) {
	settings := Appearance{Darkness: DefaultBackgroundDarkness}
	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(background_asset, ''), darkness FROM browser_appearance WHERE user_id=?", userID).Scan(&settings.BackgroundAsset, &settings.Darkness)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	return settings, TranslateSQLiteError(err)
}
func (s *SQLiteAppearanceStore) Save(ctx context.Context, userID string, settings Appearance) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO browser_appearance(user_id,background_asset,darkness) VALUES (?,NULLIF(?,''),?)
 ON CONFLICT(user_id) DO UPDATE SET background_asset=excluded.background_asset,darkness=excluded.darkness`, userID, settings.BackgroundAsset, settings.Darkness)
	return TranslateSQLiteError(err)
}
