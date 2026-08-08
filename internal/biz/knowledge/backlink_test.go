package knowledge

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// ── SP1-E：块级反链 / dangling 聚合查询 ─────────────────────────────────────

// stubBlockLinkReader DB 兜底端口桩。
type stubBlockLinkReader struct {
	byBlock  []KnowledgeBlockRefEdge
	byDoc    []KnowledgeBlockRefEdge
	dangling []KnowledgeBlockRefEdge
	err      error
}

func (s stubBlockLinkReader) ListBacklinksByBlock(context.Context, string) ([]KnowledgeBlockRefEdge, error) {
	return s.byBlock, s.err
}
func (s stubBlockLinkReader) ListBacklinksByDoc(context.Context, string) ([]KnowledgeBlockRefEdge, error) {
	return s.byDoc, s.err
}
func (s stubBlockLinkReader) ListDanglingEdges(context.Context, string) ([]KnowledgeBlockRefEdge, error) {
	return s.dangling, s.err
}
func (s stubBlockLinkReader) GetBlockOwnerDoc(context.Context, string) (string, error) {
	return "", s.err
}

// stubDocNames 源文档名批量解析桩。
type stubDocNames struct {
	names map[string]string
	err   error
}

func (s stubDocNames) ListDocumentNames(context.Context, []string) (map[string]string, error) {
	return s.names, s.err
}

// TestUsecase_ListBlockBacklinks_MemIndex 内存图已加载：块级反链直读内存图，
// 源文档名批量填充，结果按 (SrcDocID, SrcBlockID) 确定性排序。
func TestUsecase_ListBlockBacklinks_MemIndex(t *testing.T) {
	idx := NewLinkIndex()
	e1 := refEdge("c1", "d2", "b2", "c1", "d1", "tb", "T#^a") // dst 块 tb
	e2 := refEdge("c1", "d1", "b1", "c1", "d1", "tb", "#^self")
	e3 := refEdge("c1", "d3", "b3", "c1", "d1", "", "D1") // 文档级入边（dst_block 空）
	idx.LoadAll([]KnowledgeBlockRefEdge{e3, e1, e2})      // 乱序加载，验证输出排序

	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkIndex(idx, nil)
	// DB 兜底也接线但不应被命中（图已加载）。
	u.SetBacklinkRepos(stubBlockLinkReader{err: errors.New("should not be called")}, stubDocNames{names: map[string]string{
		"d1": "notes/a.md", "d2": "notes/b.md", "d3": "notes/c.md",
	}})

	items, err := u.ListBlockBacklinks(context.Background(), "tb", "")
	if err != nil {
		t.Fatalf("ListBlockBacklinks: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("块级反链 = %d, want 2（文档级边不进块级反链）", len(items))
	}
	// 排序：(d1,b1) < (d2,b2)。
	if items[0].SrcDocID != "d1" || items[1].SrcDocID != "d2" {
		t.Fatalf("排序错误: %+v", items)
	}
	if items[1].SrcDocName != "notes/b.md" || items[1].RawTarget != "T#^a" || items[1].Context == "" {
		t.Errorf("字段填充错误: %+v", items[1])
	}
}

// TestUsecase_ListBlockBacklinks_DocAggregate docID 优先：聚合该文档全部入边
// （块级 + 文档级），忽略 blockID。
func TestUsecase_ListBlockBacklinks_DocAggregate(t *testing.T) {
	idx := NewLinkIndex()
	e1 := refEdge("c1", "d2", "b2", "c1", "d1", "tb", "T#^a")
	e2 := refEdge("c1", "d3", "b3", "c1", "d1", "", "D1") // 文档级
	idx.LoadAll([]KnowledgeBlockRefEdge{e1, e2})

	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkIndex(idx, nil)
	u.SetBacklinkRepos(nil, stubDocNames{names: map[string]string{"d2": "b.md", "d3": "c.md"}})

	items, err := u.ListBlockBacklinks(context.Background(), "ignored-block", "d1")
	if err != nil {
		t.Fatalf("ListBlockBacklinks: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("文档聚合反链 = %d, want 2", len(items))
	}
	if items[0].SrcDocName != "b.md" || items[1].SrcDocName != "c.md" {
		t.Errorf("名字填充错误: %+v", items)
	}
}

// TestUsecase_ListBlockBacklinks_DBFallback 启动窗口（图未加载）走 DB 兜底；
// 图加载后不再命中 DB。
func TestUsecase_ListBlockBacklinks_DBFallback(t *testing.T) {
	edge := refEdge("c1", "d2", "b2", "c1", "d1", "tb", "T#^a")
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkIndex(NewLinkIndex(), nil) // 未 LoadAll → loaded=false
	u.SetBacklinkRepos(stubBlockLinkReader{byBlock: []KnowledgeBlockRefEdge{edge}},
		stubDocNames{names: map[string]string{"d2": "b.md"}})

	items, err := u.ListBlockBacklinks(context.Background(), "tb", "")
	if err != nil {
		t.Fatalf("ListBlockBacklinks: %v", err)
	}
	if len(items) != 1 || items[0].SrcDocName != "b.md" {
		t.Fatalf("DB 兜底结果错误: %+v", items)
	}
}

// TestUsecase_ListBlockBacklinks_Validation 参数校验与双端口均未接线降级。
func TestUsecase_ListBlockBacklinks_Validation(t *testing.T) {
	u := NewUsecaseFromRepo(noOpMockRepo())
	if _, err := u.ListBlockBacklinks(context.Background(), "", ""); !errors.Is(err, ErrIDRequired) {
		t.Fatalf("双空参数 err = %v, want ErrIDRequired", err)
	}
	// 图未接线 + 兜底未接线 → 空降级（不报错）。
	items, err := u.ListBlockBacklinks(context.Background(), "b1", "")
	if err != nil || len(items) != 0 {
		t.Fatalf("未接线降级 = %v/%v, want 空/nil", items, err)
	}
	// 空白字符等同空。
	if _, err := u.ListBlockBacklinks(context.Background(), "  ", ""); !errors.Is(err, ErrIDRequired) {
		t.Fatalf("空白参数 err = %v, want ErrIDRequired", err)
	}
}

// TestUsecase_ListBlockBacklinks_NameFallback 名字解析失败/缺失不阻塞主查询
// （名字留空，边正常返回）。
func TestUsecase_ListBlockBacklinks_NameFallback(t *testing.T) {
	idx := NewLinkIndex()
	idx.LoadAll([]KnowledgeBlockRefEdge{refEdge("c1", "d2", "b2", "c1", "d1", "tb", "T")})
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkIndex(idx, nil)
	u.SetBacklinkRepos(nil, stubDocNames{err: fmt.Errorf("db down")})

	items, err := u.ListBlockBacklinks(context.Background(), "tb", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("名字失败应降级: %v/%v", items, err)
	}
	if items[0].SrcDocName != "" {
		t.Errorf("名字失败应留空, got %q", items[0].SrcDocName)
	}
}

// TestUsecase_ListDanglingLinks dangling 按 raw_target 聚合：ref_count 降序 +
// raw_target 字典序；组内 refs 按 (SrcDocID, SrcBlockID) 排序；名字填充。
func TestUsecase_ListDanglingLinks(t *testing.T) {
	idx := NewLinkIndex()
	g1 := refEdge("c1", "d2", "b2", "", "", "", "Ghost")
	g2 := refEdge("c1", "d1", "b1", "", "", "", "Ghost")
	other := refEdge("c1", "d3", "b3", "", "", "", "Another")
	resolved := refEdge("c1", "d1", "b4", "c1", "d9", "", "D9") // 非 dangling
	idx.LoadAll([]KnowledgeBlockRefEdge{g1, other, g2, resolved})

	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkIndex(idx, nil)
	u.SetBacklinkRepos(nil, stubDocNames{names: map[string]string{"d1": "a.md", "d2": "b.md", "d3": "c.md"}})

	items, err := u.ListDanglingLinks(context.Background(), "c1")
	if err != nil {
		t.Fatalf("ListDanglingLinks: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("聚合组 = %d, want 2", len(items))
	}
	// Ghost(2) 排前，Another(1) 在后。
	if items[0].RawTarget != "Ghost" || items[0].RefCount != 2 || items[1].RawTarget != "Another" || items[1].RefCount != 1 {
		t.Fatalf("聚合排序错误: %+v", items)
	}
	// 组内 (d1,b1) < (d2,b2)。
	if items[0].Refs[0].SrcDocID != "d1" || items[0].Refs[1].SrcDocID != "d2" {
		t.Fatalf("组内排序错误: %+v", items[0].Refs)
	}
	if items[0].Refs[0].SrcDocName != "a.md" {
		t.Errorf("refs 名字填充错误: %+v", items[0].Refs[0])
	}
}

// TestUsecase_ListDanglingLinks_DBFallback 图未加载走 DB 兜底；collectionID
// 必填校验；双端口未接线降级为空。
func TestUsecase_ListDanglingLinks_DBFallback(t *testing.T) {
	u := NewUsecaseFromRepo(noOpMockRepo())
	if _, err := u.ListDanglingLinks(context.Background(), ""); !errors.Is(err, ErrCollectionIDRequired) {
		t.Fatalf("空 collectionID err = %v, want ErrCollectionIDRequired", err)
	}

	edge := refEdge("c1", "d2", "b2", "", "", "", "Ghost")
	u2 := NewUsecaseFromRepo(noOpMockRepo())
	u2.SetLinkIndex(NewLinkIndex(), nil) // 未加载
	u2.SetBacklinkRepos(stubBlockLinkReader{dangling: []KnowledgeBlockRefEdge{edge}}, nil)
	items, err := u2.ListDanglingLinks(context.Background(), "c1")
	if err != nil || len(items) != 1 || items[0].RefCount != 1 {
		t.Fatalf("DB 兜底聚合错误: %+v/%v", items, err)
	}

	u3 := NewUsecaseFromRepo(noOpMockRepo())
	items3, err := u3.ListDanglingLinks(context.Background(), "c1")
	if err != nil || len(items3) != 0 {
		t.Fatalf("未接线降级 = %v/%v, want 空/nil", items3, err)
	}
}
