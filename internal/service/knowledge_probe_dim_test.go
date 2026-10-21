package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

var errProbeDown = errors.New("probe down")

// ── G-39 对抗评审修正（F4-B1/blocker）：pending-reembed 判定的 dim 口径必须
// 取 embedder 实测输出维度（ProbeDim），配置 dim 与模型原生维度分叉时按配置
// 判定会让缺维腿永不命中、全集合无限重嵌（烧配额且搜索侧不可见）。

type probeDimStubEmbedder struct {
	cfgDim   int
	probeDim int
	probeErr error
	probed   int
}

func (s *probeDimStubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, s.cfgDim)
	}
	return out, nil
}
func (s *probeDimStubEmbedder) Dim() int { return s.cfgDim }
func (s *probeDimStubEmbedder) ProbeDim(context.Context) (int, error) {
	s.probed++
	return s.probeDim, s.probeErr
}

// 不带 ProbeDim 的裸桩（只实现 knowledge.Embedder 窄接口）。
type plainDimStubEmbedder struct{ dim int }

func (s *plainDimStubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, s.dim)
	}
	return out, nil
}
func (s *plainDimStubEmbedder) Dim() int { return s.dim }

func TestCurrentEmbedderDim_ProbeAuthoritative(t *testing.T) {
	ctx := context.Background()

	t.Run("probe dim wins over configured dim", func(t *testing.T) {
		emb := &probeDimStubEmbedder{cfgDim: 1024, probeDim: 768}
		svc := &KnowledgeService{embedder: emb, lg: loggateway.NewNoop()}
		if got := svc.currentEmbedderDim(ctx); got != 768 {
			t.Fatalf("dim = %d, want 768 (probe-authoritative, config 1024 is a lie)", got)
		}
	})

	t.Run("probe error falls back to configured dim", func(t *testing.T) {
		emb := &probeDimStubEmbedder{cfgDim: 1024, probeErr: errProbeDown}
		svc := &KnowledgeService{embedder: emb, lg: loggateway.NewNoop()}
		if got := svc.currentEmbedderDim(ctx); got != 1024 {
			t.Fatalf("dim = %d, want 1024 (fallback to configured dim on probe error)", got)
		}
	})

	t.Run("probe zero falls back to configured dim", func(t *testing.T) {
		emb := &probeDimStubEmbedder{cfgDim: 1536, probeDim: 0}
		svc := &KnowledgeService{embedder: emb, lg: loggateway.NewNoop()}
		if got := svc.currentEmbedderDim(ctx); got != 1536 {
			t.Fatalf("dim = %d, want 1536 (probe<=0 must not reach predicate)", got)
		}
	})

	t.Run("non-prober embedder uses configured dim", func(t *testing.T) {
		svc := &KnowledgeService{embedder: &plainDimStubEmbedder{dim: 384}, lg: loggateway.NewNoop()}
		if got := svc.currentEmbedderDim(ctx); got != 384 {
			t.Fatalf("dim = %d, want 384", got)
		}
	})

	t.Run("nil embedder yields zero (predicate skips missing-dim leg)", func(t *testing.T) {
		svc := &KnowledgeService{lg: loggateway.NewNoop()}
		if got := svc.currentEmbedderDim(ctx); got != 0 {
			t.Fatalf("dim = %d, want 0", got)
		}
	})
}
