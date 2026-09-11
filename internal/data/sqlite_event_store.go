package data

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
)

// SQLiteEventStore persists one activity track per user. Callers supply explicit
// timestamps; defaults, future-time validation, and input normalization belong
// to the service. Every interval check and corresponding write share a transaction.
// Competing SQLite writers may return ErrDatabaseBusy; no automatic retries occur.
type SQLiteEventStore struct{ db *sql.DB }

func NewSQLiteEventStore(db *sql.DB) *SQLiteEventStore { return &SQLiteEventStore{db: db} }

const eventColumns = `event_id, user_id, activity_type, started_at, ended_at, version, created_at, updated_at`
const activeEvents = ` FROM events WHERE deleted_at IS NULL
	AND EXISTS (SELECT 1 FROM users WHERE users.user_id = events.user_id AND users.deleted_at IS NULL)`

func (s *SQLiteEventStore) GetEvent(ctx context.Context, userID string, id int64) (*Event, error) {
	item, err := scanEvent(s.db.QueryRowContext(ctx, `SELECT `+eventColumns+activeEvents+
		` AND user_id = ? AND event_id = ?`, userID, id))
	if err != nil {
		return nil, fmt.Errorf("get event: %w", TranslateSQLiteError(err))
	}
	return item, nil
}

// StartEvent closes the current event at the new start and inserts its successor
// atomically. It never changes completed events or the caller's input object.
func (s *SQLiteEventStore) StartEvent(ctx context.Context, item *Event) (_ *Event, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("start event: %w", TranslateSQLiteError(err))
		}
	}()
	if item.EndedAt != nil {
		return nil, ErrInvalidEventInterval
	}
	tx, conn, err := beginEventWrite(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	defer tx.Rollback()
	if err := requireEventOwner(ctx, tx, item.UserID); err != nil {
		return nil, err
	}
	previous, err := scanEvent(tx.QueryRowContext(ctx, `SELECT `+eventColumns+activeEvents+
		` AND user_id = ? AND ended_at IS NULL`, item.UserID))
	if err != nil && !errors.Is(err, ErrRecordNotFound) {
		return nil, err
	}
	if previous != nil {
		if item.StartedAt.Unix() < previous.StartedAt.Unix() {
			return nil, ErrEventOverlap
		}
		if _, err := tx.ExecContext(ctx, `UPDATE events SET ended_at = ?, version = version + 1,
			updated_at = max(updated_at, ?) WHERE event_id = ? AND user_id = ?`,
			item.StartedAt.Unix(), item.UpdatedAt.Unix(), previous.EventID, item.UserID); err != nil {
			return nil, err
		}
	}
	if err := checkEventOverlap(ctx, tx, item, nil); err != nil {
		return nil, err
	}
	created, err := insertEvent(ctx, tx, item)
	if err != nil {
		return nil, err
	}
	if err := commitEventWrite(tx, conn); err != nil {
		return nil, err
	}
	return created, nil
}

// AddPastEvent inserts a completed interval without changing the ongoing event.
func (s *SQLiteEventStore) AddPastEvent(ctx context.Context, item *Event) (_ *Event, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("add past event: %w", TranslateSQLiteError(err))
		}
	}()
	if item.EndedAt == nil {
		return nil, ErrInvalidEventInterval
	}
	tx, conn, err := beginEventWrite(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	defer tx.Rollback()
	if err := requireEventOwner(ctx, tx, item.UserID); err != nil {
		return nil, err
	}
	if err := checkEventOverlap(ctx, tx, item, nil); err != nil {
		return nil, err
	}
	created, err := insertEvent(ctx, tx, item)
	if err != nil {
		return nil, err
	}
	if err := commitEventWrite(tx, conn); err != nil {
		return nil, err
	}
	return created, nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, item *Event) (*Event, error) {
	return scanEvent(tx.QueryRowContext(ctx, `INSERT INTO events
		(user_id, activity_type, started_at, ended_at, created_at, updated_at)
		SELECT ?, ?, ?, ?, ?, ? WHERE EXISTS
		(SELECT 1 FROM users WHERE user_id = ? AND deleted_at IS NULL)
		RETURNING `+eventColumns, item.UserID, item.ActivityType, item.StartedAt.Unix(),
		eventEndSeconds(item.EndedAt), item.CreatedAt.Unix(), item.UpdatedAt.Unix(), item.UserID))
}

func requireEventOwner(ctx context.Context, tx *sql.Tx, userID string) error {
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE user_id = ? AND deleted_at IS NULL)`, userID).Scan(&active); err != nil {
		return err
	}
	if !active {
		return ErrRecordNotFound
	}
	return nil
}

// ListEvents returns events intersecting [from, until), ordered by start then ID.
// Zero-duration events are included when their start lies within the range.
// The caller computes calendar boundaries; persistence has no timezone policy.
func (s *SQLiteEventStore) ListEvents(ctx context.Context, userID string, from, until time.Time) (_ []Event, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("list events: %w", TranslateSQLiteError(err))
		}
	}()
	if until.Unix() <= from.Unix() {
		return nil, ErrInvalidEventInterval
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+activeEvents+`
		AND user_id = ? AND started_at < ?
		AND (ended_at IS NULL OR ended_at > ? OR (ended_at = started_at AND started_at >= ?))
		ORDER BY started_at, event_id`, userID, until.Unix(), from.Unix(), from.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Event, 0)
	for rows.Next() {
		item, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLiteEventStore) EndEvent(ctx context.Context, userID string, id, version int64, end, updatedAt time.Time) (_ *Event, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("end event: %w", TranslateSQLiteError(err))
		}
	}()
	tx, conn, err := beginEventWrite(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	defer tx.Rollback()
	item, err := eventForEdit(ctx, tx, userID, id, version)
	if err != nil {
		return nil, err
	}
	if item.EndedAt != nil {
		return nil, ErrEventStateConflict
	}
	item.EndedAt, item.UpdatedAt = &end, updatedAt
	result, err := updateEvent(ctx, tx, item)
	if err != nil {
		return nil, err
	}
	if err := commitEventWrite(tx, conn); err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateEvent edits label/times without changing lifecycle state. Use EndEvent
// to finish an ongoing event; reopening completed events is not supported.
func (s *SQLiteEventStore) UpdateEvent(ctx context.Context, item *Event) (_ *Event, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("update event: %w", TranslateSQLiteError(err))
		}
	}()
	tx, conn, err := beginEventWrite(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	defer tx.Rollback()
	current, err := eventForEdit(ctx, tx, item.UserID, item.EventID, item.Version)
	if err != nil {
		return nil, err
	}
	if (current.EndedAt == nil) != (item.EndedAt == nil) {
		return nil, ErrEventStateConflict
	}
	result, err := updateEvent(ctx, tx, item)
	if err != nil {
		return nil, err
	}
	if err := commitEventWrite(tx, conn); err != nil {
		return nil, err
	}
	return result, nil
}

func eventForEdit(ctx context.Context, tx *sql.Tx, userID string, id, version int64) (*Event, error) {
	item, err := scanEvent(tx.QueryRowContext(ctx, `SELECT `+eventColumns+activeEvents+
		` AND user_id = ? AND event_id = ?`, userID, id))
	if err != nil {
		return nil, err
	}
	if item.Version != version {
		return nil, ErrEventVersionConflict
	}
	return item, nil
}

// Retain the connection until commit cleanup completes. modernc v1.39 leaves
// SQLite's transaction active on a busy COMMIT, but database/sql marks Tx done,
// so Tx.Rollback cannot recover it. This is deliberately local to event writes.
func beginEventWrite(ctx context.Context, db *sql.DB) (*sql.Tx, *sql.Conn, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return tx, conn, nil
}

func commitEventWrite(tx *sql.Tx, conn *sql.Conn) error {
	err := tx.Commit()
	if err == nil {
		return nil
	}
	// Cleanup must run even if the request was canceled. If SQLite already
	// rolled back, this may fail harmlessly; discard the connection rather than
	// risk returning uncertain transaction state to the pool.
	if _, cleanupErr := conn.ExecContext(context.Background(), "ROLLBACK"); cleanupErr != nil {
		conn.Raw(func(any) error { return driver.ErrBadConn })
		return errors.Join(TranslateSQLiteError(err), fmt.Errorf("rollback event commit: %w", TranslateSQLiteError(cleanupErr)))
	}
	return err
}

func updateEvent(ctx context.Context, tx *sql.Tx, item *Event) (*Event, error) {
	if err := checkEventOverlap(ctx, tx, item, &item.EventID); err != nil {
		return nil, err
	}
	return scanEvent(tx.QueryRowContext(ctx, `UPDATE events SET activity_type = ?, started_at = ?, ended_at = ?,
		version = version + 1, updated_at = max(updated_at, ?)
		WHERE user_id = ? AND event_id = ? AND version = ? AND deleted_at IS NULL
		RETURNING `+eventColumns, item.ActivityType, item.StartedAt.Unix(), eventEndSeconds(item.EndedAt),
		item.UpdatedAt.Unix(), item.UserID, item.EventID, item.Version))
}

// Empty intervals do not overlap. Nonempty intervals are [start, end), with
// NULL end unbounded. excludeID is the record being edited (nil on create).
func checkEventOverlap(ctx context.Context, tx *sql.Tx, item *Event, excludeID *int64) error {
	start, end := item.StartedAt.Unix(), eventEndSeconds(item.EndedAt)
	if end != nil {
		if *end < start {
			return ErrInvalidEventInterval
		}
		if *end == start {
			return nil
		}
	}
	var overlap bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM events
		WHERE user_id = ? AND deleted_at IS NULL AND (? IS NULL OR event_id <> ?)
		AND (ended_at IS NULL OR ended_at > started_at)
		AND (? IS NULL OR started_at < ?) AND (ended_at IS NULL OR ended_at > ?))`,
		item.UserID, excludeID, excludeID, end, end, start).Scan(&overlap)
	if err != nil {
		return err
	}
	if overlap {
		return ErrEventOverlap
	}
	return nil
}

func eventEndSeconds(end *time.Time) *int64 {
	if end == nil {
		return nil
	}
	seconds := end.Unix()
	return &seconds
}

type eventScanner interface{ Scan(...any) error }

func scanEvent(row eventScanner) (*Event, error) {
	var item Event
	var start, created, updated int64
	var end sql.NullInt64
	if err := row.Scan(&item.EventID, &item.UserID, &item.ActivityType, &start, &end,
		&item.Version, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}
	item.StartedAt, item.CreatedAt, item.UpdatedAt = TimeFromUnixSec(start), TimeFromUnixSec(created), TimeFromUnixSec(updated)
	if end.Valid {
		ended := TimeFromUnixSec(end.Int64)
		item.EndedAt = &ended
	}
	return &item, nil
}
