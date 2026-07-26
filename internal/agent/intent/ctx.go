package intent

import "context"

// artifactCtxKey is the context key carrying a pre-resolved intent Artifact.
// The clarification resume path stores the artifact produced before the
// clarification gate fired so the resumed turn can reuse it instead of paying
// for a duplicate intent-pass LLM call on the rewritten content.
type artifactCtxKey struct{}

// WithArtifact returns a context carrying a pre-resolved Artifact.
// Nil artifact is a no-op (returns ctx unchanged).
func WithArtifact(ctx context.Context, art *Artifact) context.Context {
	if art == nil {
		return ctx
	}
	return context.WithValue(ctx, artifactCtxKey{}, art)
}

// ArtifactFromContext returns the pre-resolved Artifact, or nil when absent.
func ArtifactFromContext(ctx context.Context) *Artifact {
	v, _ := ctx.Value(artifactCtxKey{}).(*Artifact)
	return v
}
