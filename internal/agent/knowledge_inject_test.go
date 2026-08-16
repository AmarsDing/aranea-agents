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
		true,
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
		true,
	)
	if !strings.Contains(got, "Available Knowledge Bases") {
		t.Fatalf("catalog cue missing: %s", got)
	}
	if strings.Contains(got, "Retrieved Knowledge") {
		t.Fatal("empty chunks must not emit retrieved section")
	}
}

// P2-1（2026-08-16）：关工具的 agent 仍可获得预检索命中的 chunks，
// 但不渲染目录与工具引导文案；无命中 chunks 时整块不注入。
func TestFormatKnowledgeCue_ToolsDisabled(t *testing.T) {
	got := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 3, ChunkCount: 12}},
		[]biz.KnowledgeChunk{{ID: "k1", DocID: "d1", Content: "SLA 承诺 99.9% 可用性。", Score: 0.91}},
		false,
	)
	if !strings.Contains(got, "## Retrieved Knowledge") || !strings.Contains(got, "SLA 承诺") {
		t.Fatalf("tools-off must still render retrieved chunks: %s", got)
	}
	if strings.Contains(got, "Available Knowledge Bases") {
		t.Fatalf("tools-off must not render catalog: %s", got)
	}
	if strings.Contains(got, "knowledge_search") {
		t.Fatalf("tools-off must not mention search tools: %s", got)
	}
	// 无命中 chunks：tools-off 下目录单独存在只会误导，整块不注入。
	if got := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 1, ChunkCount: 2}},
		nil,
		false,
	); got != "" {
		t.Fatalf("tools-off + no chunks must yield empty cue, got %q", got)
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

// P1（2026-08-15 评审修订）：首轮预检索注入也发 knowledge_recalled，
// 日常供粮主路径进入 cited 回采闭环。
func TestKnowledgeCueHook_EmitsKnowledgeRecalledNotice(t *testing.T) {
	ag := biz.Agent{
		ID:       "ag-1",
		Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true},
	}
	repo := cueSearchRepo{
		fakeKnowledgeRepos: fakeKnowledgeRepos{
			collections: []biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 1, ChunkCount: 1}},
		},
		chunks: []biz.KnowledgeChunk{
			{ID: "k1", DocID: "d1", CollectionID: "c1", Content: "SLA 承诺 99.9%。", Score: 0.88},
			{ID: "k2", DocID: "d2", CollectionID: "c1", Content: "   ", Score: 0.5}, // 空正文不渲染也不进 notice
		},
	}
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	deps := TRPCBuilderDeps{TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{KnowledgeUsecase: uc}}
	hook := newKnowledgeCueBeforeHook(ag, deps)

	ret := knowledge.NewRetriever(cueEmbedder{}, repo, nil, loggateway.NewNoop())
	rec := &noticeRecorder{}
	ctx := biz.WithActivityEmitter(context.Background(), rec)
	ctx = knowledgetool.WithRetriever(ctx, ret)
	ctx = knowledgetool.WithKnowledgeCollections(ctx, []string{"c1"})

	runBeforeModelHook(t, hook, ctx)
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 knowledge_recalled notice, got %d", len(rec.calls))
	}
	if rec.calls[0].noticeType != "knowledge_recalled" {
		t.Fatalf("noticeType = %q", rec.calls[0].noticeType)
	}
	if !strings.Contains(rec.calls[0].content, `"chunk_id":"k1"`) {
		t.Fatalf("payload missing k1: %s", rec.calls[0].content)
	}
	if strings.Contains(rec.calls[0].content, `"chunk_id":"k2"`) {
		t.Fatalf("empty-content chunk must not be noticed: %s", rec.calls[0].content)
	}
}

// cueRenderedChunks：只保留实际渲染进 cue 的 chunks。
func TestCueRenderedChunks_FiltersEmptyAndCapsTopK(t *testing.T) {
	chunks := make([]biz.KnowledgeChunk, 0, 8)
	for i := 0; i < 6; i++ {
		chunks = append(chunks, biz.KnowledgeChunk{ID: string(rune('a' + i)), Content: "正文"})
	}
	chunks = append(chunks, biz.KnowledgeChunk{ID: "empty", Content: "  "})
	got := cueRenderedChunks(chunks)
	if len(got) != knowledgeCueTopK {
		t.Fatalf("cap = %d, want %d", len(got), knowledgeCueTopK)
	}
	for _, ch := range got {
		if ch.ID == "empty" {
			t.Fatal("empty content must be filtered")
		}
	}
	if got := cueRenderedChunks(nil); len(got) != 0 {
		t.Fatalf("nil in = %v", got)
	}
}
