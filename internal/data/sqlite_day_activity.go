package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// MaxActivityBuckets bounds query parameters and the presentation projection.
const MaxActivityBuckets = 256

// ActivityCount totals calendar Event-days and observed Thoughts in one bucket.
// A multi-day Event contributes once to each day it touches, not once per bucket.
type ActivityCount struct{ EventDays, Thoughts int64 }

func (s *SQLiteMetricsStore) ActivityStart(ctx context.Context, userID string, cutoff time.Time) (time.Time, error) {
	var first sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT min(at) FROM (
 SELECT min(e.started_at) AS at FROM events e JOIN users u ON u.user_id=e.user_id
 WHERE e.user_id=? AND e.deleted_at IS NULL AND u.deleted_at IS NULL AND e.started_at<?
 UNION ALL
 SELECT min(t.observed_at) AS at FROM thoughts t JOIN users u ON u.user_id=t.user_id
 WHERE t.user_id=? AND t.deleted_at IS NULL AND u.deleted_at IS NULL AND t.observed_at<?
 )`, userID, cutoff.Unix(), userID, cutoff.Unix()).Scan(&first)
	if err != nil {
		return time.Time{}, fmt.Errorf("read activity start: %w", TranslateSQLiteError(err))
	}
	if !first.Valid {
		return time.Time{}, nil
	}
	return TimeFromUnixSec(first.Int64), nil
}

// CalendarDays counts civil dates rather than elapsed 24-hour periods. The
// boundary location owns the calendar; DST days may have 23 or 25 hours.
func CalendarDays(from, until time.Time) int {
	y, m, d := from.Date()
	a := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	y, m, d = until.In(from.Location()).Date()
	b := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return int((b.Unix() - a.Unix()) / 86400)
}

func (s *SQLiteMetricsStore) ActivityCounts(ctx context.Context, userID string, boundaries []time.Time, cutoff time.Time) ([]ActivityCount, error) {
	if len(boundaries) < 2 || len(boundaries) > MaxActivityBuckets+1 {
		return nil, errors.New("invalid activity bucket count")
	}
	for i, boundary := range boundaries {
		if boundary.Hour() != 0 || boundary.Minute() != 0 || boundary.Second() != 0 || boundary.Nanosecond() != 0 || (i > 0 && CalendarDays(boundaries[i-1], boundary) <= 0) {
			return nil, errors.New("invalid activity boundaries")
		}
	}
	counts := make([]ActivityCount, len(boundaries)-1)
	// Each Thought index range is disjoint. Do not join Thoughts to Events: that
	// would multiply counts and repeat coverage work for every Thought.
	args := make([]any, 0, 2*len(counts)+2)
	for i := range counts {
		args = append(args, boundaries[i].Unix(), boundaries[i+1].Unix())
	}
	args = append(args, userID, cutoff.Unix())
	query := `WITH buckets(first,last) AS (VALUES ` + strings.TrimSuffix(strings.Repeat("(?,?),", len(counts)), ",") + `)
 SELECT (SELECT count(*) FROM thoughts t JOIN users u ON u.user_id=t.user_id
 WHERE t.user_id=? AND t.deleted_at IS NULL AND u.deleted_at IS NULL
 AND t.observed_at>=b.first AND t.observed_at<b.last AND t.observed_at<?)
 FROM buckets b ORDER BY b.first`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read activity thoughts: %w", TranslateSQLiteError(err))
	}
	index := 0
	for rows.Next() {
		if err = rows.Scan(&counts[index].Thoughts); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan activity thoughts: %w", TranslateSQLiteError(err))
		}
		index++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read activity thoughts: %w", TranslateSQLiteError(err))
	}

	from, until := boundaries[0], boundaries[len(boundaries)-1]
	rows, err = s.db.QueryContext(ctx, `SELECT e.started_at, min(COALESCE(e.ended_at,?),?)
 FROM events e JOIN users u ON u.user_id=e.user_id
 WHERE e.user_id=? AND e.deleted_at IS NULL AND u.deleted_at IS NULL
 AND e.started_at<? AND e.started_at<?
 AND (COALESCE(e.ended_at,?)>? OR (e.ended_at=e.started_at AND e.started_at>=?))`, cutoff.Unix(), cutoff.Unix(), userID, until.Unix(), cutoff.Unix(), cutoff.Unix(), from.Unix(), from.Unix())
	if err != nil {
		return nil, fmt.Errorf("read activity events: %w", TranslateSQLiteError(err))
	}
	defer rows.Close()
	// Stream compact interval endpoints once. Difference increments cover full
	// interior buckets; only the two boundary buckets need partial-day counts.
	// This is O(events*log(buckets)), not a historical scan for every day/Thought.
	changes := make([]int64, len(counts)+1)
	for rows.Next() {
		var start, end int64
		if err := rows.Scan(&start, &end); err != nil {
			return nil, fmt.Errorf("scan activity events: %w", TranslateSQLiteError(err))
		}
		last := max(start, end-1) // Half-open end; zero-duration Events touch their start day.
		firstTime := time.Unix(max(start, from.Unix()), 0).In(from.Location())
		lastTime := time.Unix(min(last, until.Unix()-1), 0).In(from.Location())
		lo := sort.Search(len(counts), func(i int) bool { return boundaries[i+1].After(firstTime) })
		hi := sort.Search(len(counts), func(i int) bool { return boundaries[i+1].After(lastTime) })
		if lo >= len(counts) || hi < lo {
			continue
		}
		if lo == hi {
			counts[lo].EventDays += int64(CalendarDays(firstTime, lastTime) + 1)
			continue
		}
		counts[lo].EventDays += int64(CalendarDays(firstTime, boundaries[lo+1]))
		counts[hi].EventDays += int64(CalendarDays(boundaries[hi], lastTime) + 1)
		changes[lo+1]++
		changes[hi]--
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read activity events: %w", TranslateSQLiteError(err))
	}
	var active int64
	for i := range counts {
		active += changes[i]
		counts[i].EventDays += active * int64(CalendarDays(boundaries[i], boundaries[i+1]))
	}
	return counts, nil
}
