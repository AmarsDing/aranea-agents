package knowledge

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
)

// ── G-34 ADR-C：MOC 摘要 LLM 对账补齐 ────────────────────────────────────
// curate 轮内 runMOCSummaryTask：扫 moc/ 前缀下「## 摘要」段仍为占位文的页，
// 页内「## 簇成员」wikilink 反查成员文档（页即真相源）→ MOCSummarizer 出摘要 →
// ensureMOCSummarySection H2 幂等替换（人工摘要不覆写）+ 重放。预算 4/库/轮
// 成功后扣，失败 Warn 占位保留下轮重试；DryRun/非团队库/端口未接线一律跳过。

type stubMOCSummarizer struct {
	calls   int
	last    MOCSummaryRequest
	summary string
	err     error
	seq     []error  // 逐调用结果序列（nil 元素=用 summary 成功）；耗尽后回落 err/summary
	onCall  func()   // 生效前钩子（F1/F4 竞态钉：LLM 窗口内人工编辑）
}

func (s *stubMOCSummarizer) SummarizeMOC(_ context.Context, req MOCSummaryRequest) (MOCSummaryResult, error) {
	s.calls++
	s.last = req
	if s.onCall != nil {
		s.onCall()
	}
	if len(s.seq) > 0 {
		e := s.seq[0]
		s.seq = s.seq[1:]
		if e != nil {
			return MOCSummaryResult{}, e
		}
		return MOCSummaryResult{Summary: s.summary}, nil
	}
	if s.err != nil {
		return MOCSummaryResult{}, s.err
	}
	return MOCSummaryResult{Summary: s.summary}, nil
}

func mocSummaryPendingBody(title string) string {
	return buildMOCSkeletonBody(mocSkeletonParams{
		Title: title, HubDocID: "d-hub", HubRelPath: "entries/发布体系.md",
		Degree: 5, Density: 0.67,
		Members: []string{"entries/灰度.md", "entries/回滚.md"},
	})
}

func mocSummaryDocs() *mocDocStore {
	return &mocDocStore{docs: map[string]Document{
		"d-hub": {ID: "d-hub", CollectionID: "c1", RelPath: "entries/发布体系.md",
			ContentText: "发布体系统领灰度/回滚/告警三要素。"},
		"d-gray": {ID: "d-gray", CollectionID: "c1", RelPath: "entries/灰度.md",
			ContentText: "灰度发布按比例逐步放量。"},
		"d-rb": {ID: "d-rb", CollectionID: "c1", RelPath: "entries/回滚.md",
			ContentText: "回滚预案保证发布可逆。"},
	}}
}

// 成员清单 wikilink 反解：保序、去扩展名重建 rel_path、非法行丢弃。
func TestMOCMemberLinks(t *testing.T) {
	t.Parallel()
	sec := "## 簇成员\n\n- [[entries/发布体系]]\n- [[entries/灰度]]\n- 非法行\n- [[entries/回滚]]\n"
	links := mocMemberLinks(sec)
	if len(links) != 3 || links[0] != "entries/发布体系" || links[1] != "entries/灰度" || links[2] != "entries/回滚" {
		t.Fatalf("links = %v", links)
	}
	if got := mocMemberLinks("## 簇成员\n\n"); len(got) != 0 {
		t.Fatalf("empty section must yield none: %v", got)
	}
}

// 摘要段幂等替换契约：占位文可替、人工撰写不覆写、缺段重挂、重放零改动。
func TestEnsureMOCSummarySection(t *testing.T) {
	t.Parallel()
	body := mocSummaryPendingBody("发布体系")
	next, changed := ensureMOCSummarySection(body, "本簇梳理发布全链路。")
	if !changed || !strings.Contains(next, "## 摘要\n\n本簇梳理发布全链路。") {
		t.Fatalf("placeholder must be replaced:\n%s", next)
	}
	if strings.Contains(next, mocSummaryPlaceholder) {
		t.Fatalf("placeholder must be gone:\n%s", next)
	}
	// 统计段未被波及。
	if !strings.Contains(next, "- 簇规模：3 篇") {
		t.Fatalf("stats section clobbered:\n%s", next)
	}
	// 重放同摘要 → 零改动。
	if again, c := ensureMOCSummarySection(next, "本簇梳理发布全链路。"); c || again != next {
		t.Fatal("reapply same summary must be a no-op")
	}
	// 人工另一版摘要 → 巡检不覆写。
	if _, c := ensureMOCSummarySection(next, "LLM 另一措辞。"); c {
		t.Fatal("human-written summary must not be overwritten")
	}
	// 缺摘要段 → 段尾重挂。
	trunc := body
	if idx := strings.Index(trunc, "## 摘要"); idx >= 0 {
		end := strings.Index(trunc[idx:], "## 簇统计")
		trunc = strings.TrimRight(trunc[:idx], "\n") + "\n\n" + trunc[idx+end:]
	}
	again, c := ensureMOCSummarySection(trunc, "本簇梳理发布全链路。")
	if !c || !strings.Contains(again, "## 摘要\n\n本簇梳理发布全链路。") {
		t.Fatalf("missing section must be reattached:\n%s", again)
	}
	// 空摘要不出手。
	if _, c := ensureMOCSummarySection(body, "  "); c {
		t.Fatal("blank summary must be rejected")
	}
}

// 对账输入装帧：hub 居首按 800 runes 截断、成员各 400、总预算 4000；
// 超限打 Truncated 标；成员文档缺失跳过不阻断。
func TestFillMOCSummaryRequestBudgets(t *testing.T) {
	t.Parallel()
	store := mocSummaryDocs()
	store.docs["d-gray"] = Document{ID: "d-gray", CollectionID: "c1", RelPath: "entries/灰度.md",
		ContentText: strings.Repeat("灰", mocSummaryMemberMaxRunes+50)}
	store.docs["d-hub"] = Document{ID: "d-hub", CollectionID: "c1", RelPath: "entries/发布体系.md",
		ContentText: strings.Repeat("发", mocSummaryHubMaxRunes+100)}
	var replayed []PromoteTouchedDoc
	u := newMOCDisposalUsecase(store.repo(), &stubCurateRepo{}, &replayed)
	mocs := &stubMOCSummarizer{summary: "本簇围绕发布全链路。"}
	u.SetMOCSummarizer(mocs)

	pending := MOCPendingSummaryDoc{
		DocID: "d-moc", RelPath: "moc/发布体系.md", Body: mocSummaryPendingBody("发布体系"),
	}
	store.seed(pending)
	if u.fillMOCSummary(context.Background(), Collection{ID: "c1", VaultBackend: VaultBackendTeam}, pending) != mocFillOK {
		t.Fatal("fill must succeed")
	}
	req := mocs.last
	if req.Title != "发布体系" || req.Hub.RelPath != "entries/发布体系.md" ||
		len([]rune(req.Hub.Body)) != mocSummaryHubMaxRunes || !req.Truncated {
		t.Fatalf("hub excerpt = %+v", req.Hub)
	}
	// 成员保页内次序（骨架生成时已排序：回滚 < 灰度）；长成员按 400 runes 截断。
	if len(req.Members) != 2 || req.Members[0].RelPath != "entries/回滚.md" ||
		req.Members[1].RelPath != "entries/灰度.md" ||
		len([]rune(req.Members[1].Body)) != mocSummaryMemberMaxRunes ||
		len([]rune(req.Members[0].Body)) > mocSummaryMemberMaxRunes {
		t.Fatalf("members = %+v", req.Members)
	}
	total := len([]rune(req.Hub.Body))
	for _, m := range req.Members {
		total += len([]rune(m.Body))
	}
	if total > mocSummaryTotalMaxRunes {
		t.Fatalf("total budget exceeded: %d", total)
	}
}

// 全流程：占位页 → LLM 摘要落页 + replay + 报告计数；二次轮零剩占位即止。
func TestRunMOCSummaryTask_FillsPlaceholder(t *testing.T) {
	t.Parallel()
	pending := MOCPendingSummaryDoc{DocID: "d-moc", RelPath: "moc/发布体系.md", Body: mocSummaryPendingBody("发布体系")}
	curate := &stubCurateRepo{mocPending: []MOCPendingSummaryDoc{pending}}
	store := mocSummaryDocs()
	store.seed(pending)
	var replayed []PromoteTouchedDoc
	u := newMOCDisposalUsecase(store.repo(), curate, &replayed)
	mocs := &stubMOCSummarizer{summary: "本簇围绕发布全链路，灰度与回滚互补。"}
	u.SetMOCSummarizer(mocs)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.MOCSummarized != 1 {
		t.Fatalf("rep = %+v", rep)
	}
	if mocs.calls != 1 {
		t.Fatalf("summarizer calls = %d", mocs.calls)
	}
	body := store.docs["d-moc"].ContentText
	if !strings.Contains(body, "## 摘要\n\n本簇围绕发布全链路，灰度与回滚互补。") ||
		strings.Contains(body, mocSummaryPlaceholder) {
		t.Fatalf("summary not landed:\n%s", body)
	}
	if len(replayed) != 1 || replayed[0].DocID != "d-moc" {
		t.Fatalf("replay = %+v", replayed)
	}
	found := false
	for _, a := range rep.Actions {
		if a == "moc_summary" {
			found = true
		}
	}
	if !found {
		t.Fatalf("actions = %v", rep.Actions)
	}
}

// 预算闸：6 页占位每轮只补 4（成功后扣），其余下轮承接；失败不占预算。
func TestRunMOCSummaryTask_BudgetAndFailure(t *testing.T) {
	t.Parallel()
	var pending []MOCPendingSummaryDoc
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		pending = append(pending, MOCPendingSummaryDoc{
			DocID: "d-moc-" + name, RelPath: "moc/" + name + ".md", Body: mocSummaryPendingBody(name),
		})
	}
	curate := &stubCurateRepo{mocPending: pending}
	store := mocSummaryDocs()
	for _, p := range pending {
		store.seed(p)
	}
	u := newMOCDisposalUsecase(store.repo(), curate, &[]PromoteTouchedDoc{})
	mocs := &stubMOCSummarizer{summary: "x"}
	u.SetMOCSummarizer(mocs)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.MOCSummarized != mocSummaryBudgetPerRound || mocs.calls != mocSummaryBudgetPerRound {
		t.Fatalf("budget gate broken: rep=%d calls=%d", rep.MOCSummarized, mocs.calls)
	}

	// LLM 失败：占位保留可重试，不计入补全数。
	store2 := mocSummaryDocs()
	store2.seed(pending[:1][0])
	curate2 := &stubCurateRepo{mocPending: pending[:1]}
	u2 := newMOCDisposalUsecase(store2.repo(), curate2, &[]PromoteTouchedDoc{})
	u2.SetMOCSummarizer(&stubMOCSummarizer{err: apierror.Internal(apierror.DomainKnowledge, "llm down")})
	rep2, err := u2.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep2.MOCSummarized != 0 || len(store2.updated) != 0 {
		t.Fatalf("failure must keep placeholder for retry: rep=%d updated=%v", rep2.MOCSummarized, store2.updated)
	}
	// 空摘要等价失败。
	u2.SetMOCSummarizer(&stubMOCSummarizer{summary: " "})
	if rep2, err = u2.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"}); err != nil {
		t.Fatal(err)
	}
	if rep2.MOCSummarized != 0 {
		t.Fatalf("blank summary must not count: %d", rep2.MOCSummarized)
	}
}

// 跳过矩阵：端口未接线 / DryRun / 非团队库一律空转（零 LLM 零存储写）。
func TestRunMOCSummaryTask_SkipMatrix(t *testing.T) {
	t.Parallel()
	pending := []MOCPendingSummaryDoc{{DocID: "d-moc", RelPath: "moc/x.md", Body: mocSummaryPendingBody("x")}}

	// 端口未接线：跳过且不发扫描。
	curate := &stubCurateRepo{}
	u := newMOCDisposalUsecase(mocSummaryDocs().repo(), curate, &[]PromoteTouchedDoc{})
	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil || rep.MOCSummarized != 0 {
		t.Fatalf("nil port must skip: %v %+v", err, rep)
	}
	if curate.mocScanCalls != 0 {
		t.Fatalf("scan must not run without port: %d", curate.mocScanCalls)
	}

	// DryRun：端口在也不写。
	curate2 := &stubCurateRepo{mocPending: pending}
	u2 := newMOCDisposalUsecase(mocSummaryDocs().repo(), curate2, &[]PromoteTouchedDoc{})
	mocs2 := &stubMOCSummarizer{summary: "x"}
	u2.SetMOCSummarizer(mocs2)
	rep2, err := u2.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1", DryRun: true})
	if err != nil || rep2.MOCSummarized != 0 || mocs2.calls != 0 {
		t.Fatalf("dry-run must skip llm: %v %+v calls=%d", err, rep2, mocs2.calls)
	}

	// 非团队库：显式拒绝（ADR-A 防御深——本地 vault 永不巡检补摘要）。
	repo := &mockRepo{
		collGetFn: func(_ context.Context, id string) (Collection, error) {
			return Collection{ID: id, VaultBackend: VaultBackendLocal}, nil
		},
		docGetByRelFn: func(_ context.Context, _, _ string) (Document, error) {
			return Document{}, apierror.NotFound(apierror.DomainKnowledge, "x")
		},
		docCreateFn:  func(_ context.Context, d Document) (Document, error) { return d, nil },
		docContentFn: func(_ context.Context, _, _ string, _ bool) error { return nil },
	}
	curate3 := &stubCurateRepo{mocPending: pending}
	u3 := newMOCDisposalUsecase(repo, curate3, &[]PromoteTouchedDoc{})
	mocs3 := &stubMOCSummarizer{summary: "x"}
	u3.SetMOCSummarizer(mocs3)
	rep3, err := u3.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil || rep3.MOCSummarized != 0 || mocs3.calls != 0 || curate3.mocScanCalls != 0 {
		t.Fatalf("non-team gate broken: %v %+v calls=%d scans=%d", err, rep3, mocs3.calls, curate3.mocScanCalls)
	}
}

// 竞态：扫描快照带占位、落笔前人工已写摘要 → 不调 LLM 直接跳过。
func TestRunMOCSummaryTask_HumanSummaryRaceSkipped(t *testing.T) {
	t.Parallel()
	human := strings.Replace(mocSummaryPendingBody("x"), mocSummaryPlaceholder, "人工已定稿。", 1)
	curate := &stubCurateRepo{mocPending: []MOCPendingSummaryDoc{{DocID: "d-moc", RelPath: "moc/x.md", Body: human}}}
	store := mocSummaryDocs()
	store.seed(MOCPendingSummaryDoc{DocID: "d-moc", RelPath: "moc/x.md", Body: human})
	u := newMOCDisposalUsecase(store.repo(), curate, &[]PromoteTouchedDoc{})
	mocs := &stubMOCSummarizer{summary: "llm"}
	u.SetMOCSummarizer(mocs)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.MOCSummarized != 0 || mocs.calls != 0 || len(store.updated) != 0 {
		t.Fatalf("human race must be skipped pre-llm: rep=%d calls=%d updated=%v", rep.MOCSummarized, mocs.calls, store.updated)
	}
}

// 终审 F1/F4：LLM 调用窗口内人工已定稿（扫描→落笔间隔 60s）→ 落笔前重读
// 新鲜正文重跑占位门让位人工：零更新、零计数；人工正文原样保留。
func TestRunMOCSummaryTask_HumanEditAfterLLMReturn(t *testing.T) {
	t.Parallel()
	pending := MOCPendingSummaryDoc{DocID: "d-moc", RelPath: "moc/发布体系.md", Body: mocSummaryPendingBody("发布体系")}
	curate := &stubCurateRepo{mocPending: []MOCPendingSummaryDoc{pending}}
	store := mocSummaryDocs()
	store.seed(pending)
	u := newMOCDisposalUsecase(store.repo(), curate, &[]PromoteTouchedDoc{})
	mocs := &stubMOCSummarizer{summary: "llm 版本将被弃", onCall: func() {
		store.docs["d-moc"] = Document{ID: "d-moc", CollectionID: "c1", RelPath: "moc/发布体系.md",
			ContentText: strings.Replace(pending.Body, mocSummaryPlaceholder, "人工已抢答。", 1)}
	}}
	u.SetMOCSummarizer(mocs)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.MOCSummarized != 0 || mocs.calls != 1 || len(store.updated) != 0 {
		t.Fatalf("post-llm race must yield to human: rep=%d calls=%d updated=%v", rep.MOCSummarized, mocs.calls, store.updated)
	}
	if body := store.docs["d-moc"].ContentText; !strings.Contains(body, "人工已抢答。") {
		t.Fatalf("human edit must survive:\n%s", body)
	}
}

// 终审 F5：结构性不可逆失败（缺簇成员段 / wikilink 空 / hub 不存在）——
// 占位行内改写为「暂停」标记退出待补集（SQL 过滤不再命中，防扫描槽饥饿），
// 零 LLM、仅一次标记写；人工作主不自动恢复。
func TestRunMOCSummaryTask_StructuralFailInlineBlocked(t *testing.T) {
	t.Parallel()
	noMemberSection := strings.Replace(mocSummaryPendingBody("x"), "## 簇成员\n\n- [[entries/发布体系]]\n- [[entries/回滚]]\n- [[entries/灰度]]\n", "", 1)
	emptyWikilinks := strings.Replace(mocSummaryPendingBody("x"), "- [[", "- x[[", 3)
	if !strings.Contains(noMemberSection, mocSummaryPlaceholder) || strings.Contains(noMemberSection, "## 簇成员") {
		t.Fatal("member-section surgery broken")
	}
	cases := []struct {
		name     string
		body     string
		hubGone  bool
	}{
		{"member section missing", noMemberSection, false},
		{"empty wikilinks", emptyWikilinks, false},
		{"hub deleted", mocSummaryPendingBody("x"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pending := MOCPendingSummaryDoc{DocID: "d-moc", RelPath: "moc/x.md", Body: tc.body}
			store := mocSummaryDocs()
			if tc.hubGone {
				delete(store.docs, "d-hub")
			}
			store.seed(pending)
			curate := &stubCurateRepo{mocPending: []MOCPendingSummaryDoc{pending}}
			u := newMOCDisposalUsecase(store.repo(), curate, &[]PromoteTouchedDoc{})
			mocs := &stubMOCSummarizer{summary: "不应被调用"}
			u.SetMOCSummarizer(mocs)

			rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
			if err != nil {
				t.Fatal(err)
			}
			if rep.MOCSummarized != 0 || mocs.calls != 0 {
				t.Fatalf("structural fail must not burn llm/budget: rep=%d calls=%d", rep.MOCSummarized, mocs.calls)
			}
			if len(store.updated) != 1 {
				t.Fatalf("blocked marker must be the single write: %v", store.updated)
			}
			body := store.docs["d-moc"].ContentText
			if !strings.Contains(body, mocSummaryBlockedPrefix) || strings.Contains(body, mocSummaryPlaceholder) {
				t.Fatalf("pending filter must no longer match:\n%s", body)
			}
		})
	}
}

// 终审 F6：成员读取——文档缺失（NotFound）跳过该成员不阻断本页；瞬时错误
// 整页留下轮（零写零计数零 LLM），不得产残缺摘要消耗占位。
func TestRunMOCSummaryTask_MemberReadErrors(t *testing.T) {
	t.Parallel()
	pending := MOCPendingSummaryDoc{DocID: "d-moc", RelPath: "moc/x.md", Body: mocSummaryPendingBody("x")}

	// NotFound：灰度已删 → 跳过灰度仍补齐（成员只存 回滚）。
	store := mocSummaryDocs()
	delete(store.docs, "d-gray")
	store.seed(pending)
	curate := &stubCurateRepo{mocPending: []MOCPendingSummaryDoc{pending}}
	u := newMOCDisposalUsecase(store.repo(), curate, &[]PromoteTouchedDoc{})
	mocs := &stubMOCSummarizer{summary: "本簇只讲回滚与发布体系。"}
	u.SetMOCSummarizer(mocs)
	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.MOCSummarized != 1 || mocs.calls != 1 {
		t.Fatalf("missing member must not block page: rep=%d calls=%d", rep.MOCSummarized, mocs.calls)
	}
	if len(mocs.last.Members) != 1 || mocs.last.Members[0].RelPath != "entries/回滚.md" {
		t.Fatalf("gray must be skipped: %+v", mocs.last.Members)
	}

	// 瞬时错误：整页留下轮——零写、零计数、零 LLM。
	store2 := mocSummaryDocs()
	store2.seed(pending)
	base := store2.repo()
	flapping := &mockRepo{
		collGetFn: base.collGetFn,
		docGetByRelFn: func(ctx context.Context, cid, rel string) (Document, error) {
			if rel == "entries/灰度.md" {
				return Document{}, apierror.Internal(apierror.DomainKnowledge, "db flap")
			}
			return base.docGetByRelFn(ctx, cid, rel)
		},
		docCreateFn:  base.docCreateFn,
		docContentFn: base.docContentFn,
	}
	curate2 := &stubCurateRepo{mocPending: []MOCPendingSummaryDoc{pending}}
	u2 := newMOCDisposalUsecase(flapping, curate2, &[]PromoteTouchedDoc{})
	mocs2 := &stubMOCSummarizer{summary: "不应被调用"}
	u2.SetMOCSummarizer(mocs2)
	rep2, err := u2.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep2.MOCSummarized != 0 || mocs2.calls != 0 || len(store2.updated) != 0 {
		t.Fatalf("transient member error must hold page: rep=%d calls=%d updated=%v", rep2.MOCSummarized, mocs2.calls, store2.updated)
	}
}

// 终审 F7：LLM 连续失败熔断——单库本轮最多试 mocSummaryMaxConsecutiveLLMFails
// 发即跳出（防故障期 32 页 × 60s 串行停滞）；成功重置连续计数。
func TestRunMOCSummaryTask_LLMCircuitBreaker(t *testing.T) {
	t.Parallel()
	mkPending := func(names ...string) ([]MOCPendingSummaryDoc, *mocDocStore) {
		var pending []MOCPendingSummaryDoc
		store := mocSummaryDocs()
		for _, n := range names {
			p := MOCPendingSummaryDoc{DocID: "d-moc-" + n, RelPath: "moc/" + n + ".md", Body: mocSummaryPendingBody(n)}
			pending = append(pending, p)
			store.seed(p)
		}
		return pending, store
	}

	// 全败流：6 页占位只试 3 发。
	pending, store := mkPending("a", "b", "c", "d", "e", "f")
	curate := &stubCurateRepo{mocPending: pending}
	u := newMOCDisposalUsecase(store.repo(), curate, &[]PromoteTouchedDoc{})
	mocs := &stubMOCSummarizer{err: apierror.Internal(apierror.DomainKnowledge, "llm down")}
	u.SetMOCSummarizer(mocs)
	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.MOCSummarized != 0 || mocs.calls != mocSummaryMaxConsecutiveLLMFails {
		t.Fatalf("breaker must trip at %d consecutive fails: rep=%d calls=%d", mocSummaryMaxConsecutiveLLMFails, rep.MOCSummarized, mocs.calls)
	}

	// 成败交错：成功重置连续计数，6 页全部走完（3 成功）。
	pending2, store2 := mkPending("g", "h", "i", "j", "k", "l")
	curate2 := &stubCurateRepo{mocPending: pending2}
	u2 := newMOCDisposalUsecase(store2.repo(), curate2, &[]PromoteTouchedDoc{})
	down := apierror.Internal(apierror.DomainKnowledge, "flap")
	mocs2 := &stubMOCSummarizer{summary: "x", seq: []error{down, nil, down, nil, down, nil}}
	u2.SetMOCSummarizer(mocs2)
	rep2, err := u2.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep2.MOCSummarized != 3 || mocs2.calls != 6 {
		t.Fatalf("alternating must reset breaker: rep=%d calls=%d", rep2.MOCSummarized, mocs2.calls)
	}
}

// 终审 F2：总预算耗尽整段丢弃成员也置 Truncated——单节选零 rune 截断时
// LLM 仍须被告知簇不完整（成员被预算丢弃）。
func TestFillMOCSummary_BudgetExhaustMarksTruncated(t *testing.T) {
	t.Parallel()
	const memberCount = 11
	names := make([]string, 0, memberCount)
	store := mocSummaryDocs()
	for i := 0; i < memberCount; i++ {
		rel := "entries/m" + string(rune('a'+i)) + ".md"
		names = append(names, rel)
		store.docs["d-"+rel] = Document{ID: "d-" + rel, CollectionID: "c1", RelPath: rel,
			ContentText: strings.Repeat("成", mocSummaryMemberMaxRunes)} // 恰好满额：零节选截断
	}
	// hub 空正文：不占总预算（0 runes），排除 hub 截断干扰。
	store.docs["d-hub"] = Document{ID: "d-hub", CollectionID: "c1", RelPath: "entries/发布体系.md", ContentText: ""}
	body := buildMOCSkeletonBody(mocSkeletonParams{
		Title: "集群", HubDocID: "d-hub", HubRelPath: "entries/发布体系.md",
		Degree: 11, Density: 0.5, Members: names,
	})
	pending := MOCPendingSummaryDoc{DocID: "d-moc", RelPath: "moc/x.md", Body: body}
	store.seed(pending)
	u := newMOCDisposalUsecase(store.repo(), &stubCurateRepo{}, &[]PromoteTouchedDoc{})
	mocs := &stubMOCSummarizer{summary: "x"}
	u.SetMOCSummarizer(mocs)

	outcome := u.fillMOCSummary(context.Background(), Collection{ID: "c1", VaultBackend: VaultBackendTeam}, pending)
	if outcome != mocFillOK {
		t.Fatalf("outcome = %v", outcome)
	}
	if !mocs.last.Truncated {
		t.Fatal("budget-dropped member must set Truncated")
	}
	if len(mocs.last.Members) != memberCount-1 {
		t.Fatalf("exactly %d members must fit 4000-rune budget after 0-rune hub: got %d", memberCount-1, len(mocs.last.Members))
	}
}
