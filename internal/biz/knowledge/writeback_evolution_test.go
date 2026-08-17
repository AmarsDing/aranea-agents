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
// M3.2 写入时冲突检测：arbiter supersedes 高置信 → 走版本链顶替目标段；
// contradicts 高置信 → 旧段不覆盖、新事实仍追加、高风险提案留痕；
// 低置信/仲裁失败/新建页 → 降级原追加行为。

// ── stubs ─────────────────────────────────────────────────────────────────

type stubFactVersionRepo struct{ items []FactVersion }

func (s *stubFactVersionRepo) InsertFactVersion(_ context.Context, v FactVersion) error {
	s.items = append(s.items, v)
	return nil
}

type stubProposalRepo struct {
	items         []GovernanceProposal
	hasProposal   bool
	dedupKey      string
	dedupStatuses []string
}

func (s *stubProposalRepo) InsertProposal(_ context.Context, p GovernanceProposal) error {
	s.items = append(s.items, p)
	return nil
}

func (s *stubProposalRepo) HasProposal(_ context.Context, _, _, dedupKey string, statuses []string) (bool, error) {
	s.dedupKey = dedupKey
	s.dedupStatuses = append([]string(nil), statuses...)
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

// ── M3.2 写入时冲突检测 ───────────────────────────────────────────────────

// supersedes 高置信（≥0.8）→ 顶替目标旧段（版本链留痕），新事实不再追加。
func TestWriteBack_ArbiterSupersedesReplacesTarget(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	arbiter := &stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Verdict: "supersedes", TargetFactID: "fid-old", Confidence: 0.92, Reason: "同一事实的更新"},
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
		t.Fatalf("supersede must record version chain: %+v", versions.items)
	}
	if len(proposals.items) != 0 {
		t.Fatalf("supersede must not raise proposal: %+v", proposals.items)
	}
}

// supersedes 低置信（<0.8）→ 不顶替，降级为正常追加。
func TestWriteBack_ArbiterSupersedesLowConfidenceAppends(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "灰度比例 5% 起步。", "fid-old")
	versions := &stubFactVersionRepo{}
	f.u.SetEvolutionRepos(versions, &stubProposalRepo{})
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Verdict: "supersedes", TargetFactID: "fid-old", Confidence: 0.5, Reason: "疑似相关"},
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
		t.Fatalf("low-confidence supersede must fall back to append: %q", entry.ContentText)
	}
	if len(versions.items) != 0 {
		t.Fatalf("low-confidence supersede must not record version: %+v", versions.items)
	}
}

// contradicts 高置信（≥0.7）→ 旧段不覆盖，新事实仍追加，高风险提案留痕待人工二审。
func TestWriteBack_ArbiterContradictsRecordsProposal(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	versions := &stubFactVersionRepo{}
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(versions, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Verdict: "contradicts", TargetFactID: "fid-old", Confidence: 0.85, Reason: "与既有禁令直接矛盾"},
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
		t.Fatalf("contradicts must not touch version chain: %+v", versions.items)
	}
	if len(proposals.items) != 1 {
		t.Fatalf("contradicts must record one proposal: %+v", proposals.items)
	}
	p := proposals.items[0]
	if p.CollectionID != "team-1" || p.Kind != ProposalKindConflict || p.Risk != ProposalRiskHigh {
		t.Fatalf("proposal identity = %+v", p)
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

// contradicts 低置信（<0.7）→ 不留提案，正常追加。
func TestWriteBack_ArbiterContradictsLowConfidenceSkipped(t *testing.T) {
	f := newWriteBackEntryFixture()
	seedExistingEntry(f, "entries/灰度发布.md", "灰度发布", "生产环境禁止自动发布。", "fid-old")
	proposals := &stubProposalRepo{}
	f.u.SetEvolutionRepos(&stubFactVersionRepo{}, proposals)
	f.u.SetWriteBackArbiter(&stubWriteBackArbiter{verdicts: []WriteBackArbitration{
		{FactIndex: 0, Verdict: "contradicts", TargetFactID: "fid-old", Confidence: 0.6, Reason: "可能矛盾"},
	}})

	_, err := f.u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-a4",
		Facts:     []WriteBackFact{taggedFact("fid-new", "生产环境允许自动发布", "decision", 0.9, "灰度发布")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals.items) != 0 {
		t.Fatalf("low-confidence contradicts must not record proposal: %+v", proposals.items)
	}
	entry := f.docs["doc-entry"]
	if !strings.Contains(entry.ContentText, "允许自动发布") {
		t.Fatalf("new fact must append: %q", entry.ContentText)
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
