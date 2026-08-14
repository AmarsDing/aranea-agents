package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestLastUserQuery_SkipsToolLoop(t *testing.T) {
	if got := lastUserQuery([]trpcmodel.Message{
		trpcmodel.NewSystemMessage("sys"),
		trpcmodel.NewUserMessage("什么是 SLA"),
	}); got != "什么是 SLA" {
		t.Fatalf("user turn query = %q", got)
	}
	if got := lastUserQuery([]trpcmodel.Message{
		trpcmodel.NewUserMessage("什么是 SLA"),
		{Role: trpcmodel.RoleAssistant, Content: "calling tool"},
	}); got != "" {
		t.Fatalf("tool-loop continuation must skip, got %q", got)
	}
}

func TestFormatKnowledgeCue_RetrievedPassages(t *testing.T) {
	got := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 3, ChunkCount: 12}},
		[]biz.KnowledgeChunk{{ID: "k1", DocID: "d1", Content: "SLA 承诺 99.9% 可用性。", Score: 0.91}},
	)
	if !strings.Contains(got, "## Retrieved Knowledge") {
		t.Fatalf("missing retrieved section: %s", got)
	}
	if !strings.Contains(got, "SLA 承诺") {
		t.Fatalf("missing passage: %s", got)
	}
	if !strings.Contains(got, "## Available Knowledge Bases") {
		t.Fatalf("catalog must remain: %s", got)
	}
	if strings.Contains(got, "Search strategy tips") {
		t.Fatal("strategy tips are for catalog-only cue")
	}
}

func TestFormatKnowledgeCue_CatalogOnly(t *testing.T) {
	got := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", Description: "手册", DocumentCount: 1, ChunkCount: 2}},
		nil,
	)
	if !strings.Contains(got, "Available Knowledge Bases") {
		t.Fatalf("catalog cue missing: %s", got)
	}
	if strings.Contains(got, "Retrieved Knowledge") {
		t.Fatal("empty chunks must not emit retrieved section")
	}
}

type cueEmbedder struct{}

func (cueEmbedder) EmbedSingle(context.Context, string) ([]float32, error) {
	return []float32{1, 0, 0}, nil
}

type cueSearchRepo struct {
	fakeKnowledgeRepos
	chunks []biz.KnowledgeChunk
}

func (c cueSearchRepo) GetCollection(context.Context, string) (biz.KnowledgeCollection, error) {
	return biz.KnowledgeCollection{ID: "c1", EmbeddingModel: "m", Dim: 3}, nil
}

func (c cueSearchRepo) SearchChunks(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
	return c.chunks, nil
}

func TestKnowledgeCueHook_RetrievesPassages(t *testing.T) {
	ag := biz.Agent{
		ID:       "ag-1",
		Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true},
	}
	repo := cueSearchRepo{
		fakeKnowledgeRepos: fakeKnowledgeRepos{
			collections: []biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 1, ChunkCount: 1}},
		},
		chunks: []biz.KnowledgeChunk{{ID: "k1", DocID: "d1", CollectionID: "c1", Content: "SLA 承诺 99.9%。", Score: 0.88}},
	}
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	deps := TRPCBuilderDeps{TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{KnowledgeUsecase: uc}}
	hook := newKnowledgeCueBeforeHook(ag, deps)

	ret := knowledge.NewRetriever(cueEmbedder{}, repo, nil, loggateway.NewNoop())
	ctx := knowledgetool.WithRetriever(context.Background(), ret)
	ctx = knowledgetool.WithKnowledgeCollections(ctx, []string{"c1"})

	msgs := runBeforeModelHook(t, hook, ctx)
	cue := msgs[len(msgs)-1].Content
	if !strings.Contains(cue, "Retrieved Knowledge") {
		t.Fatalf("expected retrieved knowledge in cue, got %s", cue)
	}
	if !strings.Contains(cue, "SLA 承诺") {
		t.Fatalf("expected passage body, got %s", cue)
	}
}
