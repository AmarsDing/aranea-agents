package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ── 自治理知识图谱 M3 演化时序层（写回侧） ─────────────────────────────────
// M3.1 supersedes 版本链：同 fact_id 整段替换生效时旧段快照留痕；
// 幂等重写（内容未变）不留痕；repo 未接线降级安全。
// G-40（M3.2 升级）写回四操作仲裁（ADR-KN-20261028-G40-ARB4）：
//   - update 高置信（≥0.8）→ 走版本链顶替目标段；
//   - delete 置信分带——≥0.7 旧段不动+新追加+高风险 pending 提案人工二审；
//     [0.5,0.7) 自动失效（旧段移除+新追加+版本链墓碑 NewBody 空+低风险 applied
//     提案审计痕）；<0.5 纯追加；
//   - noop 零旁效应跳过；add/仲裁未判默认追加（NOOP 必须显式判定）；
//   - update/delete 目标失配降级（D7）：update→平追加；delete→追加+高风险提案；
//   - 事故闸 KNOWLEDGE_WRITEBACK_DELETE_AUTO_INVALIDATE_DISABLED=1 → 自动失效
//     带整带关闭，回落 G-40 前行为（D6）；
//   - 低置信/仲裁失败/新建页 → 降级原追加行为。

// ── stubs ─────────────────────────────────────────────────────────────────

type stubFactVersionRepo struct{ items []FactVersion }

func (s *stubFactVersionRepo) InsertFactVersion(_ context.Context, v FactVersion) error {
	s.items = append(s.items, v)
	return nil
}

func (s *stubFactVersionRepo) LatestFactVersion(_ context.Context, docID, factID string) (FactVersion, bool, error) {
	for i := len(s.items) - 1; i >= 0; i-- {
		if s.items[i].DocID == docID && s.items[i].FactID == factID {
			return s.items[i], true, nil
		}
	}
	return FactVersion{}, false, nil
}

type stubFactEdgeCloser struct {
	calls []struct{ collectionID, docID, evidence string }
}

func (s *stubFactEdgeCloser) CloseSemanticEdgesEvidencedBy(_ context.Context, collectionID, docID, evidence string) (int, error) {
	s.calls = append(s.calls, struct{ collectionID, docID, evidence string }{collectionID, docID, evidence})
	return 1, nil
}

type stubProposalRepo struct {
	items         []GovernanceProposal
	hasProposal   bool
	hasKeys       map[string]bool // 非 nil 时按 dedup_key 精确应答（跨带命名空间钉用）
	dedupKey      string
	dedupKeysSeen []string
	dedupStatuses []string
}

func (s *stubProposalRepo) InsertProposal(_ context.Context, p GovernanceProposal) error {
	s.items = append(s.items, p)
	return nil
}

func (s *stubProposalRepo) HasProposal(_ context.Context, _, _, dedupKey string, statuses []string) (bool, error) {
	s.dedupKey = dedupKey
	s.dedupKeysSeen = append(s.dedupKeysSeen, dedupKey)
	s.dedupStatuses = append([]string(nil), statuses...)
	if s.hasKeys != nil {
		return s.hasKeys[dedupKey], nil
	}
	return s.hasProposal, nil
}

type stubWriteBackArbiter struct {
	verdicts     []WriteBackArbitration
	err          error
	calls        int
	lastTitle    string
	lastExisting []WriteBackFactBlock
	lastNews     []WriteBackFact
}

func (s *stubWriteBackArbiter) ArbitrateWriteBack(_ context.Context, title string, existing []WriteBackFactBlock, news []WriteBackFact) ([]WriteBackArbitration, error) {
	s.calls++
	s.lastTitle = title
	s.lastExisting = existing
	s.lastNews = news
	return s.verdicts, s.err
}

// seedExistingEntry 预置一篇带 fact 段的存量词条页。
func seedExistingEntry(f *writeBackEntryFixture, rel, title, oldStmt, oldFactID string) Document {
	existing := Document{
		ID: "doc-entry", CollectionID: "team-1", RelPath: rel,
		ContentText: writeBackEntryHeader([]string{title}) + "## constraint\n\n" + oldStmt + "\n\n- fact_id: `" + oldFactID + "`\n",
	}
	f.docs[existing.ID] = existing
	f.byRel["team-1|"+rel] = existing.ID
	return existing
}

// ── M3.1 supersedes 版本链 ────────────────────────────────────────────────

// 同 fact_id 再写入（陈述变化）→ 旧段快照入版本链，FactID/OldBody/NewBody 正确。
func TestWriteBack_FactVersionRecordedOnReplace(t *testing.T) {
	f := newWriteBackEntryFixture()
	versions := &stubFactVersionRepo{}
	f.u.SetEvolutionRepos(versions, &stubProposalRepo{})

	in := WriteBackInput{
		SessionID: "sess-v1",
		Facts:     []WriteBackFact{taggedFact("fid-1", "值班每 8 小时轮换", "constraint", 0.9, "值班制度")},
	}
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	in.Facts[0].Statement = "值班改为每 12 小时轮换"
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if len(versions.items) != 1 {
		t.Fatalf("versions = %+v, want 1 record", versions.items)
	}
	v := versions.items[0]
	entry, _ := f.docByRel("entries/值班制度.md")
	if v.CollectionID != "team-1" || v.DocID != entry.ID || v.FactID != "fid-1" {
		t.Fatalf("version identity = %+v", v)
	}
	if !strings.Contains(v.OldBody, "每 8 小时轮换") {
		t.Fatalf("old snapshot missing: %q", v.OldBody)
	}
	if !strings.Contains(v.NewBody, "每 12 小时轮换") {
		t.Fatalf("new body missing: %q", v.NewBody)
	}
}

// 幂等重写（内容未变）→ 不留痕、不触发改动。
func TestWriteBack_FactVersionSkippedWhenIdempotent(t *testing.T) {
	f := newWriteBackEntryFixture()
	versions := &stubFactVersionRepo{}
	f.u.SetEvolutionRepos(versions, &stubProposalRepo{})

	in := WriteBackInput{
		SessionID: "sess-v2",
		Facts:     []WriteBackFact{taggedFact("fid-1", "值班每 8 小时轮换", "constraint", 0.9, "值班制度")},
	}
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if len(versions.items) != 0 {
		t.Fatalf("idempotent rewrite must not record version: %+v", versions.items)
	}
}

// 版本 repo 未接线 → 替换主流程正常，不 panic（降级安全）。
func TestWriteBack_FactVersionRepoNotWired(t *testing.T) {
	f := newWriteBackEntryFixture()
	in := WriteBackInput{
		SessionID: "sess-v3",
		Facts:     []WriteBackFact{taggedFact("fid-1", "值班每 8 小时轮换", "constraint", 0.9, "值班制度")},
	}
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	in.Facts[0].Statement = "值班改为每 12 小时轮换"
	if _, err := f.u.WriteBackSessionFacts(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	entry, _ := f.docByRel("entries/值班制度.md")
	if !strings.Contains(entry.ContentText, "每 12 小时轮换") {
		t.Fatalf("replace must still work without version repo: %q", entry.ContentText)
	}
}

// ── G-40 写回四操作仲裁（钉阵 ①~⑨，ADR-KN-20261028-G40-ARB4 §⑥；C 批 fix-up
// 消费面钉 ⑩~⑮ 见文件尾） ───────────

// 钉②：update 高置信（≥0.8）→ 顶替目标旧段（版本链留痕），新事实不再追加（回归现行）。
func TestWriteBack_ArbiterUpdateReplacesTarget(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	arbiter := &stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpUpdate, TargetFactID: "fid-old", Confidence: 0.92, Reason: "同一事实的更新"},
	}}
	f.u.SetWriteBackArbiter(arbiter)

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a1",
		Facts:     []WriteBackFact{taggedFact("fid-new", "灰度比例提升至 20% 起步", "constraint", 0.95, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if arbiter.calls != 1 {
		t.Fatalf("arbiter calls = %d, want 1", arbiter.calls)
	}
	if len(arbiter.lastExisting) != 1 || arbiter.lastExisting[0].FactID != "fid-old" {
		t.Fatalf("arbiter existing candidates = %+v", arbiter.lastExisting)
	}
	if len(arbiter.lastNews) != 1 || arbiter.lastNews[0].FactID != "fid-new" {
		t.Fatalf("arbiter news = %+v", arbiter.lastNews)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "20% 起步") || strings.Contains(entry.ContentText, "5% 起步") {
		t.Fatalf("target section not superseded: %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "fact_id: `fid-old`") {
		t.Fatalf("stable fact lineage marker was lost: %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "source_id: `fid-new`") {
		t.Fatalf("incoming fact provenance was lost: %q", entry.ContentText)
	}
	if len(versions.items) != 1 || versions.items[0].FactID != "fid-old" {
		t.Fatalf("update must record version chain: %+v", versions.items)
	}
	if len(proposals.items) != 0 {
		t.Fatalf("update must not raise proposal: %+v", proposals.items)
	}
}

// update 低置信（<0.8）→ 不顶替，降级为正常追加。
func TestWriteBack_ArbiterUpdateLowConfidenceAppends(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")
	versions := &stubFactVersionRepo{}
	f.u.SetEvolutionRepos(versions, &stubProposalRepo{})
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpUpdate, TargetFactID: "fid-old", Confidence: 0.5, Reason: "疑似相关"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a2",
		Facts:     []WriteBackFact{taggedFact("fid-new", "灰度比例提升至 20% 起步", "constraint", 0.95, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "5% 起步") || !strings.Contains(entry.ContentText, "20% 起步") {
		t.Fatalf("low-confidence update must fall back to append: %q", entry.ContentText)
	}
	if len(versions.items) != 0 {
		t.Fatalf("low-confidence update must not record version: %+v", versions.items)
	}
}

// 钉③：update 目标失配（页内无此 fact_id 段）→ 降级平追加，不留版本不产提案。
func TestWriteBack_ArbiterUpdateTargetMissAppends(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpUpdate, TargetFactID: "fid-ghost", Confidence: 0.9, Reason: "账实漂移目标"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a3u",
		Facts:     []WriteBackFact{taggedFact("fid-new", "灰度比例提升至 20% 起步", "constraint", 0.95, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "5% 起步") || !strings.Contains(entry.ContentText, "20% 起步") {
		t.Fatalf("update with missing target must degrade to append: %q", entry.ContentText)
	}
	if len(versions.items) != 0 || len(proposals.items) != 0 {
		t.Fatalf("missed update must be side-effect free: versions=%+v proposals=%+v", versions.items, proposals.items)
	}
}

// 钉⑤：delete 高置信（≥0.7）→ 旧段不覆盖，新事实仍追加，高风险 pending 提案待人工二审。
func TestWriteBack_ArbiterDeleteHighBandRecordsProposal(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.85, Reason: "与既有禁令直接矛盾"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a3",
		Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "禁止自动发布") {
		t.Fatalf("contradicted old section must stay: %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "允许自动发布") {
		t.Fatalf("new fact must still append for review: %q", entry.ContentText)
	}
	if len(versions.items) != 0 {
		t.Fatalf("high-band delete must not touch version chain: %+v", versions.items)
	}
	if len(proposals.items) != 1 {
		t.Fatalf("high-band delete must record one proposal: %+v", proposals.items)
	}
	p := proposals.items[0]
	if p.CollectionID != "team-1" || p.Kind != ProposalKindConflict || p.Risk != ProposalRiskHigh {
		t.Fatalf("proposal identity = %+v", p)
	}
	if p.Status != "" {
		t.Fatalf("high-band delete proposal must stay pending (empty status), got %q", p.Status)
	}
	if p.Payload["target_fact_id"] != "fid-old" || p.Payload["new_fact_id"] != "fid-new" {
		t.Fatalf("proposal payload = %+v", p.Payload)
	}
	if p.Payload["dedup_key"] != "conflict:fact:doc-entry:fid-old:fid-new" {
		t.Fatalf("proposal dedup_key = %v", p.Payload["dedup_key"])
	}
	if p.Payload["new_statement"] != "生产环境允许自动发布" {
		t.Fatalf("proposal payload statement = %+v", p.Payload)
	}
}

func TestRecordConflictProposal_SkipsExistingFactConflict(t *testing.T) {
	repo := &stubProposalRepo{hasProposal: true}
	u := NewUsecase(nil, nil, nil)
	u.SetEvolutionRepos(nil, repo)
	payload := map[string]any{
		"dedup_key": "conflict:fact:d1:old:new",
	}

	u.recordConflictProposal(context.Background(), "c1", "d1", payload)

	if len(repo.items) != 0 {
		t.Fatalf("duplicate conflict proposal inserted: %+v", repo.items)
	}
	if repo.dedupKey != payload["dedup_key"] {
		t.Fatalf("dedup key = %q", repo.dedupKey)
	}
	if len(repo.dedupStatuses) != 3 || repo.dedupStatuses[2] != ProposalStatusRejected {
		t.Fatalf("dedup statuses = %v", repo.dedupStatuses)
	}
}

// 钉④：delete 低置信带 [0.5,0.7) → 自动失效：旧段移除、新追加、版本链墓碑
// （NewBody 空串=失效）、低风险 applied 提案审计痕（payload 证据齐全）。
func TestWriteBack_ArbiterDeleteAutoInvalidates(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.6, Reason: "推断与旧禁令矛盾"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a4",
		Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if strings.Contains(entry.ContentText, "禁止自动发布") {
		t.Fatalf("auto-invalidate must remove old section: %q", entry.ContentText)
	}
	if strings.Contains(entry.ContentText, "fact_id: `fid-old`") {
		t.Fatalf("old fact marker must be gone with the section: %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "允许自动发布") || !strings.Contains(entry.ContentText, "fact_id: `fid-new`") {
		t.Fatalf("new fact must still append: %q", entry.ContentText)
	}
	// 版本链墓碑：OldBody=旧段快照、NewBody 空串（回放读空即识别为失效墓碑）。
	if len(versions.items) != 1 {
		t.Fatalf("auto-invalidate must record tombstone version: %+v", versions.items)
	}
	tv := versions.items[0]
	if tv.FactID != "fid-old" || !strings.Contains(tv.OldBody, "禁止自动发布") || tv.NewBody != "" {
		t.Fatalf("tombstone version = %+v, want OldBody snapshot + empty NewBody", tv)
	}
	// applied 提案审计痕：低风险直落、auto_invalidated 证据、dedup 键带 :auto
	// 命名空间（C 批修 #14——自动失效审计单与高风险人工二审单跨带不互吞）。
	if len(proposals.items) != 1 {
		t.Fatalf("auto-invalidate must record one audit proposal: %+v", proposals.items)
	}
	p := proposals.items[0]
	if p.Kind != ProposalKindConflict || p.Risk != ProposalRiskLow || p.Status != ProposalStatusApplied {
		t.Fatalf("audit proposal identity = %+v, want conflict/low/applied", p)
	}
	if p.Payload["auto_invalidated"] != true {
		t.Fatalf("payload missing auto_invalidated evidence: %+v", p.Payload)
	}
	if p.Payload["target_fact_id"] != "fid-old" || p.Payload["new_fact_id"] != "fid-new" {
		t.Fatalf("payload fact ids = %+v", p.Payload)
	}
	if p.Payload["confidence"] != 0.6 || p.Payload["reason"] != "推断与旧禁令矛盾" {
		t.Fatalf("payload confidence/reason = %+v", p.Payload)
	}
	if p.Payload["dedup_key"] != "conflict:fact:doc-entry:fid-old:fid-new:auto" {
		t.Fatalf("payload dedup_key = %v", p.Payload["dedup_key"])
	}
}

// 钉⑥：delete 目标失配（账实漂移）→ 降级同高带：追加+高风险 pending 提案；
// 不丢新事实、不误删（旧页内容原样）。
func TestWriteBack_ArbiterDeleteTargetMissEscalates(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-ghost", Confidence: 0.6, Reason: "目标段已被人工改走"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a6",
		Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "禁止自动发布") || !strings.Contains(entry.ContentText, "允许自动发布") {
		t.Fatalf("missed delete must keep old and append new: %q", entry.ContentText)
	}
	if len(versions.items) != 0 {
		t.Fatalf("missed delete must not touch version chain: %+v", versions.items)
	}
	if len(proposals.items) != 1 {
		t.Fatalf("missed delete must escalate to high-risk pending proposal: %+v", proposals.items)
	}
	p := proposals.items[0]
	if p.Kind != ProposalKindConflict || p.Risk != ProposalRiskHigh || p.Status != "" {
		t.Fatalf("escalated proposal identity = %+v, want conflict/high/pending", p)
	}
	if p.Payload["auto_invalidated"] != nil {
		t.Fatalf("escalated proposal must not carry auto_invalidated: %+v", p.Payload)
	}
}

// 钉⑦：delete c<0.5 → 纯追加（不 noop 化——未达自动带也不人造保守）。
func TestWriteBack_ArbiterDeleteBelowBandAppends(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.45, Reason: "臆测矛盾"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a7",
		Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "禁止自动发布") || !strings.Contains(entry.ContentText, "允许自动发布") {
		t.Fatalf("below-band delete must plain append: %q", entry.ContentText)
	}
	if len(versions.items) != 0 || len(proposals.items) != 0 {
		t.Fatalf("below-band delete must be side-effect free: versions=%+v proposals=%+v", versions.items, proposals.items)
	}
}

// 钉①：add 显式直出 + 仲裁未判（不在输出列表）→ 一律默认追加（不 NOOP 化），
// 零版本零提案。
func TestWriteBack_ArbiterAddAndAbsentAppend(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpAdd, Confidence: 0.95, Reason: "全新内容"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a8",
		Facts: []WriteBackFact{
			taggedFact("fid-new-1", "预发环境强制全链路回归", "constraint", 0.9, "灰度发布"),
			taggedFact("fid-new-2", "灰度观察窗至少 30 分钟", "constraint", 0.9, "灰度发布"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "全链路回归") || !strings.Contains(entry.ContentText, "30 分钟") {
		t.Fatalf("add explicit + absent arbitration must both append: %q", entry.ContentText)
	}
	if len(versions.items) != 0 || len(proposals.items) != 0 {
		t.Fatalf("add must be side-effect free: versions=%+v proposals=%+v", versions.items, proposals.items)
	}
}

// 钉⑧：noop 零旁效应——不写入、不留版本、不产提案（旧页原样）。
func TestWriteBack_ArbiterNoopSkipsZeroEffects(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpNoop, TargetFactID: "fid-old", Confidence: 0.99, Reason: "与既有段语义重复"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a9",
		Facts:     []WriteBackFact{taggedFact("fid-new", "灰度起步比例就是 5%", "constraint", 0.95, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if strings.Contains(entry.ContentText, "就是 5%") {
		t.Fatalf("noop must skip writing: %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "5% 起步") {
		t.Fatalf("noop must not disturb existing content: %q", entry.ContentText)
	}
	if len(versions.items) != 0 || len(proposals.items) != 0 {
		t.Fatalf("noop must be fully side-effect free: versions=%+v proposals=%+v", versions.items, proposals.items)
	}
}

// 钉⑨：事故闸置位 → DELETE 自动失效带整带关闭，回落 G-40 前行为（旧段不动、
// 新追加、零提案零版本；高带人工路径不受影响——由钉⑤回归覆盖）。
func TestWriteBack_ArbiterDeleteAutoInvalidateKillSwitch(t *testing.T) {
	t.Setenv("KNOWLEDGE_WRITEBACK_DELETE_AUTO_INVALIDATE_DISABLED", "1")
	f := newWriteBackEntryFixture() // env 须在构造 usecase 前置位（构造期读一次）
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.6, Reason: "推断与旧禁令矛盾"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a10",
		Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "禁止自动发布") || !strings.Contains(entry.ContentText, "允许自动发布") {
		t.Fatalf("kill-switch must restore pre-G40 behaviour (old stays + append): %q", entry.ContentText)
	}
	if len(versions.items) != 0 || len(proposals.items) != 0 {
		t.Fatalf("kill-switch band must be side-effect free: versions=%+v proposals=%+v", versions.items, proposals.items)
	}
}

// 仲裁器调用失败 → 降级为直接追加，不阻断写回、不留痕。
func TestWriteBack_ArbiterErrorDegradesToAppend(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{err: errors.New("llm timeout")})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a5",
		Facts:     []WriteBackFact{taggedFact("fid-new", "灰度比例提升至 20% 起步", "constraint", 0.95, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "5% 起步") || !strings.Contains(entry.ContentText, "20% 起步") {
		t.Fatalf("arbiter failure must degrade to append: %q", entry.ContentText)
	}
	if len(versions.items) != 0 || len(proposals.items) != 0 {
		t.Fatalf("arbiter failure must not record: versions=%+v proposals=%+v", versions.items, proposals.items)
	}
}

// 新建词条页 → 页内无既有段，不触发仲裁。
func TestWriteBack_ArbiterSkippedOnNewEntry(t *testing.T) {
	f := newWriteBackEntryFixture()
	arbiter := &stubWriteBackArbiter{}
	f.u.SetWriteBackArbiter(arbiter)

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a6",
		Facts:     []WriteBackFact{taggedFact("fid-new", "灰度比例 5% 起步", "constraint", 0.95, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if arbiter.calls != 0 {
		t.Fatalf("new entry must skip arbitration, got %d calls", arbiter.calls)
	}
}

// 仲裁器未接线 → 行为与 M3 前一致（直接追加）。
func TestWriteBack_ArbiterNotWired(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a7",
		Facts:     []WriteBackFact{taggedFact("fid-new", "灰度比例提升至 20% 起步", "constraint", 0.95, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "5% 起步") || !strings.Contains(entry.ContentText, "20% 起步") {
		t.Fatalf("no arbiter must keep legacy append behaviour: %q", entry.ContentText)
	}
}

// ── G-40 C 批评审 fix-up 消费面钉（wf_6fda0b29-07e，评审 #11–#15）────────────

// 钉⑩（C 批，评审 #12）：update 高置信但新事实自身 FactID 为空 → 禁止顶替（空
// incoming 走 D7 失配降级平追加）——无 FactID 的新段若顶替成功，目标段 fact_id
// lineage marker 会从正文永久抹掉（preserveSupersededFactID 契约）。
func TestWriteBack_ArbiterUpdateEmptyIncomingIDAppends(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpUpdate, TargetFactID: "fid-old", Confidence: 0.92, Reason: "同一事实的更新"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-c1",
		Facts:     []WriteBackFact{taggedFact("", "灰度比例提升至 20% 起步", "constraint", 0.95, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "5% 起步") || !strings.Contains(entry.ContentText, "20% 起步") {
		t.Fatalf("empty-incoming update must degrade to append: %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "fact_id: `fid-old`") {
		t.Fatalf("target lineage marker must survive: %q", entry.ContentText)
	}
	if len(versions.items) != 0 || len(proposals.items) != 0 {
		t.Fatalf("D7 degrade must be side-effect free: versions=%+v proposals=%+v", versions.items, proposals.items)
	}
}

// 钉⑪（C 批，评审 #11）：delete 自动带内空 TargetFactID → 不再静默平追加，
// 纳入 D7 升级面：追加 + 高风险 pending 提案（与正文失配同分支）。
func TestWriteBack_ArbiterDeleteEmptyTargetEscalates(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "", Confidence: 0.6, Reason: "arbiter 未命名目标"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-c2",
		Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "禁止自动发布") || !strings.Contains(entry.ContentText, "允许自动发布") {
		t.Fatalf("empty-target delete must keep old and append new: %q", entry.ContentText)
	}
	if len(versions.items) != 0 {
		t.Fatalf("empty-target delete must not touch version chain: %+v", versions.items)
	}
	if len(proposals.items) != 1 {
		t.Fatalf("empty-target delete must escalate to high-risk pending proposal: %+v", proposals.items)
	}
	p := proposals.items[0]
	if p.Kind != ProposalKindConflict || p.Risk != ProposalRiskHigh || p.Status != "" {
		t.Fatalf("escalated proposal identity = %+v, want conflict/high/pending", p)
	}
	if p.Payload["auto_invalidated"] != nil {
		t.Fatalf("escalated proposal must not carry auto_invalidated: %+v", p.Payload)
	}
}

// 钉⑫（C 批，评审 #13）：同批 update→delete 命中同一 TargetFactID —— update 先
// 消费该目标，后到的 delete 一律 D7 降级（追加+高风险 pending），禁止把刚顶替
// 的新段当旧段静默移除（0.9 更新被 0.6 矛盾吃掉且仅落低风险单）。
func TestWriteBack_ArbiterSameBatchUpdateThenDeleteSameTarget(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpUpdate, TargetFactID: "fid-old", Confidence: 0.9, Reason: "同一事实的更新"},
		{FactIndex: 1, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.6, Reason: "推断与旧口径矛盾"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-c3",
		Facts: []WriteBackFact{
			taggedFact("fid-upd", "灰度比例提升至 20% 起步", "constraint", 0.95, "灰度发布"),
			taggedFact("fid-del", "灰度起步比例可直接拉满 100%", "decision", 0.9, "灰度发布"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "20% 起步") || strings.Contains(entry.ContentText, "5% 起步") {
		t.Fatalf("update must supersede target (consumed first): %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "fact_id: `fid-old`") {
		t.Fatalf("updated block must keep lineage marker: %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "拉满 100%") {
		t.Fatalf("degraded delete fact must still append: %q", entry.ContentText)
	}
	if len(versions.items) != 1 || versions.items[0].NewBody == "" {
		t.Fatalf("update must record exactly one supersede version: %+v", versions.items)
	}
	if len(proposals.items) != 1 {
		t.Fatalf("intra-batch delete must escalate to manual review (not silent auto): %+v", proposals.items)
	}
	p := proposals.items[0]
	if p.Risk != ProposalRiskHigh || p.Status != "" {
		t.Fatalf("second op on consumed target must be high-risk pending: %+v", p)
	}
	if p.Payload["new_fact_id"] != "fid-del" || p.Payload["auto_invalidated"] != nil {
		t.Fatalf("degraded proposal payload = %+v", p.Payload)
	}
}

// 钉⑬（C 批，评审 #13 逆序）：同批 delete→update 同目标——delete 自动失效先消费
// 目标（旧段移除+墓碑+applied 审计痕），后到的 update 禁止再就地改写（同批已
// 消费 → D7 降级平追加，不留版本）。
func TestWriteBack_ArbiterSameBatchDeleteThenUpdateSameTarget(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.6, Reason: "推断与旧禁令矛盾"},
		{FactIndex: 1, Operation: WriteBackOpUpdate, TargetFactID: "fid-old", Confidence: 0.9, Reason: "同一事实的更新"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-c4",
		Facts: []WriteBackFact{
			taggedFact("fid-del", "生产环境允许自动发布", "decision", 0.9, "灰度发布"),
			taggedFact("fid-upd", "生产环境灰度窗口缩短至 10 分钟", "constraint", 0.95, "灰度发布"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if strings.Contains(entry.ContentText, "禁止自动发布") {
		t.Fatalf("delete auto-invalidate must remove old section: %q", entry.ContentText)
	}
	if !strings.Contains(entry.ContentText, "允许自动发布") || !strings.Contains(entry.ContentText, "缩短至 10 分钟") {
		t.Fatalf("both news facts must append (update degraded): %q", entry.ContentText)
	}
	if len(versions.items) != 1 || versions.items[0].NewBody != "" {
		t.Fatalf("only the delete tombstone version is recorded: %+v", versions.items)
	}
	if len(proposals.items) != 1 {
		t.Fatalf("only the auto-invalidate audit proposal is queued: %+v", proposals.items)
	}
	p := proposals.items[0]
	if p.Risk != ProposalRiskLow || p.Status != ProposalStatusApplied || p.Payload["auto_invalidated"] != true {
		t.Fatalf("auto audit proposal identity = %+v", p)
	}
}

// 钉⑭（C 批，评审 #14）：跨带 dedup 命名空间——自动失效审计单（:auto 后缀键）与
// 高风险人工二审单（无名空间键）互不抑制：applied 审计痕不得吞掉同矛盾后续
// 高置信复裁；同带 verbatim 复发仍被 :auto 键正常去重。
func TestWriteBack_ArbiterDeleteCrossBandDedupNamespaces(t *testing.T) {
	newIn := func() WriteBackInput {
		return WriteBackInput{
			SessionID: "sess-c5",
			Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
		}
	}
	run := func(t *testing.T, proposals *stubProposalRepo, v WriteBackArbitration) *writeBackEntryFixture {
		f := newWriteBackEntryFixture()
		seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
		f.u.SetEvolutionRepos(&stubFactVersionRepo{}, proposals)
		f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{v}})
		if _, err := f.u.WriteBackSessionFacts(context.Background(), newIn()); err != nil {
			t.Fatal(err)
		}
		return f
	}

	// 既有 applied 自动失效单（:auto 键）在场 → 同矛盾高置信复裁必须照常落
	// 高风险 pending 单（探测的是无名空间键，不会被 :auto 行吞掉）。
	proposals := &stubProposalRepo{hasKeys: map[string]bool{
		"conflict:fact:doc-entry:fid-old:fid-new:auto": true,
	}}
	run(t, proposals, WriteBackArbitration{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.85, Reason: "高置信复裁"})
	if len(proposals.items) != 1 || proposals.items[0].Risk != ProposalRiskHigh || proposals.items[0].Status != "" {
		t.Fatalf("high-band re-arbitration must not be swallowed by applied auto audit: %+v", proposals.items)
	}
	if len(proposals.dedupKeysSeen) != 1 || proposals.dedupKeysSeen[0] != "conflict:fact:doc-entry:fid-old:fid-new" {
		t.Fatalf("high-band probe key must keep legacy namespace: %v", proposals.dedupKeysSeen)
	}

	// 同带 verbatim 复发（applied :auto 行已在场）→ 自动失效审计单仍被去重，
	// 不重复留痕（墓碑版本照常——正文层动作与审计幂等正交）。
	proposals2 := &stubProposalRepo{hasKeys: map[string]bool{
		"conflict:fact:doc-entry:fid-old:fid-new:auto": true,
	}}
	run(t, proposals2, WriteBackArbitration{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.6, Reason: "同带重放"})
	if len(proposals2.items) != 0 {
		t.Fatalf("same-band verbatim replay must stay deduped: %+v", proposals2.items)
	}
	if len(proposals2.dedupKeysSeen) != 1 || proposals2.dedupKeysSeen[0] != "conflict:fact:doc-entry:fid-old:fid-new:auto" {
		t.Fatalf("auto audit probe key must carry :auto namespace: %v", proposals2.dedupKeysSeen)
	}
}

// 钉⑮（C 批，评审 #15）：占位 FactID=="stmt" 与空 ID 同纪律归一为陈述哈希——
// 两条同占位不同陈述的新事实同目标 delete 不再共用常量 dedup 键互相吞并。
func TestWriteBack_ArbiterDeleteStmtPlaceholderDistinctDedupKeys(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(&stubFactVersionRepo{}, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.85, Reason: "矛盾其一"},
		{FactIndex: 1, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.85, Reason: "矛盾其二"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-c6",
		Facts: []WriteBackFact{
			taggedFact("stmt", "生产环境允许自动发布", "decision", 0.9, "灰度发布"),
			taggedFact("stmt", "生产环境灰度窗口压缩一半", "constraint", 0.9, "灰度发布"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals.items) != 2 {
		t.Fatalf("two distinct stmt-placeholder facts must yield two proposals: %+v", proposals.items)
	}
	k0 := proposals.items[0].Payload["dedup_key"].(string)
	k1 := proposals.items[1].Payload["dedup_key"].(string)
	if k0 == k1 {
		t.Fatalf("placeholder dedup keys degenerated to a constant: %q", k0)
	}
	for _, k := range []string{k0, k1} {
		if !strings.HasPrefix(k, "conflict:fact:doc-entry:fid-old:") || strings.HasSuffix(k, ":stmt") {
			t.Fatalf("dedup key must normalize stmt placeholder to statement hash: %q", k)
		}
	}
}

// D8：DELETE 自动失效成功后，关闭以被删事实段为证据的 semantic 边。
func TestWriteBack_AutoInvalidateClosesEvidencedEdges(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	closer := &stubFactEdgeCloser{}
	f.u.SetFactEdgeCloser(closer)
	f.u.SetEvolutionRepos(&stubFactVersionRepo{}, &stubProposalRepo{})
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.6, Reason: "推断矛盾"},
	}})
	if _, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-d8",
		Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
	}); err != nil {
		t.Fatal(err)
	}
	if len(closer.calls) != 1 {
		t.Fatalf("edge close calls = %d, want 1", len(closer.calls))
	}
	got := closer.calls[0]
	if got.collectionID != "team-1" || got.docID != "doc-entry" || !strings.Contains(got.evidence, "禁止自动发布") {
		t.Fatalf("close args = %+v", got)
	}
}

// D8：目标失配不关边。
func TestWriteBack_DeleteMissDoesNotCloseEdges(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	closer := &stubFactEdgeCloser{}
	f.u.SetFactEdgeCloser(closer)
	f.u.SetEvolutionRepos(&stubFactVersionRepo{}, &stubProposalRepo{})
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-ghost", Confidence: 0.6, Reason: "失配"},
	}})
	if _, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-d8b",
		Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
	}); err != nil {
		t.Fatal(err)
	}
	if len(closer.calls) != 0 {
		t.Fatalf("miss must not close edges: %+v", closer.calls)
	}
}

// c1：墓碑版本可把被自动失效的事实段写回正文。
func TestRestoreTombstonedFact(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	versions := &stubFactVersionRepo{}
	f.u.SetEvolutionRepos(versions, &stubProposalRepo{})
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Operation: WriteBackOpDelete, TargetFactID: "fid-old", Confidence: 0.6, Reason: "推断矛盾"},
	}})
	if _, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-restore",
		Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.u.RestoreTombstonedFact(context.Background(), "doc-entry", "fid-old"); err != nil {
		t.Fatal(err)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "禁止自动发布") || !strings.Contains(entry.ContentText, "fact_id: `fid-old`") {
		t.Fatalf("restore must put the tombstoned section back: %q", entry.ContentText)
	}
	if len(versions.items) != 2 || versions.items[1].NewBody == "" || !strings.Contains(versions.items[1].NewBody, "禁止自动发布") {
		t.Fatalf("restore must append a reversal version: %+v", versions.items)
	}
}
