package pipeline

import (
	"context"
	"time"
)

// Pace sleeps for d as a politeness delay between source requests, returning
// early with ctx's error when the context is canceled first. A non-positive d
// returns immediately (nil unless ctx is already done). Unlike time.Sleep it
// never outlives its caller's cancellation, so a shutting-down activity stops
// pacing instead of blocking the worker.
func Pace(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
