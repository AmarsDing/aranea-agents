package service

import (
	"context"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/knowledge/blockparse"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── SP1-G：PromoteBlocks service 接线 ────────────────────────────────────────

// promoteBlockIndexStub mock BlockIndexRepo：ReplaceDocBlocks 记录块并赋 ID，
// ListDocBlocks 返回最近一次重放结果（谱系匹配数据源）。
type promoteBlockIndexStub struct {
	blocksByDoc map[string][]bizknowledge.KnowledgeBlock
}

func (s *promoteBlockIndexStub) ReplaceDocBlocks(_ context.Context, collectionID, docID string, blocks []bizknowledge.KnowledgeBlock, _ []bizknowledge.KnowledgeBlockRefInput) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	stored := make([]bizknowledge.KnowledgeBlock, len(blocks))
	for i, b := range blocks {
		b.ID = docID + "-nb" + string(rune('0'+i))
		b.CollectionID = collectionID
		b.DocID = docID
		stored[i] = b
	}
	if s.blocksByDoc == nil {
		s.blocksByDoc = map[string][]bizknowledge.KnowledgeBlock{}
	}
	s.blocksByDoc[docID] = stored
	return nil, nil
}
func (s *promoteBlockIndexStub) ListDocBlocks(_ context.Context, docID string) ([]bizknowledge.KnowledgeBlock, error) {
	return s.blocksByDoc[docID], nil
}
func (s *promoteBlockIndexStub) UpdateDocLinkKeys(context.Context, string, string, []string) error {
	return nil
}

// promoteReaderStub mock PromoteBlockReader（按请求 ID 过滤）。
type promoteReaderStub struct {
	blocks []bizknowledge.KnowledgeBlock
	edges  []bizknowledge.KnowledgeBlockRefEdge
}

func (s promoteReaderStub) GetBlocksByIDs(_ context.Context, ids []string) ([]bizknowledge.KnowledgeBlock, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []bizknowledge.KnowledgeBlock
	for _, b := range s.blocks {
		if want[b.ID] {
			out = append(out, b)
		}
	}
	return out, nil
}
func (s promoteReaderStub) ListOutEdgesByBlocks(_ context.Context, blockIDs []string) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	want := map[string]bool{}
	for _, id := range blockIDs {
		want[id] = true
	}
	var out []bizknowledge.KnowledgeBlockRefEdge
	for _, e := range s.edges {
		if want[e.SrcBlockID] {
			out = append(out, e)
		}
	}
	return out, nil
}

// promoteLineageStub mock PromoteLineageWriter（捕获谱系对）。
type promoteLineageStub struct {
	pairs []bizknowledge.PromoteLineage
}

func (s *promoteLineageStub) WritePromoteLineage(_ context.Context, pairs []bizknowledge.PromoteLineage) error {
	s.pairs = append(s.pairs, pairs...)
	return nil
}

const promoteSvcSrcContent = "第一段私有思考。 ^t1\n\n第二段引用 [[私有文档B]]。\n"

// newPromoteService 装配：源库 pc1（local，含文档 sd1 两块）+ 团队库 tc1（team），
// 晋升三端口 + 块归属桩全部接线。返回 svc/repo/谱系桩句柄。
func newPromoteService(t *testing.T) (*KnowledgeService, *us14MemRepo, *promoteLineageStub) {
	t.Helper()
	repo := newUS14MemRepo()
	ctx := context.Background()
	for _, c := range []biz.KnowledgeCollection{
		{ID: "pc1", Name: "personal", VaultBackend: bizknowledge.VaultBackendLocal, Workspace: workspace.DefaultWorkspaceID},
		{ID: "tc1", Name: "team", VaultBackend: bizknowledge.VaultBackendTeam, Workspace: workspace.DefaultWorkspaceID},
	} {
		if _, err := repo.CreateCollection(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
		ID: "sd1", CollectionID: "pc1", RelPath: "notes/a.md", Source: "a.md",
		ContentText: promoteSvcSrcContent, Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}
	parsed, _, _ := blockparse.Parse("a.md", []byte(promoteSvcSrcContent))
	srcBlocks := []bizknowledge.KnowledgeBlock{
		{ID: "sb1", CollectionID: "pc1", DocID: "sd1", Ordinal: parsed[0].Ordinal, Anchor: "t1", ContentHash: parsed[0].ContentHash},
		{ID: "sb2", CollectionID: "pc1", DocID: "sd1", Ordinal: parsed[1].Ordinal, ContentHash: parsed[1].ContentHash},
	}
	lineageW := &promoteLineageStub{}
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	uc.SetBlockIndexRepos(&promoteBlockIndexStub{}, nil)
	uc.SetPromoteRepos(promoteReaderStub{
		blocks: srcBlocks,
		edges: []bizknowledge.KnowledgeBlockRefEdge{
			{CollectionID: "pc1", SrcBlockID: "sb2", DstCollectionID: "pc1", DstDocID: "pd2", RawTarget: "私有文档B"},
		},
	}, lineageW)
	uc.SetBacklinkRepos(&stubBacklinkBlockReader{owner: map[string]string{"sb1": "sd1", "sb2": "sd1"}}, nil)
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop())
	return svc, repo, lineageW
}

// TestKnowledgeService_PromoteBlocks_HappyPath 全链路：proto 映射（谱系对 +
// cascade 候选）+ 目标文档 chunk/FTS 重放（indexed + 计数）+ 谱系回写。
func TestKnowledgeService_PromoteBlocks_HappyPath(t *testing.T) {
	svc, repo, lineageW := newPromoteService(t)
	resp, err := svc.PromoteBlocks(context.Background(), &v1.PromoteBlocksRequest{
		BlockIds:           []string{"sb1", "sb2"},
		TargetCollectionId: "tc1",
	})
	if err != nil {
		t.Fatalf("PromoteBlocks: %v", err)
	}
	if len(resp.GetCreatedBlocks()) != 2 {
		t.Fatalf("created_blocks = %d, want 2", len(resp.GetCreatedBlocks()))
	}
	got := resp.GetCreatedBlocks()[0]
	if got.GetSrcBlockId() != "sb1" || got.GetNewBlockId() == "" || got.GetTargetDocId() == "" {
		t.Errorf("lineage[0] 映射错误: %+v", got)
	}
	if len(resp.GetCascadeCandidates()) != 1 {
		t.Fatalf("cascade_candidates = %d, want 1", len(resp.GetCascadeCandidates()))
	}
	cand := resp.GetCascadeCandidates()[0]
	if cand.GetSrcBlockId() != "sb2" || cand.GetRawTarget() != "私有文档B" || cand.GetDstDocId() != "pd2" || cand.GetDstCollectionId() != "pc1" {
		t.Errorf("candidate 映射错误: %+v", cand)
	}
	if len(lineageW.pairs) != 2 || lineageW.pairs[0].SrcBlockID != "sb1" {
		t.Errorf("谱系回写 = %+v", lineageW.pairs)
	}

	// B-3：目标文档 chunk/FTS 重放完成即可检索（status=indexed + chunk 计数落库）。
	targetDoc, err := repo.GetDocument(context.Background(), got.GetTargetDocId())
	if err != nil {
		t.Fatalf("目标文档应存在: %v", err)
	}
	if targetDoc.Status != "indexed" || targetDoc.ChunkCount == 0 {
		t.Errorf("目标文档未重放: status=%s chunks=%d", targetDoc.Status, targetDoc.ChunkCount)
	}
	if !strings.Contains(targetDoc.ContentText, "第一段私有思考。 ^t1") {
		t.Errorf("目标文档内容缺少晋升块文本: %q", targetDoc.ContentText)
	}
	tc, err := repo.GetCollection(context.Background(), "tc1")
	if err != nil {
		t.Fatal(err)
	}
	if tc.DocumentCount != 1 || tc.ChunkCount != targetDoc.ChunkCount {
		t.Errorf("库计数 = docs %d/chunks %d, want 1/%d", tc.DocumentCount, tc.ChunkCount, targetDoc.ChunkCount)
	}
}

// TestKnowledgeService_PromoteBlocks_AppendExisting 目标库已有同名文档 → 追加 +
// 重放后 docDelta=0（库文档计数不变），chunkDelta 按差值补齐。
func TestKnowledgeService_PromoteBlocks_AppendExisting(t *testing.T) {
	svc, repo, _ := newPromoteService(t)
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: "td1", CollectionID: "tc1", RelPath: "notes/a.md", Source: "a.md",
		ContentText: "# 已有\n\n团队既有内容。\n", Status: "indexed", ChunkCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateCollectionCounts(context.Background(), "tc1", 1, 1); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.PromoteBlocks(context.Background(), &v1.PromoteBlocksRequest{
		BlockIds:           []string{"sb2"},
		TargetCollectionId: "tc1",
	})
	if err != nil {
		t.Fatalf("PromoteBlocks: %v", err)
	}
	if len(resp.GetCreatedBlocks()) != 1 || resp.GetCreatedBlocks()[0].GetTargetDocId() != "td1" {
		t.Fatalf("created_blocks = %+v", resp.GetCreatedBlocks())
	}
	doc, _ := repo.GetDocument(context.Background(), "td1")
	if !strings.Contains(doc.ContentText, "团队既有内容。") || !strings.Contains(doc.ContentText, "第二段引用") {
		t.Errorf("追加内容错误: %q", doc.ContentText)
	}
	if doc.Status != "indexed" || doc.ChunkCount == 0 {
		t.Errorf("重放状态错误: status=%s chunks=%d", doc.Status, doc.ChunkCount)
	}
	tc, _ := repo.GetCollection(context.Background(), "tc1")
	if tc.DocumentCount != 1 {
		t.Errorf("既有文档追加不应增加文档计数, got %d", tc.DocumentCount)
	}
	if tc.ChunkCount != doc.ChunkCount {
		t.Errorf("chunk 计数漂移: 库 %d vs 文档 %d", tc.ChunkCount, doc.ChunkCount)
	}
}

// TestKnowledgeService_PromoteBlocks_Permissions 目标库跨租户写 / 源库跨租户
// （晋升回写源块 promoted_to 属写操作）均 NotFound 防泄漏。
func TestKnowledgeService_PromoteBlocks_Permissions(t *testing.T) {
	svc, repo, _ := newPromoteService(t)
	ctx := workspace.WithContext(context.Background(), "ws-mine")

	// 目标库他人所有 → mutate 拒绝。
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "tc2", Name: "other-team", VaultBackend: bizknowledge.VaultBackendTeam, Workspace: "ws-other",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PromoteBlocks(ctx, &v1.PromoteBlocksRequest{
		BlockIds: []string{"sb1"}, TargetCollectionId: "tc2",
	}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Errorf("跨租户目标库 = %v, want NotFound", err)
	}

	// 目标库归我、源库他人所有 → 源侧 mutate 拒绝。
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "tc3", Name: "my-team", VaultBackend: bizknowledge.VaultBackendTeam, Workspace: "ws-mine",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PromoteBlocks(ctx, &v1.PromoteBlocksRequest{
		BlockIds: []string{"sb1"}, TargetCollectionId: "tc3",
	}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Errorf("跨租户源库 = %v, want NotFound", err)
	}
}

// TestKnowledgeService_PromoteBlocks_Validation 非法输入矩阵。
func TestKnowledgeService_PromoteBlocks_Validation(t *testing.T) {
	svc, _, _ := newPromoteService(t)
	// 空 block_ids。
	if _, err := svc.PromoteBlocks(context.Background(), &v1.PromoteBlocksRequest{TargetCollectionId: "tc1"}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Errorf("空 block_ids = %v, want BadRequest", err)
	}
	// 未知目标库。
	if _, err := svc.PromoteBlocks(context.Background(), &v1.PromoteBlocksRequest{
		BlockIds: []string{"sb1"}, TargetCollectionId: "ghost",
	}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Errorf("未知目标库 = %v, want NotFound", err)
	}
	// 目标库非 team。
	if _, err := svc.PromoteBlocks(context.Background(), &v1.PromoteBlocksRequest{
		BlockIds: []string{"sb1"}, TargetCollectionId: "pc1",
	}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Errorf("local 目标库 = %v, want BadRequest", err)
	}
	// 未知块（归属解析 NotFound 透传）。
	if _, err := svc.PromoteBlocks(context.Background(), &v1.PromoteBlocksRequest{
		BlockIds: []string{"ghost"}, TargetCollectionId: "tc1",
	}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Errorf("未知块 = %v, want NotFound", err)
	}
}
