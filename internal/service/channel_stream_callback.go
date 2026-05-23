package service

import (
	"context"
	"errors"
	"strings"

	chatagent "aranea-agents/internal/agent"
)

type channelStreamCallbackKey struct{}

// streamPreviewUpdater patches in-place IM preview messages during a channel turn.
type streamPreviewUpdater interface {
	Update(ctx context.Context, recipient, text string, force bool) error
}

// ChannelStreamCallback is invoked with accumulated assistant text during a native turn.
//
// Deprecated: IM channels use TurnPreviewCoordinator + EventBus projection instead of
// inline OnReplyDelta callbacks. Kept for RunNativeTurnStreaming API compatibility.
type ChannelStreamCallback func(accumulated string) error

// WithChannelStreamCallback attaches a streaming reply callback to ctx for RunNativeTurnUnary/Streaming.
//
// Deprecated: prefer TurnPreviewCoordinator for channel IM preview updates.
func WithChannelStreamCallback(ctx context.Context, fn ChannelStreamCallback) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, channelStreamCallbackKey{}, fn)
}

func channelStreamCallbackFromContext(ctx context.Context) ChannelStreamCallback {
	if ctx == nil {
		return nil
	}
	fn, _ := ctx.Value(channelStreamCallbackKey{}).(ChannelStreamCallback)
	return fn
}

// streamPreviewTurnError reports channel stream preview failures during a native turn.
func streamPreviewTurnError(ctx context.Context, result chatagent.EventStreamResult) error {
	if !result.HasError || channelStreamCallbackFromContext(ctx) == nil {
		return nil
	}
	detail := strings.TrimSpace(result.LastError)
	if detail == "" {
		detail = "stream preview update failed"
	}
	return errors.New(detail)
}
