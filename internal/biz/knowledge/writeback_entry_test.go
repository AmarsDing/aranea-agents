package knowledge

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
)

// ── P0 词条优先写回（2026-08-15 评审修订三刀） ──────────────────────────────

func TestEntrySlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"灰度发布", "灰度发布"},
		{"Deep Work", "deep-work"},
		{"on_call 制度", "on-call-制度"},
		{"  -- Leading - and trailing -- ", "leading-and-trailing"},
		{"!!!", ""},
	}
	for _, c := range cases {
		if got := entrySlug(c.in); got != c.want {
			t.Errorf("entrySlug(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizedEntryTags(t *testing.T) {
	got := normalizedEntryTags([]string{" 灰度 ", "灰度", "x", "", "发布制度"})
	want := []string{"灰度", "发布制度"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestReplaceH2BlockContaining(t *testing.T) {
	body := "# 词条\n\n> 前言 fact_id: `x` 不算块。\n\n## preference\n\n旧陈述。\n\n- fact_id: `fid-1`\n\n## constraint\n\n别的段。\n"
	// 命中中间小节：整段替换，前后保留。
	nb, ok := replaceH2BlockContaining(body, "fact_id: `fid-1`", "## preference\n\n新陈述。\n\n- fact_id: `fid-1`")
	if !ok {
		t.Fatal("want replaced")
	}
	if !strings.Contains(nb, "新陈述。") || strings.Contains(nb, "旧陈述。") {
		t.Fatalf("old section not replaced: %q", nb)
	}
	if !strings.Contains(nb, "## constraint\n\n别的段。") || !strings.HasPrefix(nb, "# 词条") {
		t.Fatalf("neighbours clobbered: %q", nb)
	}
	// 命中末尾小节。
	nb, ok = replaceH2BlockContaining(body, "别的段", "## constraint\n\n新尾段。")
	if !ok || !strings.HasSuffix(strings.TrimSpace(nb), "新尾段。") {
		t.Fatalf("tail replace: ok=%v %q", ok, nb)
	}
	// marker 在前言区（非事实块）：不替换。
	if _, ok = replaceH2BlockContaining(body, "fact_id: `x`", "## preference\n\ny"); ok {
		t.Fatal("preamble marker must not replace")
	}
	// marker 不存在。
	if _, ok = replaceH2BlockContaining(body, "fact_id: `nope`", "## z"); ok {
		t.Fatal("missing marker must return false")
	}
}

// writeBackEntryFixture 装配词条写回测试环境：内存文档/集合存根 + 内存解析索引。
type writeBackEntryFixture struct {
	repo        *mockRepo
	idx         *memBlockIndex
	u           *Usecase
	docs        map[string]Document // docID → doc
	byRel       map[string]string   // collectionID|rel → docID
	candidates  []ResolveDocCandidate
	createCount int
	seq         int
}

func newWriteBackEntryFixture() *writeBackEntryFixture {
	f := &writeBackEntryFixture{
		docs:  map[string]Document{},
		byRel: map[string]string{},
	}
	repo := noOpMockRepo()
	repo.collListFn = func(_ context.Context, _ string, _, _ int) ([]Collection, int, error) {
		return []Collection{{ID: "team-1", Name: WriteBackCollectionName, VaultBackend: VaultBackendTeam}}, 1, nil
	}
	repo.collGetFn = func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, Name: WriteBackCollectionName, VaultBackend: VaultBackendTeam}, nil
	}
	repo.docGetByRelFn = func(_ context.Context, collectionID, rel string) (Document, error) {
		if id, ok := f.byRel[collectionID+"|"+rel]; ok {
			return f.docs[id], nil
		}
		return Document{}, apierror.NotFound("KNOWLEDGE", "missing %s", rel)
	}
	repo.docGetFn = func(_ context.Context, id string) (Document, error) {
		if d, ok := f.docs[id]; ok {
			return d, nil
		}
		return Document{}, apierror.NotFound("KNOWLEDGE", "missing %s", id)
	}
	repo.docCreateFn = func(_ context.Context, d Document) (Document, error) {
		f.seq++
		d.ID = fmt.Sprintf("doc-new-%d", f.seq)
		f.docs[d.ID] = d
		f.byRel[d.CollectionID+"|"+d.RelPath] = d.ID
		f.createCount++
		return d, nil
	}
	repo.docContentFn = func(_ context.Context, id, contentText string, organized bool) error {
		d := f.docs[id]
		d.ContentText = contentText
		d.Organized = organized
		f.docs[id] = d
		return nil
	}
	repo.docListFn = func(_ context.Context, collectionID string, _, _ int) ([]Document, int, error) {
		return nil, 0, nil
	}
	f.repo = repo
	f.idx = newMemBlockIndex(func() []ResolveDocCandidate { return f.candidates })
	u := NewUsecaseFromRepo(repo)
	u.SetBlockIndexRepos(f.idx, f.idx)
	f.u = u
	return f
}

func (f *writeBackEntryFixture) docByRel(rel string) (Document, bool) {
	id, ok := f.byRel["team-1|"+rel]
	if !ok {
		return Document{}, false
	}
	return f.docs[id], true
}

func taggedFact(id, stmt, kind string, conf float64, tags ...string) WriteBackFact {
	return WriteBackFact{FactID: id, Statement: stmt, FactKind: kind, Confidence: conf, SourceKind: "auto_memory", Tags: tags}
}

// P0-1：带 tags 事实落词条页（新建），日记保留 provenance 并带词条指针。
func TestWriteBack_TaggedFactCreatesEntryAndDiaryPointer(t *testing.T) {
	f := newWriteBackEntryFixture()
	res, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-1",
		Facts:     []WriteBackFact{taggedFact("fid-1", "部署必须走灰度且保留回滚开关", "constraint", 0.95, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Appended != 1 {
		t.Fatalf("diary result = %+v", res)
	}
	entry, ok := f.docByRel("entries/灰度发布.md")
	if !ok {
		t.Fatalf("entry doc not created: %v", f.byRel)
	}
	if !strings.Contains(entry.ContentText, "title: 灰度发布") || !strings.Contains(entry.ContentText, "# 灰度发布") {
		t.Fatalf("entry header missing: %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "fact_id: `fid-1`") || !strings.Contains(entry.ContentText, "部署必须走灰度") {
		t.Fatalf("fact block missing in entry: %q", entry.ContentText)
	}
	// 日记按当天 UTC 切分；直接扫 inbox 前缀定位。
	var diary Document
	diaryFound := false
	for k, id := range f.byRel {
		if strings.Contains(k, "inbox/writeback-") {
			diary = f.docs[id]
			diaryFound = true
		}
	}
	if !diaryFound {
		t.Fatal("diary not written")
	}
	if !strings.Contains(diary.ContentText, "entry: [[灰度发布]]") {
		t.Fatalf("diary missing entry pointer: %q", diary.ContentText)
	}
}

// P0-1：tag 命中已有词条别名 → upsert 进旧页，不新建。
func TestWriteBack_TagMatchesEntryByAlias(t *testing.T) {
	f := newWriteBackEntryFixture()
	existing := Document{
		ID: "doc-entry", CollectionID: "team-1", RelPath: "entries/release-train.md",
		ContentText: writeBackEntryHeader([]string{"发布火车", "灰度"}) + "## decision\n\n既有决策。\n",
	}
	f.docs[existing.ID] = existing
	f.byRel["team-1|entries/release-train.md"] = existing.ID
	f.candidates = []ResolveDocCandidate{{
		DocID: existing.ID, CollectionID: "team-1", RelPath: existing.RelPath,
		Title: "发布火车", Aliases: []string{"灰度"},
	}}

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-2",
		Facts:     []WriteBackFact{taggedFact("fid-2", "发布窗口固定在周四下午", "decision", 0.9, "灰度")},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := f.docs["doc-entry"]
	if !strings.Contains(got.ContentText, "发布窗口固定在周四下午") || !strings.Contains(got.ContentText, "既有决策。") {
		t.Fatalf("upsert into existing entry failed: %q", got.ContentText)
	}
	for rel := range f.byRel {
		if strings.Contains(rel, "entries/灰度.md") {
			t.Fatal("alias hit must not create a new entry page")
		}
	}
}

// P0-3：同一 fact_id 再写入 → 词条旧段被替换，不追加、不重复。
func TestWriteBack_SameFactIDReplacesEntrySection(t *testing.T) {
	f := newWriteBackEntryFixture()
	in := WriteBackInput{
		SessionID: "sess-3",
		Facts:     []WriteBackFact{taggedFact("fid-1", "值班每 8 小时轮换", "constraint", 0.9, "值班制度")},
	}
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	in.Facts[0].Statement = "值班改为每 12 小时轮换"
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	entry, ok := f.docByRel("entries/值班制度.md")
	if !ok {
		t.Fatal("entry missing")
	}
	if strings.Count(entry.ContentText, "fact_id: `fid-1`") != 1 {
		t.Fatalf("fact_id duplicated: %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "每 12 小时轮换") || strings.Contains(entry.ContentText, "每 8 小时轮换") {
		t.Fatalf("old statement not replaced: %q", entry.ContentText)
	}
}

// P0-1：无 tags 事实回退日记流水，不造词条页。
func TestWriteBack_NoTagsFallsBackToDiary(t *testing.T) {
	f := newWriteBackEntryFixture()
	res, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-4b",
		Facts:     []WriteBackFact{taggedFact("fid-9b", "无标签事实只进日记", "decision", 0.9)},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 无 tags → 无词条改动，重放集合为空（service 层只重放日记）。
	if len(res.EntryDocs) != 0 {
		t.Fatalf("untagged fact must not touch entries: %+v", res.EntryDocs)
	}
	for rel := range f.byRel {
		if strings.Contains(rel, "entries/") {
			t.Fatalf("untagged fact must not create entry: %s", rel)
		}
	}
}

// ── 词条 touched docs 收集（2026-08-15 修复：service 层据此重放 chunk/FTS）──────

// 新建词条 → EntryDocs 含新建文档（Created=true）；同页两条事实只记一次 touched。
func TestWriteBack_EntryDocsTouched_NewEntry(t *testing.T) {
	f := newWriteBackEntryFixture()
	res, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-t1",
		Facts: []WriteBackFact{
			taggedFact("fid-a", "部署必须走灰度且保留回滚开关", "constraint", 0.95, "灰度发布"),
			taggedFact("fid-b", "灰度比例从 5% 起步按天递增", "decision", 0.9, "灰度发布"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.EntryDocs) != 1 {
		t.Fatalf("same entry page must be touched once: %+v", res.EntryDocs)
	}
	entry, _ := f.docByRel("entries/灰度发布.md")
	if res.EntryDocs[0].DocID != entry.ID || !res.EntryDocs[0].Created {
		t.Fatalf("touched doc = %+v, want {DocID:%s Created:true}", res.EntryDocs[0], entry.ID)
	}
}

// 追加进既有词条 → EntryDocs 记既有文档（Created=false）。
func TestWriteBack_EntryDocsTouched_ExistingEntry(t *testing.T) {
	f := newWriteBackEntryFixture()
	existing := Document{
		ID: "doc-entry", CollectionID: "team-1", RelPath: "entries/release-train.md",
		ContentText: writeBackEntryHeader([]string{"发布火车"}) + "## decision\n\n既有决策。\n",
	}
	f.docs[existing.ID] = existing
	f.byRel["team-1|entries/release-train.md"] = existing.ID
	f.candidates = []ResolveDocCandidate{{
		DocID: existing.ID, CollectionID: "team-1", RelPath: existing.RelPath, Title: "发布火车",
	}}

	res, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-t2",
		Facts:     []WriteBackFact{taggedFact("fid-c", "发布窗口固定在周四下午", "decision", 0.9, "发布火车")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.EntryDocs) != 1 || res.EntryDocs[0].DocID != "doc-entry" || res.EntryDocs[0].Created {
		t.Fatalf("existing entry touched = %+v", res.EntryDocs)
	}
}

// 同 fact_id 同陈述重写（内容不变）→ 词条无实际改动，EntryDocs 为空。
func TestWriteBack_EntryDocsTouched_NoChangeSkipped(t *testing.T) {
	f := newWriteBackEntryFixture()
	in := WriteBackInput{
		SessionID: "sess-t3",
		Facts:     []WriteBackFact{taggedFact("fid-d", "值班每 8 小时轮换", "constraint", 0.9, "值班制度")},
	}
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	res, err := f.u.WriteBackSessionFacts(context.Background(), in) // 原样重放
	if err != nil {
		t.Fatal(err)
	}
	if len(res.EntryDocs) != 0 {
		t.Fatalf("no-change rewrite must not touch entries: %+v", res.EntryDocs)
	}
}

// ── 写回 chunk 重放钩子（2026-08-15：knowledge_write 工具直调 biz 层的收口点）──

// 接线后：日记 + 词条页一并进重放集合；未接线不触发（降级安全）。
func TestWriteBack_ReplayHookReceivesDiaryAndEntries(t *testing.T) {
	f := newWriteBackEntryFixture()
	var gotTouched []PromoteTouchedDoc
	var gotCol Collection
	f.u.SetWriteBackReplay(func(_ context.Context, col Collection, touched []PromoteTouchedDoc) error {
		gotCol = col
		gotTouched = touched
		return nil
	})
	res, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-r1",
		Facts:     []WriteBackFact{taggedFact("fid-r", "部署必须走灰度且保留回滚开关", "constraint", 0.95, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotCol.ID != "team-1" {
		t.Fatalf("hook collection = %q", gotCol.ID)
	}
	// touched = 日记（created）+ 词条页（created）。
	if len(gotTouched) != 2 {
		t.Fatalf("touched = %+v, want diary+entry", gotTouched)
	}
	if gotTouched[0].DocID != res.DocID || !gotTouched[0].Created {
		t.Fatalf("first touched must be diary: %+v", gotTouched[0])
	}
	if gotTouched[1].DocID != res.EntryDocs[0].DocID {
		t.Fatalf("second touched must be entry doc: %+v", gotTouched[1])
	}
}

// 全量去重命中（无任何改动）→ 不触发重放。
func TestWriteBack_ReplayHookSkippedWhenNoChange(t *testing.T) {
	f := newWriteBackEntryFixture()
	calls := 0
	f.u.SetWriteBackReplay(func(context.Context, Collection, []PromoteTouchedDoc) error {
		calls++
		return nil
	})
	in := WriteBackInput{
		SessionID: "sess-r2",
		Facts:     []WriteBackFact{taggedFact("fid-r2", "值班每 8 小时轮换", "constraint", 0.9, "值班制度")},
	}
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("first write must replay once, got %d", calls)
	}
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("idempotent replay must skip hook, got %d calls", calls)
	}
}

// ── P0-2 别名成链：basename + title + aliases 多键 ──────────────────────────

func TestAutolinkWikiMentionsMulti_AliasAndTitleKeys(t *testing.T) {
	targets := []AutolinkTarget{{
		Canonical: "release-train",
		Keys:      []string{"release-train", "发布火车", "灰度"},
	}}
	// 别名命中 → 链接到 canonical basename。
	got, n := AutolinkWikiMentionsMulti("周四走灰度发版。", nil, targets)
	if n != 1 || got != "周四走[[release-train]]发版。" {
		t.Fatalf("alias key: got %q n=%d", got, n)
	}
	// title 命中同样落 canonical。
	got, n = AutolinkWikiMentionsMulti("发布火车本周暂停。", nil, targets)
	if n != 1 || got != "[[release-train]]本周暂停。" {
		t.Fatalf("title key: got %q n=%d", got, n)
	}
}

func TestAutolinkWikiMentionsMulti_AmbiguousKeySkipped(t *testing.T) {
	targets := []AutolinkTarget{
		{Canonical: "协议-a", Keys: []string{"协议-a", "通信协议"}},
		{Canonical: "协议-b", Keys: []string{"协议-b", "通信协议"}},
	}
	got, n := AutolinkWikiMentionsMulti("讨论通信协议细节。", nil, targets)
	if n != 0 || got != "讨论通信协议细节。" {
		t.Fatalf("ambiguous key must skip: got %q n=%d", got, n)
	}
}

func TestAutolinkWikiMentionsMulti_SelfKeysExempt(t *testing.T) {
	targets := []AutolinkTarget{{Canonical: "灰度", Keys: []string{"灰度", "发布火车"}}}
	got, n := AutolinkWikiMentionsMulti("灰度 与 发布火车。", []string{"发布火车", "灰度"}, targets)
	if n != 0 {
		t.Fatalf("self keys must not link: got %q n=%d", got, n)
	}
}

func TestMentionNeedles_MultiKey(t *testing.T) {
	m := noOpMockRepo()
	target := Document{ID: "d1", CollectionID: "c1", RelPath: "entries/release-train.md"}
	u := NewUsecaseFromRepo(m)
	// 未接线 resolveIndex：basename 单键（旧行为）。
	got := u.mentionNeedles(context.Background(), target)
	if len(got) != 1 || got[0] != "release-train" {
		t.Fatalf("basename-only: %v", got)
	}
	// 接线后：basename + title + aliases 多键，去重、去单字符。
	idx := newMemBlockIndex(func() []ResolveDocCandidate {
		return []ResolveDocCandidate{{
			DocID: "d1", CollectionID: "c1", RelPath: target.RelPath,
			Title: "发布火车", Aliases: []string{"灰度", "x", "发布火车"},
		}}
	})
	u.SetBlockIndexRepos(idx, idx)
	got = u.mentionNeedles(context.Background(), target)
	want := []string{"release-train", "发布火车", "灰度"}
	if len(got) != len(want) {
		t.Fatalf("needles = %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("needles = %v want %v", got, want)
		}
	}
}
