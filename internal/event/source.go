package event

import (
	"context"
	"strings"
)

// WithEnvelopePlatform tags ctx with IM platform (feishu|lark|dingtalk|wecom) for UserBubble badges.
func WithEnvelopePlatform(ctx context.Context, platform string) context.Context {
	platform = strings.TrimSpace(platform)
	if platform == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, envelopePlatformKey{}, platform)
}

// EnvelopePlatformFromContext reads the platform tag from ctx.
func EnvelopePlatformFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(envelopePlatformKey{}).(string)
	return strings.TrimSpace(v)
}

// WithEnvelopeChannelKey tags ctx with channel config key for UserBubble badges.
func WithEnvelopeChannelKey(ctx context.Context, channelKey string) context.Context {
	channelKey = strings.TrimSpace(channelKey)
	if channelKey == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, envelopeChannelKeyKey{}, channelKey)
}

// EnvelopeChannelKeyFromContext reads channel_key from ctx.
func EnvelopeChannelKeyFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(envelopeChannelKeyKey{}).(string)
	return strings.TrimSpace(v)
}

// WithChannelEnvelopeContext stamps source=channel, platform, and channel_key for channel ingress turns.
func WithChannelEnvelopeContext(ctx context.Context, platform, channelKey string) context.Context {
	return WithEnvelopeChannelKey(WithEnvelopePlatform(WithEnvelopeSource(ctx, "channel"), platform), channelKey)
}

type envelopePlatformKey struct{}
type envelopeSourceKey struct{}
type envelopeChannelKeyKey struct{}

// WithEnvelopeSource tags ctx so EventProjector can stamp envelope.source (web|channel|cron|a2a).
func WithEnvelopeSource(ctx context.Context, source string) context.Context {
	source = strings.TrimSpace(source)
	if source == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, envelopeSourceKey{}, source)
}

// EnvelopeSourceFromContext reads the source tag from ctx.
func EnvelopeSourceFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(envelopeSourceKey{}).(string)
	return strings.TrimSpace(v)
}
