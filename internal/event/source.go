package event

import (
	"context"
	"strings"
)

type envelopeSourceKey struct{}

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
