package knowledge

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
)

// ── G-34 MOC 自动摘要词条（ADR-KN-20261021-A/B/C）─────────────────────────
// A：applied 时同步落 moc/<slug>.md 骨架页（零 LLM，DocType=moc/pending/Organized），
//    rel_path 冲突 → slug+确定性短后缀；文档层 get-or-create 为最终幂等担保。
// B：dedup_key 带成员签名（簇结构变化可重提，解除三态死信）。
// C：摘要占位段本轮只落，LLM 对账补齐属 ADR-C（本文件覆盖其侦测锚常量）。

func mocTestParams() mocSkeletonParams {
	return mocSkeletonParams{
		Title:      "发布体系",
		HubDocID:   "d-hub",
		HubRelPath: "entries/发布体系.md",
		Degree:     5,
		Density:    0.667,
		Members:    []string{"entries/回滚.md", "entries/灰度.md", "entries/告警.md"},
	}
}

// ADR-B：成员签名序无关 + 成员集合变化必变签（dedup 三态禁言的死信解除前提）。
func TestMOCMemberSignature(t *testing.T) {
	t.Parallel()
	members := []string{"entries/b.md", "entries/a.md", "entries/c.md"}
	reversed := []string{"entries/c.md", "entries/a.md", "entries/b.md"}
	duplicated := []string{"entries/a.md", "entries/a.md", "entries/b.md", "entries/c.md"}
	sig := mocMemberSignature(members)
	if sig == "" || len(sig) != mocMemberSigLen {
		t.Fatalf("sig = %q, want %d hex chars", sig, mocMemberSigLen)
	}
	if sig != mocMemberSignature(reversed) || sig != mocMemberSignature(duplicated) {
		t.Fatal("signature must be order/duplicate independent")
	}
	if sig == mocMemberSignature([]string{"entries/a.md", "entries/b.md", "entries/d.md"}) {
		t.Fatal("member-set change must change signature")
	}
	// 输入切片不被排序副作用改写（调用方持有的 payload 复用）。
	if members[0] != "entries/b.md" {
		t.Fatalf("input mutated: %v", members)
	}
}

// 骨架页形态：frontmatter（title+type:moc）、hub 标记、摘要占位、统计段、成员清单。
func TestBuildMOCSkeletonBody(t *testing.T) {
	t.Parallel()
	p := mocTestParams()
	body := buildMOCSkeletonBody(p)

	for _, want := range []string{
		"title: 发布体系", // yamlQuote：CJK 属安全字符不包裹（词条页同式）
		"type: moc",
		"<!-- moc:hub d-hub -->",
		"## 摘要\n\n" + mocSummaryPlaceholder,
		"- 簇规模：4 篇",
		"- Hub 度数：5",
		"- 边密度：0.67",
		"## 簇成员",
		"- [[entries/发布体系]]", // hub 恒为成员清单第一项
		"- [[entries/告警]]",
		"- [[entries/回滚]]",
		"- [[entries/灰度]]",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("skeleton missing %q in:\n%s", want, body)
		}
	}
	// 成员清单确定性排序：UTF-8 字节序 告警 < 回滚 < 灰度，hub 之外按序钉死。
	ai, ri, gi := strings.Index(body, entriesMemberLink("entries/告警.md")),
		strings.Index(body, entriesMemberLink("entries/回滚.md")),
		strings.Index(body, entriesMemberLink("entries/灰度.md"))
	if !(ai >= 0 && ai < ri && ri < gi) {
		t.Fatalf("member order not sorted (告警<回滚<灰度): a=%d r=%d g=%d\n%s", ai, ri, gi, body)
	}
	if extractMOCHubMarker(body) != "d-hub" {
		t.Fatalf("hub marker roundtrip failed")
	}

	// 空成员：簇规模=1，成员清单仅 hub。
	solo := buildMOCSkeletonBody(mocSkeletonParams{
		Title: "孤岛", HubDocID: "d-solo", HubRelPath: "entries/孤岛.md", Degree: 3, Density: 1,
	})
	if !strings.Contains(solo, "- 簇规模：1 篇") || !strings.Contains(solo, "- 边密度：1.00") {
		t.Fatalf("solo cluster skeleton:\n%s", solo)
	}
	if strings.Count(solo, "- [[") != 1 {
		t.Fatalf("solo skeleton must have exactly one member link:\n%s", solo)
	}
}

// 幂等重放：同参再次 ensure 零改动（applied 重放不产生无关 diff，检索不重索引）。
func TestEnsureMOCSkeletonBody_Idempotent(t *testing.T) {
	t.Parallel()
	p := mocTestParams()
	body := buildMOCSkeletonBody(p)
	next, changed := ensureMOCSkeletonBody(body, p)
	if changed || next != body {
		t.Fatalf("reapply must be a no-op, changed=%v", changed)
	}
	// 空 body → 等价全新 build（文档曾建但正文被人工清空的角落形态）。
	built, changed := ensureMOCSkeletonBody("", p)
	if !changed || built != body {
		t.Fatal("empty body must rebuild full skeleton")
	}
}

// 簇演进二次 applied：受管段（统计/成员）整体替换，「摘要」人工撰写内容与页尾人工散文原样保留。
func TestEnsureMOCSkeletonBody_UpdatePreservesHuman(t *testing.T) {
	t.Parallel()
	p := mocTestParams()
	body := buildMOCSkeletonBody(p)
	// 模拟治理巡检/人工把摘要占位替换为真摘要，并在页尾追加人工笔记。
	body = strings.Replace(body, mocSummaryPlaceholder, "本簇梳理发布全链路。", 1) + "\n## 人工笔记\n\n发布窗口避开周五。\n"

	p2 := p
	p2.Degree, p2.Density = 7, 0.8
	p2.Members = append(append([]string(nil), p.Members...), "entries/金丝雀.md")
	next, changed := ensureMOCSkeletonBody(body, p2)
	if !changed {
		t.Fatal("cluster evolution must update page")
	}
	for _, want := range []string{
		"- 簇规模：5 篇", "- Hub 度数：7", "- 边密度：0.80",
		"- [[entries/金丝雀]]",
		"本簇梳理发布全链路。",           // 摘要段不被拓扑更新触碰（ADR-C 专属领地）
		"## 人工笔记\n\n发布窗口避开周五。", // 人工追加段原样保留
	} {
		if !strings.Contains(next, want) {
			t.Fatalf("updated skeleton missing %q in:\n%s", want, next)
		}
	}
	if strings.Contains(next, "- 簇规模：4 篇") {
		t.Fatalf("stale stats must be replaced:\n%s", next)
	}
	// 二次 ensure 归零。
	if again, c := ensureMOCSkeletonBody(next, p2); c || again != next {
		t.Fatal("second apply must be idempotent")
	}
}

// 人工删掉受管段：ensure 重挂段尾（自动维护契约），不丢摘要。
func TestEnsureMOCSkeletonBody_ReattachSections(t *testing.T) {
	t.Parallel()
	p := mocTestParams()
	body := buildMOCSkeletonBody(p)
	if idx := strings.Index(body, "## 簇成员"); idx < 0 {
		t.Fatal("fixture: member section must exist")
	} else {
		body = strings.TrimRight(body[:idx], "\n") // 截掉成员段模拟人工删除
	}
	next, changed := ensureMOCSkeletonBody(body, p)
	if !changed || !strings.Contains(next, "## 簇成员") || !strings.Contains(next, "- [[entries/告警]]") {
		t.Fatalf("dropped managed section must be reattached:\n%s", next)
	}
	if !strings.Contains(next, mocSummaryPlaceholder) {
		t.Fatalf("summary placeholder must survive:\n%s", next)
	}
}

// payload 反序列化现实：members 经 JSON 落库后回来时 []any；degree/density 为 float64。
func TestMOCParamsFromPayload(t *testing.T) {
	t.Parallel()
	p, ok := mocParamsFromPayload(map[string]any{
		"hub_doc_id":      "d-hub",
		"hub_rel_path":    "entries/发布体系.md",
		"degree":          float64(5),
		"density":         float64(0.5),
		"suggested_title": "发布体系",
		"members":         []any{"entries/a.md", "entries/b.md"},
	})
	if !ok {
		t.Fatal("valid payload rejected")
	}
	if p.HubDocID != "d-hub" || p.Title != "发布体系" || p.Degree != 5 || p.Density != 0.5 ||
		len(p.Members) != 2 || p.Members[1] != "entries/b.md" {
		t.Fatalf("params = %+v", p)
	}
	// []string 直传（进程内构造）同样接受。
	if _, ok := mocParamsFromPayload(map[string]any{
		"hub_rel_path": "entries/x.md", "suggested_title": "x",
		"members": []string{"entries/a.md"},
	}); !ok {
		t.Fatal("[]string members must be accepted")
	}
	// 历史提案缺关键字段降级 no-op。
	for _, bad := range []map[string]any{
		{},
		{"hub_rel_path": "entries/x.md"}, // 缺 title
		{"suggested_title": "x"},         // 缺 hub_rel_path
		{"hub_rel_path": " ", "suggested_title": "x"}, // 空白 hub_rel_path
	} {
		if _, ok := mocParamsFromPayload(bad); ok {
			t.Fatalf("legacy/broken payload must not be ok: %+v", bad)
		}
	}
}

// ── disposal（applyGovernanceDisposal moc case）───────────────────────────

func mocDisposalView() GovernanceProposalView {
	return GovernanceProposalView{
		ID: 77, CollectionID: "c1", Kind: ProposalKindMOCEmerge, Status: ProposalStatusPending,
		Payload: map[string]any{
			"dedup_key":       "moc:d-hub:0000000000",
			"hub_doc_id":      "d-hub",
			"hub_rel_path":    "entries/发布体系.md",
			"degree":          float64(5),
			"density":         float64(0.667),
			"suggested_title": "发布体系",
			"suggested_path":  "moc/发布体系.md",
			"members":         []any{"entries/灰度.md", "entries/回滚.md", "entries/告警.md"},
		},
	}
}

// mockRepo 组装：collections 恒团队库；documents 走内存表（rel_path 检索 + 建/更记录）。
type mocDocStore struct {
	docs    map[string]Document // id → doc
	created []Document
	updated []string
	nextID  int
}

// seed 把待补页落进内存文档表——fillMOCSummary 双重新鲜读（F1/F4 终审钉）
// 需同快照可读；不写会影响「页被删 → 让位」判别（NotFound ≠ 占位门）。
func (s *mocDocStore) seed(p MOCPendingSummaryDoc) {
	s.docs[p.DocID] = Document{ID: p.DocID, CollectionID: "c1", RelPath: p.RelPath, ContentText: p.Body}
}

func (s *mocDocStore) repo() *mockRepo {
	if s.docs == nil {
		s.docs = map[string]Document{}
	}
	return &mockRepo{
		collGetFn: func(_ context.Context, id string) (Collection, error) {
			return Collection{ID: id, Workspace: "w1", VaultBackend: VaultBackendTeam}, nil
		},
		docGetFn: func(_ context.Context, id string) (Document, error) {
			if doc, ok := s.docs[id]; ok {
				return doc, nil
			}
			return Document{}, apierror.NotFound(apierror.DomainKnowledge, "document %q", id)
		},
		docGetByRelFn: func(_ context.Context, _, relPath string) (Document, error) {
			for _, d := range s.docs {
				if d.RelPath == relPath {
					return d, nil
				}
			}
			return Document{}, apierror.NotFound(apierror.DomainKnowledge, "document %q", relPath)
		},
		docCreateFn: func(_ context.Context, d Document) (Document, error) {
			s.nextID++
			d.ID = strings.TrimSpace(d.ID)
			if d.ID == "" {
				d.ID = "moc-doc-" + string(rune('0'+s.nextID))
			}
			s.docs[d.ID] = d
			s.created = append(s.created, d)
			return d, nil
		},
		docContentFn: func(_ context.Context, id, contentText string, _ bool) error {
			d := s.docs[id]
			d.ContentText = contentText
			s.docs[id] = d
			s.updated = append(s.updated, id)
			return nil
		},
	}
}

func newMOCDisposalUsecase(repo *mockRepo, curate *stubCurateRepo, replayed *[]PromoteTouchedDoc) *Usecase {
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)
	u.SetWriteBackReplay(func(_ context.Context, col Collection, touched []PromoteTouchedDoc) error {
		*replayed = append(append([]PromoteTouchedDoc(nil), *replayed...), touched...)
		return nil
	})
	return u
}

// 建页路径：CreateDocument 行形态 + DocType/Organized/pending + replay 钩子被调。
func TestApplyMOCDisposal_CreatesSkeletonPage(t *testing.T) {
	t.Parallel()
	store := &mocDocStore{}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{mocDisposalView()}}
	var replayed []PromoteTouchedDoc
	u := newMOCDisposalUsecase(store.repo(), curate, &replayed)

	if err := u.ResolveGovernanceProposal(context.Background(), 77, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 {
		t.Fatalf("created = %+v", store.created)
	}
	page := store.created[0]
	if page.RelPath != "moc/发布体系.md" || page.Source != "moc/发布体系.md" {
		t.Fatalf("page path = %+v", page)
	}
	if page.DocType != docTypeMOC || page.MimeType != "text/markdown" ||
		!page.Organized || page.Status != "pending" {
		t.Fatalf("page shape = %+v", page)
	}
	body := page.ContentText
	if extractMOCHubMarker(body) != "d-hub" ||
		!strings.Contains(body, "## 摘要\n\n"+mocSummaryPlaceholder) ||
		!strings.Contains(body, "- [[entries/发布体系]]") {
		t.Fatalf("page body =\n%s", body)
	}
	if len(replayed) != 1 || replayed[0].DocID != page.ID || !replayed[0].Created {
		t.Fatalf("replay = %+v", replayed)
	}
	if curate.resolvedID != 77 || curate.resolvedStatus != ProposalStatusApplied {
		t.Fatalf("proposal resolution = %d/%s", curate.resolvedID, curate.resolvedStatus)
	}
}

// 既有本簇 MOC 页：Update 路径 + replay Created=false；二次 applied 幂等零写。
func TestApplyMOCDisposal_UpdatesExistingPage(t *testing.T) {
	t.Parallel()
	old := mocTestParams()
	old.Density = 0.5
	existing := Document{
		ID: "d-moc", CollectionID: "c1", RelPath: "moc/发布体系.md", Source: "moc/发布体系.md",
		MimeType: "text/markdown", DocType: docTypeMOC, Status: "ready", Organized: true,
		ContentText: buildMOCSkeletonBody(old),
	}
	store := &mocDocStore{docs: map[string]Document{"d-moc": existing}}
	view := mocDisposalView()
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{view}}
	var replayed []PromoteTouchedDoc
	u := newMOCDisposalUsecase(store.repo(), curate, &replayed)

	if err := u.ResolveGovernanceProposal(context.Background(), 77, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 0 || len(store.updated) != 1 || store.updated[0] != "d-moc" {
		t.Fatalf("created=%+v updated=%+v", store.created, store.updated)
	}
	body := store.docs["d-moc"].ContentText
	if !strings.Contains(body, "- 边密度：0.67") {
		t.Fatalf("stats not refreshed:\n%s", body)
	}
	if len(replayed) != 1 || replayed[0].Created {
		t.Fatalf("update replay = %+v", replayed)
	}

	// 幂等：提案假设能再次处置（直接回拨 disposal），正文同参零写零重放。
	replayed = nil
	if err := u.applyGovernanceDisposal(context.Background(), view, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if len(store.updated) != 1 || len(store.created) != 0 || len(replayed) != 0 {
		t.Fatalf("idempotent reapply must not write: created=%+v updated=%+v replayed=%+v",
			store.created, store.updated, replayed)
	}
}

// rel_path 冲突：人工词条页先占位 → slug+确定性短后缀另起；人工页字节不动。
func TestApplyMOCDisposal_RelPathConflictSuffixed(t *testing.T) {
	t.Parallel()
	human := Document{
		ID: "d-human", CollectionID: "c1", RelPath: "moc/发布体系.md", ContentText: "人工笔记",
	}
	store := &mocDocStore{docs: map[string]Document{"d-human": human}}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{mocDisposalView()}}
	var replayed []PromoteTouchedDoc
	u := newMOCDisposalUsecase(store.repo(), curate, &replayed)

	if err := u.ResolveGovernanceProposal(context.Background(), 77, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 {
		t.Fatalf("created = %+v", store.created)
	}
	page := store.created[0]
	if !strings.HasPrefix(page.RelPath, "moc/发布体系-") || !strings.HasSuffix(page.RelPath, ".md") ||
		page.RelPath == "moc/发布体系.md" {
		t.Fatalf("conflict suffixed rel = %q", page.RelPath)
	}
	if page.RelPath != mocConflictRelPath("发布体系", "d-hub") {
		t.Fatalf("suffix must be deterministic: got %q want %q", page.RelPath, mocConflictRelPath("发布体系", "d-hub"))
	}
	if store.docs["d-human"].ContentText != "人工笔记" {
		t.Fatal("human page must stay untouched")
	}
	// 冲突页的二次 applied：命中同一张后缀页走 Update 而非再造三页。
	if err := u.applyGovernanceDisposal(context.Background(), mocDisposalView(), ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 {
		t.Fatalf("suffixed page must be re-found, created=%+v", store.created)
	}
}

// 他簇 MOC 同标题占位（hub marker 不同）→ 同样后缀化，不覆写他簇页。
func TestApplyMOCDisposal_OtherClusterPageNotClaimed(t *testing.T) {
	t.Parallel()
	other := buildMOCSkeletonBody(mocSkeletonParams{
		Title: "发布体系", HubDocID: "d-other", HubRelPath: "entries/另一中心.md", Degree: 3,
	})
	foreign := Document{
		ID: "d-other-moc", CollectionID: "c1", RelPath: "moc/发布体系.md",
		DocType: docTypeMOC, ContentText: other,
	}
	store := &mocDocStore{docs: map[string]Document{"d-other-moc": foreign}}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{mocDisposalView()}}
	u := newMOCDisposalUsecase(store.repo(), curate, &[]PromoteTouchedDoc{})

	if err := u.ResolveGovernanceProposal(context.Background(), 77, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 || store.created[0].RelPath == "moc/发布体系.md" {
		t.Fatalf("other cluster page must not be claimed: %+v", store.created)
	}
	if extractMOCHubMarker(store.docs["d-other-moc"].ContentText) != "d-other" {
		t.Fatal("other cluster page mutated")
	}
}

// 非 team 库防御闸：本地 vault 拒绝 disposal（提案停留 pending 显式错误，禁止伪 applied）。
func TestApplyMOCDisposal_NonTeamVaultRejected(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		collGetFn: func(_ context.Context, id string) (Collection, error) {
			return Collection{ID: id, VaultBackend: VaultBackendLocal}, nil
		},
		docGetByRelFn: func(_ context.Context, _, _ string) (Document, error) {
			return Document{}, apierror.NotFound(apierror.DomainKnowledge, "x")
		},
		docCreateFn: func(_ context.Context, d Document) (Document, error) {
			t.Fatal("local vault must never create moc page")
			return d, nil
		},
	}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{mocDisposalView()}}
	u := newMOCDisposalUsecase(repo, curate, &[]PromoteTouchedDoc{})

	err := u.ResolveGovernanceProposal(context.Background(), 77, ProposalStatusApplied)
	if err == nil || apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("non-team disposal must fail visibly, err=%v", err)
	}
	if curate.resolvedID != 0 {
		t.Fatalf("proposal must stay pending, resolved=%d", curate.resolvedID)
	}
}

// 历史提案缺字段降级 no-op：零写零错误（既有 kind 先例语义）。
func TestApplyMOCDisposal_LegacyPayloadNoop(t *testing.T) {
	t.Parallel()
	store := &mocDocStore{}
	legacy := mocDisposalView()
	legacy.Payload = map[string]any{"dedup_key": "moc:d-hub"}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{legacy}}
	u := newMOCDisposalUsecase(store.repo(), curate, &[]PromoteTouchedDoc{})

	if err := u.ResolveGovernanceProposal(context.Background(), 77, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 0 || len(store.updated) != 0 {
		t.Fatalf("legacy payload must be no-op: %+v / %+v", store.created, store.updated)
	}
	if curate.resolvedStatus != ProposalStatusApplied {
		t.Fatalf("legacy no-op still resolves applied, got %q", curate.resolvedStatus)
	}
}

// ADR-B：dedup_key = moc:<hub>:<成员签名>——同 hub 同簇沉默、簇演进可重提。
func TestCurateKnowledge_MOCEmergeDedupMemberSignature(t *testing.T) {
	makeHub := func(members ...string) HubClusterStat {
		h := HubClusterStat{HubDocID: "d-hub", HubRelPath: "entries/发布体系.md", Degree: 5}
		for i, m := range members {
			h.Neighbors = append(h.Neighbors, HubMember{DocID: strings.Repeat("d", i+1), RelPath: m})
		}
		return h
	}
	base := []string{"entries/a.md", "entries/b.md", "entries/c.md", "entries/d.md"}
	evolved := []string{"entries/a.md", "entries/b.md", "entries/c.md", "entries/e.md"}

	proposalFor := func(members []string) GovernanceProposal {
		curate := &stubCurateRepo{hubs: []HubClusterStat{makeHub(members...)}, edgesWithin: 7}
		proposals := &stubProposalRepo{}
		u := newCurateUsecase(curate, proposals)
		if _, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"}); err != nil {
			t.Fatal(err)
		}
		if len(proposals.items) != 1 {
			t.Fatalf("proposals = %+v", proposals.items)
		}
		return proposals.items[0]
	}

	p1, p2 := proposalFor(base), proposalFor(evolved)
	k1, _ := p1.Payload["dedup_key"].(string)
	k2, _ := p2.Payload["dedup_key"].(string)
	prefix := "moc:d-hub:"
	if !strings.HasPrefix(k1, prefix) || len(k1) != len(prefix)+mocMemberSigLen {
		t.Fatalf("dedup_key = %q, want %q + %d-hex member signature", k1, prefix, mocMemberSigLen)
	}
	if k1 == k2 {
		t.Fatalf("cluster evolution must unlock re-proposal: %q vs %q", k1, k2)
	}
	// 同簇（成员次序扰动）签名不变——curate 幂等保障不破。
	p3 := proposalFor([]string{base[3], base[2], base[1], base[0]})
	k3, _ := p3.Payload["dedup_key"].(string)
	if k1 != k3 {
		t.Fatalf("same cluster different scan order must share key: %q vs %q", k1, k3)
	}
}
