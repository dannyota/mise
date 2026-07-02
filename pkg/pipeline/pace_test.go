package pipeline_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/mise/pkg/pipeline"
)

func TestPaceSleepsFullDuration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		if err := pipeline.Pace(t.Context(), 3*time.Second); err != nil {
			t.Fatalf("Pace() error = %v, want nil", err)
		}
		if got := time.Since(start); got != 3*time.Second {
			t.Errorf("Pace() slept %v, want exactly 3s (virtual time)", got)
		}
	})
}

func TestPaceReturnsEarlyOnCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			time.Sleep(time.Second)
			cancel()
		}()
		start := time.Now()
		err := pipeline.Pace(ctx, time.Hour)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Pace() error = %v, want context.Canceled", err)
		}
		if got := time.Since(start); got != time.Second {
			t.Errorf("Pace() returned after %v, want exactly 1s (virtual time)", got)
		}
	})
}

func TestPaceZeroDurationReturnsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		if err := pipeline.Pace(t.Context(), 0); err != nil {
			t.Fatalf("Pace(0) error = %v, want nil", err)
		}
		if got := time.Since(start); got != 0 {
			t.Errorf("Pace(0) took %v, want no virtual time", got)
		}
	})
}

func TestPaceCanceledContextWinsOverZeroDuration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := pipeline.Pace(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("Pace(canceled, 0) error = %v, want context.Canceled", err)
	}
}
