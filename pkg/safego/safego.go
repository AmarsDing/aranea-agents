package safego

import (
	"context"
	"log/slog"
	"runtime/debug"
)

func Go(ctx context.Context, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic recovered",
					"where", name,
					"err", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}
