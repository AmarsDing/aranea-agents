package knowledge

import (
	"context"
	"errors"
	"os"
	"testing"

	trpcreranker "trpc.group/trpc-go/trpc-agent-go/knowledge/reranker"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type stubTRPCReranker struct {
	score float64
	err   error
	empty bool
}

func (s stubTRPCReranker) Rerank(context.Context, *trpcreranker.Query, []*trpcreranker.Result) ([]*trpcreranker.Result, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.empty {
		return nil, nil
	}
	return []*trpcreranker.Result{{Score: s.score}}, nil
}

func TestKnowledgeRerankerAdapter_ScoreUsesBackend(t *testing.T) {
	a := NewKnowledgeRerankerAdapter(stubTRPCReranker{score: 0.91}, loggateway.NewNoop())
	if got := a.Score("alpha beta", "alpha beta gamma"); got != 0.91 {
		t.Fatalf("Score = %v, want 0.91", got)
	}
}

func TestKnowledgeRerankerAdapter_FallbackOnError(t *testing.T) {
	a := NewKnowledgeRerankerAdapter(stubTRPCReranker{err: errors.New("boom")}, loggateway.NewNoop())
	got := a.Score("alpha beta", "alpha beta gamma")
	want := fallbackJaccard("alpha beta", "alpha beta gamma")
	if got != want {
		t.Fatalf("Score = %v, want fallback %v", got, want)
	}
}

func TestKnowledgeRerankerAdapter_FallbackOnEmpty(t *testing.T) {
	a := NewKnowledgeRerankerAdapter(stubTRPCReranker{empty: true}, loggateway.NewNoop())
	got := a.Score("alpha beta", "alpha beta gamma")
	want := fallbackJaccard("alpha beta", "alpha beta gamma")
	if got != want {
		t.Fatalf("Score = %v, want fallback %v", got, want)
	}
}

func TestNewMemoryReranker_DefaultLexical(t *testing.T) {
	t.Setenv("KRATOS_MEMORY_RERANKER", "")
	r := NewMemoryReranker(loggateway.NewNoop())
	if _, ok := r.(*biz.CrossEncoderReranker); !ok {
		t.Fatalf("default reranker type %T, want *biz.CrossEncoderReranker", r)
	}
}

func TestNewMemoryReranker_UnknownBackendFallsBack(t *testing.T) {
	t.Setenv("KRATOS_MEMORY_RERANKER", "not-a-backend")
	r := NewMemoryReranker(loggateway.NewNoop())
	if _, ok := r.(*biz.CrossEncoderReranker); !ok {
		t.Fatalf("unknown backend type %T, want *biz.CrossEncoderReranker", r)
	}
}

func TestNewMemoryReranker_CohereNilKnowledgeEnvFallsBack(t *testing.T) {
	t.Setenv("KRATOS_MEMORY_RERANKER", "cohere")
	t.Setenv("KRATOS_KNOWLEDGE_RERANKER", "")
	r := NewMemoryReranker(loggateway.NewNoop())
	if _, ok := r.(*biz.CrossEncoderReranker); !ok {
		t.Fatalf("cohere without knowledge env type %T, want *biz.CrossEncoderReranker", r)
	}
}

func TestNewMemoryReranker_IgnoresUnrelatedEnv(t *testing.T) {
	// Ensure a leftover process env cannot leak into default path.
	_ = os.Unsetenv("KRATOS_MEMORY_RERANKER")
	r := NewMemoryReranker(loggateway.NewNoop())
	if r == nil {
		t.Fatal("NewMemoryReranker returned nil")
	}
}
