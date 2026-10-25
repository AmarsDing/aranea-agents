package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"
)

type restoredEmbedSettingRepo struct{ sysSettingRepoStub }

func (r *restoredEmbedSettingRepo) GetKnowledgeEmbed(context.Context) (biz.KnowledgeEmbedSetting, error) {
	return biz.KnowledgeEmbedSetting{Provider: "gemini", Model: "gemini-embedding-001", Dim: 768}, nil
}

func TestKnowledgeEmbedder_RestoresSavedDimensionBeforeFallback(t *testing.T) {
	t.Setenv("KRATOS_KNOWLEDGE_EMBED_DIM", "")
	c := &conf.Data{Postgres: &conf.Data_Postgres{VectorDim: 1536}}
	e := NewKnowledgeEmbedder(c, &restoredEmbedSettingRepo{}, nil, loggateway.NewNoop())
	_, _, model, dim, _, _ := e.Config()
	if dim != 768 || model != "gemini-embedding-001" {
		t.Fatalf("saved model/dimension must restore together: model=%s dim=%d", model, dim)
	}
	t.Setenv("KRATOS_KNOWLEDGE_EMBED_DIM", "1024")
	e = NewKnowledgeEmbedder(c, &restoredEmbedSettingRepo{}, nil, loggateway.NewNoop())
	if e.Dim() != 1024 {
		t.Fatalf("explicit environment dimension must win: %d", e.Dim())
	}
}

func TestKnowledgeEmbedder_DimensionFallback(t *testing.T) {
	t.Setenv("KRATOS_KNOWLEDGE_EMBED_DIM", "")
	c := &conf.Data{Postgres: &conf.Data_Postgres{VectorDim: 1024}}
	if got := NewKnowledgeEmbedder(c, nil, nil, loggateway.NewNoop()).Dim(); got != 1024 {
		t.Fatalf("conf fallback=%d", got)
	}
	if got := NewKnowledgeEmbedder(nil, nil, nil, loggateway.NewNoop()).Dim(); got != 1536 {
		t.Fatalf("default fallback=%d", got)
	}
}
