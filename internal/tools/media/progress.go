package media

import (
	"context"
	"fmt"
	"time"
)

// ProgressReporter is a function that publishes progress updates during
// long-running media generation tasks. The implementation sends
// ActivityEvent updated events with meta.progress.
type ProgressReporter func(ctx context.Context, value, max int, label string) error

// PublishProgress publishes a progress update for the current tool execution.
// The ActivityEvent system routes this to the frontend via WebSocket.
//
// Usage in tools:
//
//	reporter := NewProgressReporter(ctx)
//	reporter(ctx, 30, 100, "采样中 30%")
func PublishProgress(ctx context.Context, value, max int, label string) error {
	// TODO: Wire to ActivityEvent bus when available in tool context.
	// For now, log the progress as a placeholder. The actual implementation
	// will inject an event bus via context (similar to how session ID is injected).
	return nil
}

// PollWithProgress polls an async media generation job, publishing progress
// updates at regular intervals. Returns when the job completes or ctx is cancelled.
func PollWithProgress(
	ctx context.Context,
	interval time.Duration,
	getStatus func(ctx context.Context) (progress int, done bool, err error),
	reporter ProgressReporter,
) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			progress, done, err := getStatus(ctx)
			if err != nil {
				continue // transient error, keep polling
			}
			if reporter != nil {
				_ = reporter(ctx, progress, 100, fmt.Sprintf("生成中 %d%%", progress))
			}
			if done {
				return nil
			}
		}
	}
}
