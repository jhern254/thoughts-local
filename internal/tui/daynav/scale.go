package daynav

import (
	"context"
	"errors"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/render"
)

func alignedBucketStart(day time.Time, bucketDays int) time.Time {
	index := calendarDayIndex(day)
	// A positive remainder also anchors dates before the epoch correctly.
	remainder := (index%bucketDays + bucketDays) % bucketDays
	return day.AddDate(0, 0, -remainder)
}

// A new history snapshot/resolution pays for the full-range scale once. Each
// query and projection is bounded; ordinary panning reads only its viewport.
// Reusing ActivityCounts preserves the same ownership, DST and Event-day rules.
func readHistoryReference(ctx context.Context, reader Reader, user string, first, until time.Time, bucketDays int, cutoff time.Time) (float64, error) {
	var reference float64
	for from := alignedBucketStart(first, bucketDays); from.Before(until); {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		count := min(data.MaxActivityBuckets, (data.CalendarDays(from, until)+bucketDays-1)/bucketDays)
		boundaries := make([]time.Time, count+1)
		for i := range boundaries {
			boundaries[i] = from.AddDate(0, 0, i*bucketDays)
		}
		counts, err := reader.ActivityCounts(ctx, user, boundaries, cutoff)
		if err != nil {
			return 0, err
		}
		if len(counts) != count {
			return 0, errors.New("invalid activity scale result")
		}
		values := make([]render.DayActivity, count)
		for i, value := range counts {
			values[i] = render.DayActivity{Events: float64(value.EventDays) / float64(bucketDays), Thoughts: float64(value.Thoughts) / float64(bucketDays)}
		}
		reference = max(reference, render.DayActivityReference(values))
		from = boundaries[count]
	}
	return reference, ctx.Err()
}
