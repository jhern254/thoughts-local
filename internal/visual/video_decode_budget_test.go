package visual

import (
	"context"
	"errors"
	"testing"
)

func TestVideoDecodeBudget_Write(t *testing.T) {
	t.Run("counts chunked frames including negative timestamp preroll", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		budget := videoDecodeBudget{cancel: cancel, maxFrames: 2}
		for _, chunk := range []string{"fra", "me:0 pts:-4 pts_time:-2\nthoughts.frame=1\nframe:1 pts:0", " pts_time:0\nthoughts.frame=1\n"} {
			if _, err := budget.Write([]byte(chunk)); err != nil {
				t.Fatal(err)
			}
		}
		if budget.frames != 2 || budget.wantValue || len(budget.line) != 0 || ctx.Err() != nil {
			t.Fatalf("frames=%d, incomplete=%t, context=%v", budget.frames, budget.wantValue, ctx.Err())
		}
	})
	t.Run("cancels immediately when decoded work exceeds the budget", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		budget := videoDecodeBudget{cancel: cancel, maxFrames: 1}
		_, err := budget.Write([]byte("frame:0 pts:0 pts_time:0\nthoughts.frame=1\nframe:1 pts:0 pts_time:0\n"))
		if !errors.Is(err, ErrVideo) || !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("error=%v, context=%v, want rejected work and cancellation", err, ctx.Err())
		}
	})
	t.Run("rejects malformed counter output without retaining unbounded text", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		budget := videoDecodeBudget{cancel: cancel, maxFrames: 1}
		_, err := budget.Write(make([]byte, 257))
		if !errors.Is(err, ErrVideo) || !errors.Is(ctx.Err(), context.Canceled) || len(budget.line) > 256 {
			t.Fatalf("error=%v, context=%v, retained bytes=%d", err, ctx.Err(), len(budget.line))
		}
	})
}
