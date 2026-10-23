package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SetThoughtMindset keeps designation history independent of content revisions.
// The transaction's snapshot protects the read/transition pair: a competing
// writer must either serialize or return SQLITE_BUSY, never overwrite history.
func (s *SQLiteThoughtStore) SetThoughtMindset(ctx context.Context, userID string, id int64, active bool, at time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, TranslateSQLiteError(err)
	}
	defer tx.Rollback()
	var exists bool
	err = tx.QueryRowContext(ctx, `
        SELECT EXISTS (
            SELECT 1
            FROM thoughts t
            JOIN users u ON u.user_id = t.user_id
            WHERE t.thought_id = ? AND t.user_id = ?
                AND t.deleted_at IS NULL AND u.deleted_at IS NULL
        )`, id, userID).Scan(&exists)
	if err != nil {
		return false, TranslateSQLiteError(err)
	}
	if !exists {
		return false, ErrRecordNotFound
	}
	var periodID, started int64
	var ended sql.NullInt64
	err = tx.QueryRowContext(ctx, `
        SELECT mindset_period_id, started_at, ended_at
        FROM thought_mindset_periods
        WHERE thought_id = ?
        ORDER BY started_at DESC, mindset_period_id DESC
        LIMIT 1`, id).Scan(&periodID, &started, &ended)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, TranslateSQLiteError(err)
	}
	hasPeriod := err == nil
	current := hasPeriod && !ended.Valid
	if current == active {
		return current, TranslateSQLiteError(tx.Commit())
	}
	if hasPeriod {
		latest := started
		if ended.Valid {
			latest = ended.Int64
		}
		if at.Unix() < latest {
			return false, ErrMindsetChronology
		}
	}
	if active {
		_, err = tx.ExecContext(ctx, `
            INSERT INTO thought_mindset_periods (thought_id, started_at)
            VALUES (?, ?)`, id, at.Unix())
	} else {
		_, err = tx.ExecContext(ctx, `
            UPDATE thought_mindset_periods
            SET ended_at = ?
            WHERE mindset_period_id = ?`, at.Unix(), periodID)
	}
	if err != nil {
		return false, fmt.Errorf("set thought mindset: %w", TranslateSQLiteError(err))
	}
	if err = tx.Commit(); err != nil {
		return false, TranslateSQLiteError(err)
	}
	return active, nil
}

// ListMindsetsInRange includes intervals intersecting [from, until), plus
// same-second mark/unmark actions on that day. Ranking collapses reactivations
// to their latest overlapping period without losing the earlier history.
func (s *SQLiteThoughtStore) ListMindsetsInRange(ctx context.Context, userID string, from, until time.Time) ([]MindsetThought, error) {
	rows, err := s.db.QueryContext(ctx, `
        WITH overlapping AS (
            SELECT p.thought_id, p.started_at, p.mindset_period_id,
                row_number() OVER (
                    PARTITION BY p.thought_id
                    ORDER BY p.started_at DESC, p.mindset_period_id DESC
                ) AS recency
            FROM thought_mindset_periods p
            JOIN thoughts t ON t.thought_id = p.thought_id
            JOIN users u ON u.user_id = t.user_id
            WHERE t.user_id = ? AND t.deleted_at IS NULL AND u.deleted_at IS NULL
                AND p.started_at < ?
                AND (
                    p.ended_at IS NULL OR p.ended_at > ?
                    OR (p.ended_at = p.started_at AND p.started_at >= ?)
                )
        )
        SELECT t.thought_id, t.thought, s.subject_name,
            t.observed_at, t.created_at,
            EXISTS (
                SELECT 1 FROM thought_mindset_periods current
                WHERE current.thought_id = t.thought_id AND current.ended_at IS NULL
            )
        FROM overlapping p
        JOIN thoughts t ON t.thought_id = p.thought_id
        LEFT JOIN subjects s ON s.subject_id = t.subject_id
            AND s.user_id = t.user_id AND s.deleted_at IS NULL
        WHERE p.recency = 1
        ORDER BY p.started_at, p.mindset_period_id`, userID, until.Unix(), from.Unix(), from.Unix())
	if err != nil {
		return nil, fmt.Errorf("list mindsets: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	items := []MindsetThought{}
	for rows.Next() {
		var item MindsetThought
		var subject sql.NullString
		var observed, created int64
		if err := rows.Scan(&item.ThoughtID, &item.Text, &subject, &observed, &created, &item.IsMindset); err != nil {
			return nil, TranslateSQLiteError(err)
		}
		if subject.Valid {
			item.SubjectName = &subject.String
		}
		item.ObservedAt, item.CreatedAt = TimeFromUnixSec(observed), TimeFromUnixSec(created)
		items = append(items, item)
	}
	return items, TranslateSQLiteError(rows.Err())
}
