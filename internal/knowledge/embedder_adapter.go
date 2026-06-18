package knowledge

// DEPRECATED: 框架架构差异，永久阻塞（alignment-plan.md §十一/B-2）。
// 框架中没有任何组件直接消费 embedder.Embedder 接口——框架的 Knowledge
// 模块通过 knowledge.Knowledge 接口抽象了整个检索流程，Embedder 是项目
// 内部实现细节。wire.go 直接绑定 MultiProviderEmbedder 是正确的。下一迭代
// 将删除此死代码（CS-B2）。

import (
	"context"

	"aranea-agents/pkg/loggateway"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/embedder"
)

// EmbedderAdapter adapts MultiProviderEmbedder to the framework's embedder.Embedder interface.
//
// DEPRECATED: 框架架构差异，永久阻塞。见文件头说明。
type EmbedderAdapter struct {
	inner *MultiProviderEmbedder
	lg    loggateway.Logger
}

var _ embedder.Embedder = (*EmbedderAdapter)(nil)

// NewEmbedderAdapter creates a new EmbedderAdapter wrapping the given MultiProviderEmbedder.
func NewEmbedderAdapter(inner *MultiProviderEmbedder, lg loggateway.Logger) *EmbedderAdapter {
	return &EmbedderAdapter{
		inner: inner,
		lg:    lg.With(loggateway.Domain("knowledge.embedder_adapter")),
	}
}

// GetEmbedding generates an embedding vector for the given text, converting float32 to float64.
func (a *EmbedderAdapter) GetEmbedding(ctx context.Context, text string) ([]float64, error) {
	vec, err := a.inner.EmbedSingle(ctx, text)
	if err != nil {
		return nil, err
	}
	return float32sToFloat64s(vec), nil
}

// GetEmbeddingWithUsage generates an embedding vector with usage information.
// The usage map is empty because MultiProviderEmbedder does not track token usage.
func (a *EmbedderAdapter) GetEmbeddingWithUsage(ctx context.Context, text string) ([]float64, map[string]any, error) {
	vec, err := a.inner.EmbedSingle(ctx, text)
	if err != nil {
		return nil, nil, err
	}
	return float32sToFloat64s(vec), map[string]any{}, nil
}

// GetDimensions returns the embedding dimensionality.
func (a *EmbedderAdapter) GetDimensions() int {
	return a.inner.Dim()
}

func float32sToFloat64s(in []float32) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}
