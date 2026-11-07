// Package history runs the services' history: a reading of every service each minute, and
// the purge of what is older than the retention.
package history

import (
	"context"
	"log/slog"
	"time"
)

// Store is where the readings go; pgstore implements it.
type Store interface {
	SampleHistory(ctx context.Context, now time.Time) error
	PurgeHistory(ctx context.Context, now time.Time) error
}

// PurgeEvery is how often the old history is erased.
const PurgeEvery = time.Hour

// Watch reads every service now, then at each interval, and purges at start then hourly,
// until ctx ends.
func Watch(ctx context.Context, store Store, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	var purged time.Time
	for {
		now := time.Now()
		if err := store.SampleHistory(ctx, now); err != nil && ctx.Err() == nil {
			slog.Error("history", "err", err)
		}
		if now.Sub(purged) >= PurgeEvery {
			if err := store.PurgeHistory(ctx, now); err != nil && ctx.Err() == nil {
				slog.Error("history purge", "err", err)
			} else {
				purged = now
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
