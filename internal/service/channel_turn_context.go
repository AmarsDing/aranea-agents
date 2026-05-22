package service

import (
	"context"
	"time"
)

type channelTurnDeadlineKey struct{}

// ChannelTurnDeadlines optional per-turn limits injected by Channel ingress.
type ChannelTurnDeadlines struct {
	TurnTimeout      time.Duration
	FirstByteTimeout time.Duration
}

// WithChannelTurnDeadlines attaches Channel-level turn timeouts to ctx.
func WithChannelTurnDeadlines(ctx context.Context, d ChannelTurnDeadlines) context.Context {
	if d.TurnTimeout <= 0 && d.FirstByteTimeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, channelTurnDeadlineKey{}, d)
}

func channelTurnDeadlinesFromContext(ctx context.Context) (ChannelTurnDeadlines, bool) {
	if ctx == nil {
		return ChannelTurnDeadlines{}, false
	}
	d, ok := ctx.Value(channelTurnDeadlineKey{}).(ChannelTurnDeadlines)
	return d, ok
}

func applyChannelTurnTimeout(parent context.Context, turnSec int) (context.Context, context.CancelFunc) {
	if turnSec <= 0 {
		return parent, func() {}
	}
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) <= time.Duration(turnSec)*time.Second {
		return parent, func() {}
	}
	return context.WithTimeout(parent, time.Duration(turnSec)*time.Second)
}

func firstByteTimeoutFromContext(ctx context.Context) (time.Duration, bool) {
	d, ok := channelTurnDeadlinesFromContext(ctx)
	if !ok || d.FirstByteTimeout <= 0 {
		return 0, false
	}
	return d.FirstByteTimeout, true
}
