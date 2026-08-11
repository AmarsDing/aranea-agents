package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ── P2-7：unlinked mentions（未链接提及） ────────────────────────────────────

// stubMentionSearcher 内容扫描端口桩。
type stubMentionSearcher struct {
	hits    []DocContentHit
	err     error
	gotColl string
	gotNeed string
	gotExcl string
	gotLim  int
}

func (s *stubMentionSearcher) SearchDocContentMentions(_ context.Context, collectionID, needle, excludeDocID string, limit int) ([]DocContentHit, error) {
	s.gotColl, s.gotNeed, s.gotExcl, s.gotLim = collectionID, needle, excludeDocID, limit
	return s.hits, s.err
}

func mentionTargetRepo() *mockRepo {
	m := noOpMockRepo()
	m.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "c1", RelPath: "notes/目标笔记.md"}, nil
	}
	return m
}

// TestUsecase_ListUnlinkedMentions 核心语义：[[...]] 内出现不计、大小写不敏感、
// 计数排序 + 片段生成；端口参数（collection/needle/exclude/limit）正确透传。
func TestUsecase_ListUnlinkedMentions(t *testing.T) {
	searcher := &stubMentionSearcher{hits: []DocContentHit{
		{DocID: "d2", DocName: "a.md", Content: "这篇提到目标笔记一次。"},
		{DocID: "d3", DocName: "b.md", Content: "目标笔记 与 [[目标笔记]] 再 目标笔记。"}, // 2 次纯文本 + 1 次链接内
		{DocID: "d4", DocName: "c.md", Content: "仅在链接里 [[目标笔记|别名]]。"},          // 剔除后 0 次，不出现
		{DocID: "d5", DocName: "d.md", Content: "大小写 TARGET笔记 不算（子串不等）。"},     // 中文场景不命中
	}}
	u := NewUsecaseFromRepo(mentionTargetRepo())
	u.SetMentionSearcher(searcher)

	items, err := u.ListUnlinkedMentions(context.Background(), "d1")
	if err != nil {
		t.Fatalf("ListUnlinkedMentions: %v", err)
	}
	if searcher.gotColl != "c1" || searcher.gotNeed != "目标笔记" || searcher.gotExcl != "d1" || searcher.gotLim != mentionCandidateLimit {
		t.Errorf("端口参数 = (%q,%q,%q,%d)", searcher.gotColl, searcher.gotNeed, searcher.gotExcl, searcher.gotLim)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2: %+v", len(items), items)
	}
	// Count 降序：d3(2) 在 d2(1) 前。
	if items[0].SrcDocID != "d3" || items[0].Count != 2 {
		t.Errorf("items[0] = %+v, want d3 count=2", items[0])
	}
	if items[1].SrcDocID != "d2" || items[1].Count != 1 {
		t.Errorf("items[1] = %+v, want d2 count=1", items[1])
	}
	if !strings.Contains(items[1].Snippet, "目标笔记") {
		t.Errorf("snippet 缺首次出现上下文: %q", items[1].Snippet)
	}
}

// TestUsecase_ListUnlinkedMentions_CaseInsensitive 英文目标大小写不敏感匹配。
func TestUsecase_ListUnlinkedMentions_CaseInsensitive(t *testing.T) {
	m := noOpMockRepo()
	m.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "c1", Source: "Deep Work.md"}, nil
	}
	searcher := &stubMentionSearcher{hits: []DocContentHit{
		{DocID: "d2", DocName: "x.md", Content: "I love deep work and DEEP WORK habits."},
	}}
	u := NewUsecaseFromRepo(m)
	u.SetMentionSearcher(searcher)

	items, err := u.ListUnlinkedMentions(context.Background(), "d1")
	if err != nil {
		t.Fatalf("ListUnlinkedMentions: %v", err)
	}
	if len(items) != 1 || items[0].Count != 2 {
		t.Fatalf("items = %+v, want 1 item count=2", items)
	}
}

// TestUsecase_ListUnlinkedMentions_Degrade 端口未接线 / 短名 / 空 docID 的降级与校验。
func TestUsecase_ListUnlinkedMentions_Degrade(t *testing.T) {
	// 端口未接线 → 空。
	u := NewUsecaseFromRepo(mentionTargetRepo())
	items, err := u.ListUnlinkedMentions(context.Background(), "d1")
	if err != nil || len(items) != 0 {
		t.Errorf("未接线应降级为空, got items=%v err=%v", items, err)
	}
	// 空 docID → ErrIDRequired。
	if _, err := u.ListUnlinkedMentions(context.Background(), "  "); !errors.Is(err, ErrIDRequired) {
		t.Errorf("空 docID err = %v, want ErrIDRequired", err)
	}
	// 单字符名 → 空（噪声守卫）。
	m := mentionTargetRepo()
	m.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "c1", RelPath: "中.md"}, nil
	}
	u2 := NewUsecaseFromRepo(m)
	searcher := &stubMentionSearcher{err: errors.New("should not be called")}
	u2.SetMentionSearcher(searcher)
	items, err = u2.ListUnlinkedMentions(context.Background(), "d1")
	if err != nil || len(items) != 0 {
		t.Errorf("单字符名应返回空, got items=%v err=%v", items, err)
	}
}

// TestUsecase_ListUnlinkedMentions_SearchErr 端口错误透传。
func TestUsecase_ListUnlinkedMentions_SearchErr(t *testing.T) {
	u := NewUsecaseFromRepo(mentionTargetRepo())
	u.SetMentionSearcher(&stubMentionSearcher{err: errors.New("db down")})
	if _, err := u.ListUnlinkedMentions(context.Background(), "d1"); err == nil {
		t.Error("端口错误应透传")
	}
}

// TestMentionNeedle 显示名提取：basename + 去扩展名。
func TestMentionNeedle(t *testing.T) {
	cases := []struct{ rel, src, want string }{
		{"notes/Sub/My Note.md", "", "My Note"},
		{"", "Report.PDF", "Report"},
		{"plain", "", "plain"},
		{"a/b/.md", "", ""},      // 无主干名（dotfile）→ 空
		{"", "  ", ""},           // 全空 → 空
		{"dir/无扩展", "", "无扩展"}, // 无扩展名保留
	}
	for _, c := range cases {
		if got := mentionNeedle(c.rel, c.src); got != c.want {
			t.Errorf("mentionNeedle(%q,%q) = %q, want %q", c.rel, c.src, got, c.want)
		}
	}
}
