package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
)

// UNION visits each descendant once, including inactive and deleted descendants
// whose earned contributions still belong in the requested Goal's history.
const goalLineageCTE = `
    WITH RECURSIVE requested_goal(goal_id) AS (
        SELECT g.goal_id
        FROM goals g
        JOIN users u ON u.user_id = g.user_id
        WHERE g.user_id = ? AND g.goal_id = ?
            AND g.deleted_at IS NULL AND u.deleted_at IS NULL
    ), reachable_goals(goal_id) AS (
        SELECT goal_id FROM requested_goal
        UNION
        SELECT p.goal_id
        FROM goal_parents p
        JOIN reachable_goals r ON r.goal_id = p.parent_goal_id
        WHERE p.user_id = ?
    )`

// Gather this Goal's earned Event IDs before reading Events. A correlated
// membership check would repeatedly scan contributions for an empty history.
const earnedEventSelect = `, earned_events(event_id) AS (
        SELECT DISTINCT p.event_id
        FROM goal_progress p
        JOIN reachable_goals r ON r.goal_id = p.goal_id
        WHERE p.user_id = ?
    )
    SELECT
        e.event_id,
        substr(COALESCE(e.activity_type, ''), 1, 81),
        e.started_at,
        e.ended_at,
        e.ended_at - e.started_at
    FROM earned_events earned
    JOIN events e ON e.event_id = earned.event_id
    WHERE e.user_id = ? AND e.deleted_at IS NULL AND e.ended_at IS NOT NULL`

func (s *SQLiteGoalProgressStore) ListEarnedEvents(ctx context.Context, userID string, goalID int64, request GoalEventPageRequest) (GoalEventPage, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return GoalEventPage{}, err
	}
	defer tx.Rollback()
	if err := requireLineageGoal(ctx, tx, userID, goalID); err != nil {
		return GoalEventPage{}, err
	}
	query := goalLineageCTE + earnedEventSelect
	args := []any{userID, goalID, userID, userID, userID}
	comparison, order := "<", " DESC"
	if request.Direction == EarnedEventsNewer {
		comparison, order = ">", " ASC"
	}
	if cursor := request.Cursor; cursor != nil {
		query += " AND (e.started_at, e.event_id) " + comparison + " (?, ?)"
		args = append(args, cursor.StartedAt.Unix(), cursor.EventID)
	}
	query += " ORDER BY e.started_at" + order + ", e.event_id" + order + " LIMIT ?"
	args = append(args, GoalEventPageSize+1)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return GoalEventPage{}, fmt.Errorf("list earned Events: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	page := GoalEventPage{Events: make([]GoalEventLineage, 0, GoalEventPageSize+1)}
	for rows.Next() {
		item, err := scanEarnedEvent(rows)
		if err != nil {
			return GoalEventPage{}, err
		}
		page.Events = append(page.Events, item)
	}
	if err := rows.Err(); err != nil {
		return GoalEventPage{}, err
	}
	page.More = len(page.Events) > GoalEventPageSize
	if page.More {
		page.Events = page.Events[:GoalEventPageSize]
	}
	if request.Direction == EarnedEventsNewer {
		slices.Reverse(page.Events)
	}
	if err := tx.Commit(); err != nil {
		return GoalEventPage{}, err
	}
	return page, nil
}

func requireLineageGoal(ctx context.Context, tx *sql.Tx, userID string, goalID int64) error {
	var id int64
	err := tx.QueryRowContext(ctx, `
    SELECT g.goal_id
    FROM goals g
    JOIN users u ON u.user_id = g.user_id
    WHERE g.user_id = ? AND g.goal_id = ? AND g.deleted_at IS NULL AND u.deleted_at IS NULL`, userID, goalID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRecordNotFound
	}
	return err
}

func scanEarnedEvent(row interface{ Scan(...any) error }) (GoalEventLineage, error) {
	var item GoalEventLineage
	var start, end int64
	// Scanning elapsed seconds into int64 also rejects SQLite's floating-point
	// result when subtracting extreme timestamps overflows its integer range.
	if err := row.Scan(&item.EventID, &item.Activity, &start, &end, &item.ElapsedSeconds); err != nil {
		return item, fmt.Errorf("read earned Event: %w", err)
	}
	if end < start || item.ElapsedSeconds < 0 {
		return item, ErrInvalidEventInterval
	}
	item.StartedAt, item.EndedAt = TimeFromUnixSec(start), TimeFromUnixSec(end)
	preview := []rune(item.Activity)
	if len(preview) > 80 {
		item.Activity = string(preview[:80]) + "…"
	}
	return item, nil
}

func readEarnedEvent(ctx context.Context, tx *sql.Tx, userID string, goalID, eventID int64) (GoalEventLineage, error) {
	item, err := scanEarnedEvent(tx.QueryRowContext(ctx, goalLineageCTE+earnedEventSelect+" AND e.event_id = ?", userID, goalID, userID, userID, userID, eventID))
	if errors.Is(err, sql.ErrNoRows) {
		return GoalEventLineage{}, ErrRecordNotFound
	}
	return item, err
}

// Match only the surviving Subject reference. recorded_subject_id is historical
// identification and must never attach Thoughts from a new Subject reusing it.
// EXISTS keeps a Thought unique even when several descendants earned it.
const lineageThoughtCondition = `
    AND EXISTS (
        SELECT 1
        FROM goal_progress p
        JOIN reachable_goals r ON r.goal_id = p.goal_id
        JOIN subjects contributing_subject ON contributing_subject.subject_id = p.subject_id
            AND contributing_subject.user_id = p.user_id
            AND contributing_subject.deleted_at IS NULL
        WHERE p.user_id = t.user_id AND p.event_id = ? AND p.subject_id = t.subject_id
    )`

func (s *SQLiteGoalProgressStore) ListEventLineageThoughts(ctx context.Context, userID string, goalID, eventID int64, request ThoughtSummaryViewRequest) (GoalEventThoughtPage, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return GoalEventThoughtPage{}, err
	}
	defer tx.Rollback()
	item, err := readEarnedEvent(ctx, tx, userID, goalID, eventID)
	if err != nil {
		return GoalEventThoughtPage{}, err
	}
	query := goalLineageCTE + eventThoughtSummarySelect + `
        AND t.observed_at >= ? AND t.observed_at < ?` + lineageThoughtCondition
	args := []any{userID, goalID, userID, userID, item.StartedAt.Unix(), item.EndedAt.Unix(), eventID}
	thoughts, err := browseThoughtSummaries(ctx, tx, query, args, request, ThoughtSummaryViewBatchSize)
	if err != nil {
		return GoalEventThoughtPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return GoalEventThoughtPage{}, err
	}
	return GoalEventThoughtPage{Event: item, Thoughts: thoughts}, nil
}

func (s *SQLiteGoalProgressStore) GetEventLineageThought(ctx context.Context, userID string, goalID, eventID, thoughtID int64) (*Thought, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	item, err := readEarnedEvent(ctx, tx, userID, goalID, eventID)
	if err != nil {
		return nil, err
	}
	// The existing full Thought projection includes its current Mindset state.
	// Qualify membership in the same read transaction before returning its body.
	var matched int64
	err = tx.QueryRowContext(ctx, goalLineageCTE+`
    SELECT t.thought_id
    FROM thoughts t
    WHERE t.user_id = ? AND t.thought_id = ? AND t.deleted_at IS NULL
        AND t.observed_at >= ? AND t.observed_at < ?`+lineageThoughtCondition,
		userID, goalID, userID, userID, thoughtID, item.StartedAt.Unix(), item.EndedAt.Unix(), eventID).Scan(&matched)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	thought, err := scanThought(tx.QueryRowContext(ctx, thoughtSelect+" WHERE user_id = ? AND thought_id = ?", userID, matched))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return thought, nil
}
