package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func TestAgentHasKnowledgeSearch(t *testing.T) {
	if agentHasKnowledgeSearch(biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "spirit"}}) {
		t.Fatal("spirit orchestrator must not expose knowledge_search")
	}
	if !agentHasKnowledgeSearch(biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "coding"}}) {
		t.Fatal("coding profile includes knowledge_search")
	}
	if agentHasKnowledgeSearch(biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "read_only"}}) {
		t.Fatal("read_only must not include knowledge_search")
	}
	if !agentHasKnowledgeSearch(biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "spirit", ToolsAllowJSON: `["knowledge_search"]`}}) {
		t.Fatal("explicit allow JSON must opt in")
	}
}

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
	if got := lastUserQuery([]trpcmodel.Message{
		trpcmodel.NewUserMessage("什么是 SLA"),
		asDynamicCue("Available Knowledge Bases"),
	}); got != "什么是 SLA" {
		t.Fatalf("trailing dynamic cue must not become the query, got %q", got)
	}
}

func TestFormatKnowledgeCue_RetrievedPassages(t *testing.T) {
	got, _ := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 3, ChunkCount: 12}},
		[]biz.KnowledgeChunk{{ID: "k1", DocID: "d1", Content: "SLA 承诺 99.9% 可用性。", Score: 0.91}},
		true,
		true,
		false,
		false,
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

func TestFormatKnowledgeCue_CitationNumbersSkipEmpty(t *testing.T) {
	got, cited := formatKnowledgeCue(
		nil,
		[]biz.KnowledgeChunk{
			{ID: "skip", DocID: "d0", Content: "   ", Score: 0.9},
			{ID: "k1", DocID: "d1", Content: "第一段", Score: 0.8},
			{ID: "", DocID: "d2", Content: "无 ID 不进脚注", Score: 0.7},
			{ID: "k2", DocID: "d3", Content: "第二段", Score: 0.6},
		},
		false,
		false,
		false,
		false,
	)
	if !strings.Contains(got, "[1] (doc=d1") || !strings.Contains(got, "[2] (doc=d3") {
		t.Fatalf("cue numbering: %s", got)
	}
	if strings.Contains(got, "[3]") || strings.Contains(got, "doc=d0") {
		t.Fatalf("empty/no-id must not occupy [n]: %s", got)
	}
	if len(cited) != 2 || cited[0].ID != "k1" || cited[1].ID != "k2" {
		t.Fatalf("cited = %+v, want k1 then k2", cited)
	}
}

func TestFormatKnowledgeCue_CatalogOnly(t *testing.T) {
	got, _ := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", Description: "手册", DocumentCount: 1, ChunkCount: 2}},
		nil,
		true,
		true,
		false,
		false,
	)
	if !strings.Contains(got, "knowledge_search") || !strings.Contains(got, "knowledge_reflect") {
		t.Fatalf("empty retrieval + KB tools must name resident tools, got %s", got)
	}
	if !strings.Contains(got, "already on your tool face") {
		t.Fatalf("KB-tool face must say tools are already available, got %s", got)
	}
	if strings.Contains(got, "Available Knowledge Bases") || strings.Contains(got, "产品手册") {
		t.Fatalf("empty retrieval must not dump catalog, got %s", got)
	}
	if strings.Contains(got, "Search strategy tips") || strings.Contains(got, "For specific factual") {
		t.Fatalf("empty retrieval must not inject strategy tips, got %s", got)
	}
	spirit, _ := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", Description: "手册", DocumentCount: 1, ChunkCount: 2}},
		nil,
		true,
		false,
		false,
		false,
	)
	if !strings.Contains(spirit, "not on your tool face") {
		t.Fatalf("Spirit empty retrieval must not-hunt, got %s", spirit)
	}
	if strings.Contains(spirit, "Call `knowledge_search`") {
		t.Fatalf("Spirit must not be told to call knowledge_search, got %s", spirit)
	}
	if strings.Contains(spirit, "web_fetch") == false {
		t.Fatalf("Spirit empty retrieval must forbid web_fetch substitute, got %s", spirit)
	}
	if got, _ := formatKnowledgeCue(nil, nil, true, true, false, false); got != "" {
		t.Fatalf("no collections must stay empty, got %q", got)
	}
}

// P2-1（2026-08-16）：关工具的 agent 仍可获得预检索命中的 chunks，
// 但不渲染目录与工具引导文案；无命中 chunks 时整块不注入。
func TestFormatKnowledgeCue_ToolsDisabled(t *testing.T) {
	got, _ := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 3, ChunkCount: 12}},
		[]biz.KnowledgeChunk{{ID: "k1", DocID: "d1", Content: "SLA 承诺 99.9% 可用性。", Score: 0.91}},
		false,
		false,
		false,
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
	if got, _ := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 1, ChunkCount: 2}},
		nil,
		false,
		false,
		false,
		false,
	); got != "" {
		t.Fatalf("tools-off + no chunks must yield empty cue, got %q", got)
	}
}

func TestFormatKnowledgeCue_GroundedOnly(t *testing.T) {
	got, _ := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 3, ChunkCount: 12}},
		[]biz.KnowledgeChunk{{ID: "k1", DocID: "d1", Content: "SLA 承诺 99.9% 可用性。", Score: 0.91}},
		true,
		true,
		true,
		false,
	)
	if !strings.Contains(got, "Use ONLY these passages") {
		t.Fatalf("grounded hits must forbid world knowledge: %s", got)
	}
	if strings.Contains(got, "Available Knowledge Bases") {
		t.Fatalf("grounded must not list catalog: %s", got)
	}
	emptyTools, _ := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册"}},
		nil,
		true,
		true,
		true,
		false,
	)
	if !strings.Contains(emptyTools, "Do not use world knowledge") {
		t.Fatalf("grounded empty must refuse: %s", emptyTools)
	}
	if !strings.Contains(emptyTools, "knowledge_search") {
		t.Fatalf("grounded+tools empty may search KB: %s", emptyTools)
	}
	emptyNoTools, _ := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "产品手册"}},
		nil,
		false,
		false,
		true,
		false,
	)
	if !strings.Contains(emptyNoTools, "MUST say you do not have evidence") {
		t.Fatalf("grounded tools-off empty must hard-refuse: %s", emptyNoTools)
	}
	if strings.Contains(emptyNoTools, "knowledge_search") {
		t.Fatalf("grounded tools-off must not mention tools: %s", emptyNoTools)
	}
}

func TestFormatKnowledgeCue_MemoryGroundedSuppressesCatalog(t *testing.T) {
	got, _ := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "eval-ops-kb", DocumentCount: 5, ChunkCount: 12}},
		nil,
		true,
		true,
		false,
		true,
	)
	if strings.Contains(got, "Available Knowledge Bases") || strings.Contains(got, "eval-ops-kb") {
		t.Fatalf("memory-grounded cue must not list catalog: %s", got)
	}
	if !strings.Contains(got, "## L2+L3 memory") {
		t.Fatalf("must point at injected memory: %s", got)
	}
	if !strings.Contains(got, "Do not invent") {
		t.Fatalf("must forbid hallucination: %s", got)
	}
}

func TestFormatKnowledgeCue_MemoryGroundedKeepsPassagesWithoutCatalog(t *testing.T) {
	got, _ := formatKnowledgeCue(
		[]biz.KnowledgeCollection{{ID: "c1", Name: "eval-ops-kb", DocumentCount: 5, ChunkCount: 12}},
		[]biz.KnowledgeChunk{{ID: "k1", DocID: "d1", Content: "SLA 承诺 99.9%。", Score: 0.9}},
		true,
		true,
		false,
		true,
	)
	if !strings.Contains(got, "SLA 承诺") {
		t.Fatalf("passages must remain: %s", got)
	}
	if strings.Contains(got, "Available Knowledge Bases") {
		t.Fatalf("catalog must stay hidden: %s", got)
	}
	if !strings.Contains(got, "Prefer injected L2+L3 memory") {
		t.Fatalf("must prefer memory over passages: %s", got)
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
	if !strings.Contains(rec.calls[0].content, `"n":1`) {
		t.Fatalf("pre-retrieval notice must number [1]: %s", rec.calls[0].content)
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
	chunks = append(chunks, biz.KnowledgeChunk{ID: "", Content: "no-id"})
	got := cueRenderedChunks(chunks)
	if len(got) != knowledgeCueTopK {
		t.Fatalf("cap = %d, want %d", len(got), knowledgeCueTopK)
	}
	for _, ch := range got {
		if ch.ID == "empty" || ch.ID == "" {
			t.Fatal("empty content/id must be filtered")
		}
	}
	if got := cueRenderedChunks(nil); len(got) != 0 {
		t.Fatalf("nil in = %v", got)
	}
}

// countingCueRepo 统计 ListCollections 调用次数，证明 per-turn 缓存命中时
// 不再回表（2026-08-21 全链路审查 B2 补测）。
type countingCueRepo struct {
	fakeKnowledgeRepos
	listCalls int
}

func (c *countingCueRepo) ListCollections(ctx context.Context, ws string, limit, offset int) ([]biz.KnowledgeCollection, int, error) {
	c.listCalls++
	return c.fakeKnowledgeRepos.ListCollections(ctx, ws, limit, offset)
}

// TestResolveKnowledgeCue_TurnCache 覆盖 per-turn 缓存四分支：首轮 fresh
// 构建并写缓存；工具循环续轮（query==""）命中缓存不重查 DB；查询变化重建；
// 无 invocation 上下文退化为每次 fresh（与 TestBuildRuntimeMemoryCue_TurnCacheReuse
// 同构）。
func TestResolveKnowledgeCue_TurnCache(t *testing.T) {
	repo := &countingCueRepo{fakeKnowledgeRepos: fakeKnowledgeRepos{
		collections: []biz.KnowledgeCollection{{ID: "c1", Name: "产品手册", DocumentCount: 1, ChunkCount: 1}},
	}}
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	lg := loggateway.NewNoop()
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "s1", UserID: "u1"}}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	msgs := []trpcmodel.Message{trpcmodel.NewUserMessage("什么是 SLA")}
	cue1, _, fresh := resolveKnowledgeCue(ctx, uc, lg, msgs, true, true, false)
	if !fresh {
		t.Fatal("first call in a turn must build fresh")
	}
	if repo.listCalls != 1 {
		t.Fatalf("ListCollections calls = %d, want 1", repo.listCalls)
	}

	// 工具循环续轮：尾部 assistant 消息 → lastUserQuery=="" → 命中缓存。
	loopMsgs := append(msgs, trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "calling tool"})
	cue2, _, fresh := resolveKnowledgeCue(ctx, uc, lg, loopMsgs, true, true, false)
	if fresh {
		t.Fatal("tool-loop continuation must reuse the turn cache")
	}
	if cue2 != cue1 {
		t.Fatalf("cached turn must replay the same cue: %q vs %q", cue2, cue1)
	}
	if repo.listCalls != 1 {
		t.Fatalf("cache hit must not re-list collections, got %d", repo.listCalls)
	}

	// 查询变化（新 user 消息）→ 缓存失效，重新构建。
	changed := append(loopMsgs, trpcmodel.NewUserMessage("换个话题"))
	if _, _, fresh = resolveKnowledgeCue(ctx, uc, lg, changed, true, true, false); !fresh {
		t.Fatal("query change must trigger a fresh build")
	}
	if repo.listCalls != 2 {
		t.Fatalf("rebuild after query change: ListCollections = %d, want 2", repo.listCalls)
	}

	// 无 invocation（单测/异常路径）→ 每次 fresh，保持旧行为。
	if _, _, fresh = resolveKnowledgeCue(context.Background(), uc, lg, msgs, true, true, false); !fresh {
		t.Fatal("no invocation context must stay fresh (legacy behavior)")
	}
}

func TestResolveKnowledgeCue_UsesPrefetch(t *testing.T) {
	t.Parallel()
	prefetch := &TurnCuePrefetch{knowledge: &prefetchedKnowledgeCue{
		query: cleanRecallQuery("什么是 SLA"),
		cue:   "## Retrieved Knowledge\nprefetched",
	}}
	ctx := WithTurnCuePrefetch(context.Background(), prefetch)
	cue, _, fresh := resolveKnowledgeCue(ctx, nil, loggateway.NewNoop(), []trpcmodel.Message{trpcmodel.NewUserMessage("什么是 SLA")}, true, true, false)
	if !fresh {
		t.Fatal("prefetch consume is the first real inject")
	}
	if cue != prefetch.knowledge.cue {
		t.Fatalf("cue=%q", cue)
	}
}
