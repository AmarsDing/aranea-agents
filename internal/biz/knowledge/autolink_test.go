package knowledge

import (
	"context"
	"strings"
	"testing"
)

func TestAutolinkWikiMentions_CJK(t *testing.T) {
	got, n := AutolinkWikiMentions("这篇提到目标笔记一次。", "", []string{"目标笔记"})
	if n != 1 || got != "这篇提到[[目标笔记]]一次。" {
		t.Fatalf("got %q n=%d", got, n)
	}
}

func TestAutolinkWikiMentions_ASCIIWholeWord(t *testing.T) {
	got, n := AutolinkWikiMentions("I love deep work and networking.", "", []string{"Deep Work"})
	if n != 1 || got != "I love [[Deep Work]] and networking." {
		t.Fatalf("got %q n=%d", got, n)
	}
	// "work" 不得切 "Deep Work" 已占用区间；也不得切 "networking"
	got, n = AutolinkWikiMentions("The network is working.", "", []string{"work"})
	if n != 0 {
		t.Fatalf("short ascii skipped, got %q n=%d", got, n)
	}
	got, n = AutolinkWikiMentions("The network is working hard.", "", []string{"working"})
	if n != 1 || !strings.Contains(got, "[[working]]") {
		t.Fatalf("whole word working: %q n=%d", got, n)
	}
}

func TestAutolinkWikiMentions_SkipProtected(t *testing.T) {
	src := "---\ntitle: 目标笔记\n---\n见 [[目标笔记]] 与 `目标笔记` 和\n```\n目标笔记\n```\n还有 [x](https://example.com/目标笔记) 与正文目标笔记。"
	got, n := AutolinkWikiMentions(src, "", []string{"目标笔记"})
	if n != 1 {
		t.Fatalf("n=%d got=%q", n, got)
	}
	if strings.Count(got, "[[目标笔记]]") != 2 { // 原有 1 + 新 1
		t.Fatalf("wikilink count: %q", got)
	}
	if !strings.Contains(got, "正文[[目标笔记]]。") && !strings.Contains(got, "正文[[目标笔记]]") {
		t.Fatalf("plain mention not wrapped: %q", got)
	}
	if !strings.Contains(got, "`目标笔记`") {
		t.Fatalf("inline code mutated: %q", got)
	}
}

func TestAutolinkWikiMentions_SkipFenceAndURL(t *testing.T) {
	src := "见 https://example.com/DeepWork 页。正文 Deep Work。"
	got, n := AutolinkWikiMentions(src, "", []string{"Deep Work"})
	if n != 1 || strings.Contains(got, "https://example.com/[[") {
		t.Fatalf("url mutated: %q n=%d", got, n)
	}
	if !strings.Contains(got, "正文 [[Deep Work]]。") {
		t.Fatalf("plain not wrapped: %q", got)
	}
}

func TestAutolinkWikiMentions_AmbiguousAndSelf(t *testing.T) {
	got, n := AutolinkWikiMentions("见 目标笔记。", "目标笔记", []string{"目标笔记", "其他"})
	if n != 0 {
		t.Fatalf("self should skip: %q n=%d", got, n)
	}
	got, n = AutolinkWikiMentions("见 同名。", "", []string{"同名", "同名"})
	if n != 0 {
		t.Fatalf("ambiguous should skip: %q n=%d", got, n)
	}
}

func TestAutolinkWikiMentions_LongestFirst(t *testing.T) {
	got, n := AutolinkWikiMentions("讨论通信协议细节。", "", []string{"协议", "通信协议"})
	if n != 1 || got != "讨论[[通信协议]]细节。" {
		t.Fatalf("got %q n=%d", got, n)
	}
}

func TestAutolinkWikiMentions_Idempotent(t *testing.T) {
	once, n := AutolinkWikiMentions("见 目标笔记。", "", []string{"目标笔记"})
	if n != 1 {
		t.Fatalf("first n=%d", n)
	}
	twice, n2 := AutolinkWikiMentions(once, "", []string{"目标笔记"})
	if n2 != 0 || twice != once {
		t.Fatalf("not idempotent: %q n=%d", twice, n2)
	}
}

func TestAutolinkWikiMentions_CaseInsensitiveCanonical(t *testing.T) {
	got, n := AutolinkWikiMentions("I love DEEP WORK.", "", []string{"Deep Work"})
	if n != 1 || got != "I love [[Deep Work]]." {
		t.Fatalf("got %q n=%d", got, n)
	}
}

func TestMaybeAutolinkOutgoing(t *testing.T) {
	m := noOpMockRepo()
	m.docListFn = func(_ context.Context, collectionID string, _, _ int) ([]Document, int, error) {
		if collectionID != "c1" {
			t.Errorf("collection = %q", collectionID)
		}
		return []Document{
			{ID: "d1", RelPath: "notes/目标笔记.md"},
			{ID: "d2", RelPath: "notes/其他.md"},
		}, 2, nil
	}
	u := NewUsecaseFromRepo(m)
	got := u.MaybeAutolinkOutgoing(context.Background(), "c1", "d2", "", "请看目标笔记。")
	if got != "请看[[目标笔记]]。" {
		t.Fatalf("got %q", got)
	}
	// 排除自身标题
	got = u.MaybeAutolinkOutgoing(context.Background(), "c1", "d1", "", "请看目标笔记。")
	if got != "请看目标笔记。" {
		t.Fatalf("self still linked: %q", got)
	}
	// 列出失败降级
	m.docListFn = func(_ context.Context, _ string, _, _ int) ([]Document, int, error) {
		return nil, 0, context.Canceled
	}
	orig := "请看目标笔记。"
	if u.MaybeAutolinkOutgoing(context.Background(), "c1", "", "", orig) != orig {
		t.Fatal("list error should degrade")
	}
}

func TestRebuildBlockIndex_CompilesMentionsWithoutRewritingSource(t *testing.T) {
	f := newBackfillFixture()
	f.collections["c1"] = Collection{ID: "c1", Workspace: "w"}
	f.docs["d-t"] = Document{ID: "d-t", CollectionID: "c1", RelPath: "notes/目标笔记.md", ContentText: "目标正文"}
	srcBody := "这篇提到目标笔记一次。"
	f.docs["d-s"] = Document{ID: "d-s", CollectionID: "c1", RelPath: "notes/src.md", ContentText: srcBody}
	f.candidatesFn = func() []ResolveDocCandidate {
		return []ResolveDocCandidate{
			{DocID: "d-t", CollectionID: "c1", RelPath: "notes/目标笔记.md"},
			{DocID: "d-s", CollectionID: "c1", RelPath: "notes/src.md"},
		}
	}
	links := &stubLinkRepo{}
	f.u.SetLinkRepos(links, nil)
	if err := f.u.RebuildBlockIndex(context.Background(), "c1", "d-s", srcBody); err != nil {
		t.Fatalf("RebuildBlockIndex: %v", err)
	}
	if f.docs["d-s"].ContentText != srcBody {
		t.Fatalf("source rewritten: %q", f.docs["d-s"].ContentText)
	}
	if len(f.contentUpdates) != 0 {
		t.Fatalf("content updates: %v", f.contentUpdates)
	}
	found := false
	for _, l := range links.links {
		if l.DocID == "d-s" && l.TargetDocID == "d-t" && l.LinkType == LinkTypeExplicit {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected explicit mention link, got %+v", links.links)
	}
}
