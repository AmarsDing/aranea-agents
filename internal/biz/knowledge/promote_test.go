package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/knowledge/blockparse"
	"aranea-agents/pkg/apierror"
)

// stubPromoteBlockIndex mock BlockIndexRepo：ReplaceDocBlocks 记录传入块并赋 ID，
// ListDocBlocks 返回最近一次重放结果（谱系匹配的数据源）。
type stubPromoteBlockIndex struct {
	blocksByDoc map[string][]KnowledgeBlock
}

func newStubPromoteBlockIndex() *stubPromoteBlockIndex {
	return &stubPromoteBlockIndex{blocksByDoc: map[string][]KnowledgeBlock{}}
}

func (s *stubPromoteBlockIndex) ReplaceDocBlocks(_ context.Context, collectionID, docID string, blocks []KnowledgeBlock, _ []KnowledgeBlockRefInput) ([]KnowledgeBlockRefEdge, error) {
	stored := make([]KnowledgeBlock, len(blocks))
	for i, b := range blocks {
		b.ID = docID + "-nb" + itoa(i)
		b.CollectionID = collectionID
		b.DocID = docID
		stored[i] = b
	}
	s.blocksByDoc[docID] = stored
	return nil, nil
}
func (s *stubPromoteBlockIndex) ListDocBlocks(_ context.Context, docID string) ([]KnowledgeBlock, error) {
	return s.blocksByDoc[docID], nil
}
func (s *stubPromoteBlockIndex) UpdateDocLinkKeys(context.Context, string, string, []string) error {
	return nil
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [8]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// stubPromoteReader mock PromoteBlockReader。
type stubPromoteReader struct {
	blocks []KnowledgeBlock
	edges  []KnowledgeBlockRefEdge
	err    error
}

func (s stubPromoteReader) GetBlocksByIDs(_ context.Context, ids []string) ([]KnowledgeBlock, error) {
	if s.err != nil {
		return nil, s.err
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []KnowledgeBlock
	for _, b := range s.blocks {
		if want[b.ID] {
			out = append(out, b)
		}
	}
	return out, nil
}
func (s stubPromoteReader) ListOutEdgesByBlocks(_ context.Context, blockIDs []string) ([]KnowledgeBlockRefEdge, error) {
	if s.err != nil {
		return nil, s.err
	}
	want := map[string]bool{}
	for _, id := range blockIDs {
		want[id] = true
	}
	var out []KnowledgeBlockRefEdge
	for _, e := range s.edges {
		if want[e.SrcBlockID] {
			out = append(out, e)
		}
	}
	return out, nil
}

// stubLineageWriter mock PromoteLineageWriter（捕获谱系对）。
type stubLineageWriter struct {
	pairs []PromoteLineage
	err   error
}

func (s *stubLineageWriter) WritePromoteLineage(_ context.Context, pairs []PromoteLineage) error {
	s.pairs = append(s.pairs, pairs...)
	return s.err
}

const promoteSrcContent = "# 笔记A\n\n第一段私有思考。 ^t1\n\n第二段引用 [[私有文档B]]。\n"

// promoteFixture 公共装配：源库 pc1 文档 sd1（notes/a.md），team 库 tc1。
// 返回 usecase + 各 mock 句柄。
func promoteFixture(t *testing.T, targetExists bool) (*Usecase, *mockRepo, *stubPromoteBlockIndex, *stubLineageWriter) {
	t.Helper()
	parsed, _, _ := blockparse.Parse("a.md", []byte(promoteSrcContent))
	srcBlocks := []KnowledgeBlock{
		{ID: "sb1", CollectionID: "pc1", DocID: "sd1", Ordinal: 1, Anchor: "t1", ContentHash: parsed[1].ContentHash},
		{ID: "sb2", CollectionID: "pc1", DocID: "sd1", Ordinal: 2, ContentHash: parsed[2].ContentHash},
	}
	repo := noOpMockRepo()
	repo.collGetFn = func(_ context.Context, id string) (Collection, error) {
		if id == "tc1" {
			return Collection{ID: "tc1", VaultBackend: VaultBackendTeam}, nil
		}
		return Collection{ID: id, VaultBackend: VaultBackendLocal}, nil
	}
	repo.docGetFn = func(_ context.Context, id string) (Document, error) {
		if id == "sd1" {
			return Document{ID: "sd1", CollectionID: "pc1", RelPath: "notes/a.md", Source: "a.md", ContentText: promoteSrcContent}, nil
		}
		return Document{ID: id}, nil
	}
	repo.docGetByRelFn = func(_ context.Context, collectionID, relPath string) (Document, error) {
		if targetExists && collectionID == "tc1" && relPath == "notes/a.md" {
			return Document{ID: "td1", CollectionID: "tc1", RelPath: relPath, ContentText: "# 已有\n\n团队既有内容。\n", Organized: true}, nil
		}
		return Document{}, apierror.NotFound("KNOWLEDGE", "doc not found")
	}
	repo.docCreateFn = func(_ context.Context, d Document) (Document, error) {
		d.ID = "td-new"
		return d, nil
	}
	blockIdx := newStubPromoteBlockIndex()
	lineageW := &stubLineageWriter{}
	u := NewUsecaseFromRepo(repo)
	u.SetBlockIndexRepos(blockIdx, nil)
	u.SetPromoteRepos(stubPromoteReader{blocks: srcBlocks}, lineageW)
	return u, repo, blockIdx, lineageW
}

// TestPromoteBlocks_CreateTargetDoc 目标库无同名文档 → 新建文档，内容为晋升块
// 文本拼接；谱系对回写（新块 promoted_from、源块 promoted_to）；touched 含新文档。
func TestPromoteBlocks_CreateTargetDoc(t *testing.T) {
	u, repo, _, lineageW := promoteFixture(t, false)
	var created Document
	repo.docCreateFn = func(_ context.Context, d Document) (Document, error) {
		created = d
		d.ID = "td-new"
		return d, nil
	}
	res, err := u.PromoteBlocks(context.Background(), []string{"sb1", "sb2"}, "tc1")
	if err != nil {
		t.Fatalf("PromoteBlocks: %v", err)
	}
	if created.RelPath == "" {
		t.Fatal("应创建目标文档")
	}
	if created.CollectionID != "tc1" || created.RelPath != "notes/a.md" {
		t.Errorf("目标文档归属/路径 = %s/%s", created.CollectionID, created.RelPath)
	}
	if !strings.Contains(created.ContentText, "第一段私有思考。 ^t1") ||
		!strings.Contains(created.ContentText, "第二段引用 [[私有文档B]]。") {
		t.Errorf("目标文档内容缺少晋升块文本: %q", created.ContentText)
	}
	if strings.Contains(created.ContentText, "# 笔记A") {
		t.Errorf("未晋升的块不应进入目标文档: %q", created.ContentText)
	}
	// 谱系对：两个源块各对应一个新块。
	if len(res.CreatedBlocks) != 2 {
		t.Fatalf("CreatedBlocks = %d, want 2", len(res.CreatedBlocks))
	}
	if res.CreatedBlocks[0].SrcBlockID != "sb1" || res.CreatedBlocks[0].TargetDocID != "td-new" {
		t.Errorf("lineage[0] = %+v", res.CreatedBlocks[0])
	}
	if res.CreatedBlocks[0].NewBlockID == "" || res.CreatedBlocks[1].NewBlockID == "" {
		t.Error("新块 ID 应非空")
	}
	if len(lineageW.pairs) != 2 {
		t.Fatalf("谱系回写 pairs = %d, want 2", len(lineageW.pairs))
	}
	if len(res.TouchedTargetDocs) != 1 || res.TouchedTargetDocs[0].DocID != "td-new" || !res.TouchedTargetDocs[0].Created {
		t.Errorf("TouchedTargetDocs = %v", res.TouchedTargetDocs)
	}
}

// TestPromoteBlocks_AppendExistingDoc 目标库已有同名文档 → 追加块文本到尾部。
func TestPromoteBlocks_AppendExistingDoc(t *testing.T) {
	u, repo, _, _ := promoteFixture(t, true)
	var appended string
	repo.docContentFn = func(_ context.Context, id, contentText string, _ bool) error {
		appended = contentText
		return nil
	}
	res, err := u.PromoteBlocks(context.Background(), []string{"sb2"}, "tc1")
	if err != nil {
		t.Fatalf("PromoteBlocks: %v", err)
	}
	if !strings.HasPrefix(appended, "# 已有\n\n团队既有内容。") {
		t.Errorf("追加应保留既有内容, got %q", appended)
	}
	if !strings.Contains(appended, "第二段引用 [[私有文档B]]。") {
		t.Errorf("追加应含晋升块文本, got %q", appended)
	}
	if len(res.CreatedBlocks) != 1 || res.CreatedBlocks[0].TargetDocID != "td1" {
		t.Errorf("CreatedBlocks = %+v", res.CreatedBlocks)
	}
}

// TestPromoteBlocks_CascadeCandidates 源块出边指向私有库文档 → cascade 候选；
// 指向目标 team 库或本已 dangling 的边不进候选。
func TestPromoteBlocks_CascadeCandidates(t *testing.T) {
	u, _, _, _ := promoteFixture(t, false)
	parsed, _, _ := blockparse.Parse("a.md", []byte(promoteSrcContent))
	u.SetPromoteRepos(stubPromoteReader{
		blocks: []KnowledgeBlock{
			{ID: "sb1", CollectionID: "pc1", DocID: "sd1", Ordinal: 1, Anchor: "t1", ContentHash: parsed[1].ContentHash},
			{ID: "sb2", CollectionID: "pc1", DocID: "sd1", Ordinal: 2, ContentHash: parsed[2].ContentHash},
		},
		edges: []KnowledgeBlockRefEdge{
			{CollectionID: "pc1", SrcBlockID: "sb2", DstCollectionID: "pc1", DstDocID: "pd2", RawTarget: "私有文档B"},
			{CollectionID: "pc1", SrcBlockID: "sb1", DstCollectionID: "tc1", DstDocID: "td9", RawTarget: "团队已有"},
			{CollectionID: "pc1", SrcBlockID: "sb1", RawTarget: "本就悬空"},
		},
	}, &stubLineageWriter{})
	res, err := u.PromoteBlocks(context.Background(), []string{"sb1", "sb2"}, "tc1")
	if err != nil {
		t.Fatalf("PromoteBlocks: %v", err)
	}
	if len(res.CascadeCandidates) != 1 {
		t.Fatalf("CascadeCandidates = %d, want 1: %+v", len(res.CascadeCandidates), res.CascadeCandidates)
	}
	c := res.CascadeCandidates[0]
	if c.SrcBlockID != "sb2" || c.RawTarget != "私有文档B" || c.DstDocID != "pd2" || c.DstCollectionID != "pc1" {
		t.Errorf("candidate = %+v", c)
	}
}

// TestPromoteBlocks_Validation 非法输入矩阵。
func TestPromoteBlocks_Validation(t *testing.T) {
	// 目标非 team。
	u, _, _, _ := promoteFixture(t, false)
	if _, err := u.PromoteBlocks(context.Background(), []string{"sb1"}, "pc1"); !errors.Is(err, ErrPromoteTargetNotTeam) {
		t.Errorf("local 目标 = %v, want ErrPromoteTargetNotTeam", err)
	}
	// 块缺失。
	u2, _, _, _ := promoteFixture(t, false)
	u2.SetPromoteRepos(stubPromoteReader{blocks: []KnowledgeBlock{
		{ID: "sb1", CollectionID: "pc1", DocID: "sd1", Ordinal: 1},
	}}, &stubLineageWriter{})
	if _, err := u2.PromoteBlocks(context.Background(), []string{"sb1", "ghost"}, "tc1"); err == nil {
		t.Error("缺失块应报错")
	}
	// 源块已在目标库。
	u3, _, _, _ := promoteFixture(t, false)
	u3.SetPromoteRepos(stubPromoteReader{blocks: []KnowledgeBlock{
		{ID: "tb1", CollectionID: "tc1", DocID: "td1", Ordinal: 0},
	}}, &stubLineageWriter{})
	if _, err := u3.PromoteBlocks(context.Background(), []string{"tb1"}, "tc1"); err == nil {
		t.Error("同库晋升应报错")
	}
	// 未接线 promote 端口。
	u4 := NewUsecaseFromRepo(noOpMockRepo())
	if _, err := u4.PromoteBlocks(context.Background(), []string{"sb1"}, "tc1"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("未接线 = %v, want ErrUnavailable", err)
	}
}
