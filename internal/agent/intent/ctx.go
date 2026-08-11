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

// speculativeCtxKey carries a speculatively pre-computed Artifact (voice C2:
// ASR partial 稳定后提前跑意图识别，final 文本一致时注入复用）。
//
// 与 artifactCtxKey（澄清续跑）的关键语义差异：投机产物是 fresh 的——
// 尚未经过澄清门评估，消费者必须保留其澄清残留（clarifications/ambiguities/
// needs_clarification），让澄清门照常判定；澄清续跑产物已过门，需剥离残留。
// 两个 key 隔离，互不串扰。
type speculativeCtxKey struct{}

// WithSpeculativeArtifact returns a context carrying a speculative Artifact.
// Nil artifact is a no-op (returns ctx unchanged).
func WithSpeculativeArtifact(ctx context.Context, art *Artifact) context.Context {
	if art == nil {
		return ctx
	}
	return context.WithValue(ctx, speculativeCtxKey{}, art)
}

// SpeculativeArtifactFromContext returns the speculative Artifact, or nil when absent.
func SpeculativeArtifactFromContext(ctx context.Context) *Artifact {
	v, _ := ctx.Value(speculativeCtxKey{}).(*Artifact)
	return v
}
