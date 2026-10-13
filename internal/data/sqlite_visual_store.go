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
	settings := Visual{
		Darkness: DefaultBackgroundDarkness,
		Framing:  DefaultBackgroundFraming(),
	}
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(background_asset, ''), darkness, fit, zoom, position_x, position_y
 FROM visual WHERE user_id=?`, userID).Scan(
		&settings.BackgroundAsset, &settings.Darkness, &settings.Framing.Fit,
		&settings.Framing.Zoom, &settings.Framing.PositionX, &settings.Framing.PositionY,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	return settings, TranslateSQLiteError(err)
}
func (s *SQLiteVisualStore) Save(ctx context.Context, userID string, settings Visual) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO visual(user_id,background_asset,darkness,fit,zoom,position_x,position_y) VALUES (?,NULLIF(?,''),?,?,?,?,?)
 ON CONFLICT(user_id) DO UPDATE SET
 background_asset=excluded.background_asset,darkness=excluded.darkness,fit=excluded.fit,
 zoom=excluded.zoom,position_x=excluded.position_x,position_y=excluded.position_y`,
		userID, settings.BackgroundAsset, settings.Darkness, settings.Framing.Fit,
		settings.Framing.Zoom, settings.Framing.PositionX, settings.Framing.PositionY)
	return TranslateSQLiteError(err)
}
