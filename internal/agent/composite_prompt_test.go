package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

type compositeRecallStub struct {
	hits []biz.CompositeRecallHit
}

func (s compositeRecallStub) RecallComposite(_ context.Context, _ biz.CompositeRecallQuery) ([]biz.CompositeRecallHit, error) {
	return s.hits, nil
}

func TestCompositeMemoryCue_FormatsFusedBlock(t *testing.T) {
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true, L2RecallEnabled: true, L3Enabled: true, L0InjectL3: true,
	})
	cue := CompositeMemoryCue(context.Background(), compositeRecallStub{hits: []biz.CompositeRecallHit{
		{Layer: "L2", Line: "Session A: fixed bug"},
		{Layer: "L3", Line: "Prefers Go"},
	}}, biz.Agent{ID: "a1"}, policy, biz.MemoryRuntimeContext{}, "sess", "bug", 0, nil)
	if !strings.Contains(cue, "L2+L3 memory") {
		t.Fatalf("missing header: %s", cue)
	}
	if !strings.Contains(cue, "[L2]") || !strings.Contains(cue, "[L3]") {
		t.Fatalf("missing layer tags: %s", cue)
	}
}

// L2 episode 全文 summary 必须压成 gist（2026-08-18 域 B up-03 缺陷根修）：
// 3 条 markdown 长摘要会把 800-token 共享预算吃到仅剩 2 条 L3 容量，
// 答案承载的 L3 变体被挤出注入块。压 gist 后 L3 事实应完整进入注入块。
func TestCompositeMemoryCue_L2GistCapFreesBudgetForL3(t *testing.T) {
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true, L2RecallEnabled: true, L3Enabled: true, L0InjectL3: true,
	})
	// 复刻 up-03 证据形态：L2 长摘要（markdown 表格 ~400 chars）+ L3 短事实。
	longSummary := "核心交换机的登录密码是什么？: **结论：无法提供**\n\n| 渠道 | 说明 |\n|---|---|\n" +
		strings.Repeat("| 密码保险箱 | 统一托管在密码管理系统中，按设备名检索申请授权，全程审计留痕 ", 8) +
		"L2尾部标记ENDMARKER"
	l3Answer := "为节能，机房空调温度由 22℃ 上调至 24℃"
	l3Threshold := "动环温度告警阈值为超过 28℃ 触发告警，空调设定基线为 24℃"
	l3Other1 := "数据库服务器 SRV-DB-03 原运行 MySQL 5.7，后迁移至 PostgreSQL 16"
	l3Other2 := "2026 年 7 月出口带宽扩容至 2 Gbps"

	cue, kept := CompositeMemoryCueWithHits(context.Background(), compositeRecallStub{hits: []biz.CompositeRecallHit{
		{Layer: "L2", Line: longSummary, Score: 0.9},
		{Layer: "L3", Line: l3Threshold, Score: 0.8},
		{Layer: "L3", Line: l3Answer, Score: 0.7},
		{Layer: "L3", Line: l3Other1, Score: 0.6},
		{Layer: "L3", Line: l3Other2, Score: 0.5},
	}}, biz.Agent{ID: "a1"}, policy, biz.MemoryRuntimeContext{}, "sess", "机房空调", 0, nil)

	// L2 行已被压到 gist 预算内（尾部内容截断，且带省略号）。
	if strings.Contains(cue, "ENDMARKER") {
		t.Fatalf("L2 line should be truncated to gist, got full summary in cue")
	}
	// 全部 4 条 L3 事实都应进入注入块（压 gist 前仅剩 ~2 条容量）。
	for _, want := range []string{l3Answer, l3Threshold, l3Other1, l3Other2} {
		if !strings.Contains(cue, want) {
			t.Fatalf("L3 fact starved out of cue: %q\ncue=%s", want, cue)
		}
	}
	if len(kept) != 5 {
		t.Fatalf("expected 5 kept hits (1 L2 gist + 4 L3), got %d", len(kept))
	}
}

// L3 短陈述不受 L2 gist 截断影响：只有 Layer=L2 的行才压 gist。
func TestCapL2GistLineOnlyAffectsLongLines(t *testing.T) {
	short := "Prefers Go"
	if got := capL2GistLine(short); got != short {
		t.Fatalf("short line must pass through, got %q", got)
	}
	long := strings.Repeat("长", recallL2GistMaxRunes+10)
	got := capL2GistLine(long)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated line should carry ellipsis, got %q", got)
	}
	if len([]rune(got)) != recallL2GistMaxRunes+1 { // +1 为省略号
		t.Fatalf("truncated line should be %d runes + ellipsis, got %d", recallL2GistMaxRunes, len([]rune(got)))
	}
}

// TestCompositeMemoryCue_MergesProactiveHits verifies that proactive recall
// hits are merged with RecallComposite results, deduplicated by line, and
// ranked by score (P3-11).
func TestCompositeMemoryCue_MergesProactiveHits(t *testing.T) {
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true, L2RecallEnabled: true, L3Enabled: true, L0InjectL3: true,
	})
	recallHits := []biz.CompositeRecallHit{
		{Layer: "L2", Line: "Session A: fixed bug", Score: 0.5},
		{Layer: "L3", Line: "Prefers Go", Score: 0.8},
	}
	proactiveHits := []biz.CompositeRecallHit{
		{Layer: "L3", Line: "Prefers Go", Score: 0.9},      // duplicate of recall hit
		{Layer: "L3", Line: "Lives in London", Score: 0.7}, // unique proactive hit
	}
	cue := CompositeMemoryCue(context.Background(), compositeRecallStub{hits: recallHits},
		biz.Agent{ID: "a1"}, policy, biz.MemoryRuntimeContext{}, "sess", "bug", 0, proactiveHits)
	if !strings.Contains(cue, "L2+L3 memory") {
		t.Fatalf("missing header: %s", cue)
	}
	if !strings.Contains(cue, "fixed bug") {
		t.Fatalf("missing recall hit: %s", cue)
	}
	if !strings.Contains(cue, "Prefers Go") {
		t.Fatalf("missing deduplicated hit: %s", cue)
	}
	if !strings.Contains(cue, "Lives in London") {
		t.Fatalf("missing proactive hit: %s", cue)
	}
	// Verify deduplication: "Prefers Go" should appear only once
	if strings.Count(cue, "Prefers Go") != 1 {
		t.Fatalf("expected 'Prefers Go' to appear once (deduplicated), got %d: %s", strings.Count(cue, "Prefers Go"), cue)
	}
}

// TestCompositeMemoryCue_ProactiveOnly verifies that proactive hits are
// rendered even when RecallComposite returns no hits.
func TestCompositeMemoryCue_ProactiveOnly(t *testing.T) {
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true, L2RecallEnabled: true, L3Enabled: true, L0InjectL3: true,
	})
	proactiveHits := []biz.CompositeRecallHit{
		{Layer: "L3", Line: "Lives in London", Score: 0.7},
	}
	cue := CompositeMemoryCue(context.Background(), compositeRecallStub{hits: nil},
		biz.Agent{ID: "a1"}, policy, biz.MemoryRuntimeContext{}, "sess", "bug", 0, proactiveHits)
	if !strings.Contains(cue, "L2+L3 memory") {
		t.Fatalf("missing header: %s", cue)
	}
	if !strings.Contains(cue, "Lives in London") {
		t.Fatalf("missing proactive hit: %s", cue)
	}
}

// ── FR-12.7: resident profile card cue ───────────────────────────────────

type fakeProfileCardReader struct {
	card *biz.ProfileCard
	err  error
}

func (f fakeProfileCardReader) GetProfileCard(_ context.Context, _, _ string) (*biz.ProfileCard, error) {
	return f.card, f.err
}

// TestProfileCardCue_RendersCard verifies the resident card is rendered with
// its block header (100% injection, no recall scoring).
func TestProfileCardCue_RendersCard(t *testing.T) {
	reader := fakeProfileCardReader{card: &biz.ProfileCard{
		AgentID: "a1", UserID: "u1", Content: "- 名叫张三\n- 喜欢咖啡",
	}}
	cue := ProfileCardCue(context.Background(), reader, "a1", "u1")
	if !strings.Contains(cue, "## 用户档案（长期记忆摘要，始终生效）") {
		t.Fatalf("missing card header: %q", cue)
	}
	if !strings.Contains(cue, "喜欢咖啡") {
		t.Fatalf("missing card content: %q", cue)
	}
}

// TestProfileCardCue_TruncatesLongCard verifies the hook-side hard rune cap
// (safety net beyond the distiller's own budget).
func TestProfileCardCue_TruncatesLongCard(t *testing.T) {
	reader := fakeProfileCardReader{card: &biz.ProfileCard{
		AgentID: "a1", Content: strings.Repeat("档", profileCardMaxRunes+50),
	}}
	cue := ProfileCardCue(context.Background(), reader, "a1", "")
	body := strings.TrimPrefix(cue, "## 用户档案（长期记忆摘要，始终生效）\n")
	if runes := []rune(body); len(runes) != profileCardMaxRunes+1 || !strings.HasSuffix(body, "…") {
		t.Fatalf("expected %d runes + ellipsis, got %d runes", profileCardMaxRunes, len([]rune(body)))
	}
}

// TestProfileCardCue_NoCard verifies best-effort behavior: nil reader, read
// error, missing card, and blank content all render "" without breaking a turn.
func TestProfileCardCue_NoCard(t *testing.T) {
	ctx := context.Background()
	if cue := ProfileCardCue(ctx, nil, "a1", "u1"); cue != "" {
		t.Fatalf("nil reader: expected empty, got %q", cue)
	}
	if cue := ProfileCardCue(ctx, fakeProfileCardReader{}, "a1", "u1"); cue != "" {
		t.Fatalf("no card: expected empty, got %q", cue)
	}
	errReader := fakeProfileCardReader{err: errors.New("db down")}
	if cue := ProfileCardCue(ctx, errReader, "a1", "u1"); cue != "" {
		t.Fatalf("read error: expected empty, got %q", cue)
	}
	blank := fakeProfileCardReader{card: &biz.ProfileCard{AgentID: "a1", Content: "  \n "}}
	if cue := ProfileCardCue(ctx, blank, "a1", "u1"); cue != "" {
		t.Fatalf("blank content: expected empty, got %q", cue)
	}
}

// TestMergeCompositeHits_Deduplication verifies that mergeCompositeHits
// deduplicates by line (case-insensitive) and respects the limit.
func TestMergeCompositeHits_Deduplication(t *testing.T) {
	recallHits := []biz.CompositeRecallHit{
		{Layer: "L2", Line: "Fixed bug", Score: 0.5},
		{Layer: "L3", Line: "Prefers Go", Score: 0.8},
	}
	proactiveHits := []biz.CompositeRecallHit{
		{Layer: "L3", Line: "prefers go", Score: 0.9}, // case-insensitive duplicate
		{Layer: "L3", Line: "Lives in London", Score: 0.7},
	}
	merged := mergeCompositeHits(recallHits, proactiveHits, 10, false)
	if len(merged) != 3 {
		t.Fatalf("expected 3 deduplicated hits, got %d: %+v", len(merged), merged)
	}
	// Higher score should come first; duplicate "prefers go" is dropped, keeping "Prefers Go" (0.8)
	if merged[0].Line != "Prefers Go" || merged[0].Score != 0.8 {
		t.Fatalf("expected 'Prefers Go' (0.8) first, got %+v", merged[0])
	}
}

// TestMergeCompositeHits_Limit verifies that mergeCompositeHits respects the
// limit parameter after deduplication and sorting.
func TestMergeCompositeHits_Limit(t *testing.T) {
	recallHits := []biz.CompositeRecallHit{
		{Layer: "L2", Line: "A", Score: 0.3},
		{Layer: "L2", Line: "B", Score: 0.5},
	}
	proactiveHits := []biz.CompositeRecallHit{
		{Layer: "L3", Line: "C", Score: 0.9},
		{Layer: "L3", Line: "D", Score: 0.7},
	}
	merged := mergeCompositeHits(recallHits, proactiveHits, 2, false)
	if len(merged) != 2 {
		t.Fatalf("expected 2 hits after limit, got %d: %+v", len(merged), merged)
	}
	// Top 2 by score: C (0.9) and D (0.7)
	if merged[0].Line != "C" || merged[1].Line != "D" {
		t.Fatalf("expected [C, D], got [%s, %s]", merged[0].Line, merged[1].Line)
	}
}

// TestCompositeMemoryCueWithHits_ReturnsMergedHits verifies the WithHits
// variant returns the same cue as CompositeMemoryCue plus the merged,
// deduplicated hit list used for recall-transparency events (R4).
func TestCompositeMemoryCueWithHits_ReturnsMergedHits(t *testing.T) {
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true, L2RecallEnabled: true, L3Enabled: true, L0InjectL3: true,
	})
	recallHits := []biz.CompositeRecallHit{
		{Layer: "L2", Line: "Session A: fixed bug", Score: 0.5},
		{Layer: "L3", Line: "Prefers Go", Score: 0.8},
	}
	proactiveHits := []biz.CompositeRecallHit{
		{Layer: "L3", Line: "Prefers Go", Score: 0.9}, // duplicate
		{Layer: "L3", Line: "Lives in London", Score: 0.7},
	}
	cue, hits := CompositeMemoryCueWithHits(context.Background(), compositeRecallStub{hits: recallHits},
		biz.Agent{ID: "a1"}, policy, biz.MemoryRuntimeContext{}, "sess", "bug", 0, proactiveHits)
	if !strings.Contains(cue, "L2+L3 memory") {
		t.Fatalf("missing header: %s", cue)
	}
	if len(hits) != 3 {
		t.Fatalf("expected 3 merged hits, got %d: %+v", len(hits), hits)
	}
	if hits[0].Line != "Prefers Go" {
		t.Fatalf("hits must be score-sorted, first = %+v", hits[0])
	}
}

// TestCompositeMemoryCueWithHits_NoHits verifies empty recall yields no hits.
func TestCompositeMemoryCueWithHits_NoHits(t *testing.T) {
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true, L2RecallEnabled: true, L3Enabled: true, L0InjectL3: true,
	})
	cue, hits := CompositeMemoryCueWithHits(context.Background(), compositeRecallStub{hits: nil},
		biz.Agent{ID: "a1"}, policy, biz.MemoryRuntimeContext{}, "sess", "bug", 0, nil)
	if cue != "" || len(hits) != 0 {
		t.Fatalf("empty recall must yield empty cue and no hits, got cue=%q hits=%d", cue, len(hits))
	}
}

// ── PinnedPreferenceCue (FR-M3) ─────────────────────────────────────────

type preferenceListerStub struct {
	rows  [][]byte
	err   error
	kinds []string
	limit int32
}

func (s *preferenceListerStub) ListActivePreferenceFacts(_ context.Context, _, _ string, kinds []string, limit int32) ([][]byte, error) {
	s.kinds = kinds
	s.limit = limit
	return s.rows, s.err
}

func pinnedRow(id, kind, statement string) []byte {
	b, _ := json.Marshal(map[string]any{"id": id, "fact_kind": kind, "statement": statement})
	return b
}

func TestPinnedPreferenceCue_FormatsBlock(t *testing.T) {
	stub := &preferenceListerStub{rows: [][]byte{
		pinnedRow("f1", "preference", "User prefers dark mode"),
		pinnedRow("f2", "constraint", "Never use tool X"),
	}}
	cue := PinnedPreferenceCue(context.Background(), stub, "agent-1", "user-1")
	if !strings.Contains(cue, "用户偏好与工作要求") {
		t.Fatalf("missing header: %s", cue)
	}
	// P2a: compliance guidance line must accompany the header.
	if !strings.Contains(cue, "逐条核对") {
		t.Fatalf("missing compliance guidance: %s", cue)
	}
	if !strings.Contains(cue, "- [PREFERENCE] User prefers dark mode") {
		t.Fatalf("missing preference line: %s", cue)
	}
	if !strings.Contains(cue, "- [CONSTRAINT] Never use tool X") {
		t.Fatalf("missing constraint line: %s", cue)
	}
}

// TestPinnedPreferenceCue_RendersSelfMarkingKinds verifies G 维 P1: facts from
// the immediate self-marking taxonomy (user_preference / agent_instruction)
// enter the pinned block — agent_instruction renders with the [RULE] prefix,
// user_preference keeps [PREFERENCE].
func TestPinnedPreferenceCue_RendersSelfMarkingKinds(t *testing.T) {
	stub := &preferenceListerStub{rows: [][]byte{
		pinnedRow("f1", "user_preference", "回答必须使用中文"),
		pinnedRow("f2", "agent_instruction", "每次回答末尾必须附尾注"),
	}}
	cue, ids := PinnedPreferenceCueWithIDs(context.Background(), stub, "agent-1", "user-1")
	if !strings.Contains(cue, "- [PREFERENCE] 回答必须使用中文") {
		t.Fatalf("missing user_preference line: %s", cue)
	}
	if !strings.Contains(cue, "- [RULE] 每次回答末尾必须附尾注") {
		t.Fatalf("missing agent_instruction RULE line: %s", cue)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 pinned fact IDs, got %v", ids)
	}
}

func TestPinnedPreferenceCue_QueriesGovernedKindsWithCap(t *testing.T) {
	stub := &preferenceListerStub{}
	_ = PinnedPreferenceCue(context.Background(), stub, "agent-1", "user-1")
	want := []string{"preference", "constraint", "user_preference", "agent_instruction"}
	if len(stub.kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", stub.kinds, want)
	}
	for i, k := range want {
		if stub.kinds[i] != k {
			t.Fatalf("kinds = %v, want %v", stub.kinds, want)
		}
	}
	if stub.limit != pinnedPreferenceMax {
		t.Fatalf("limit = %d, want %d", stub.limit, pinnedPreferenceMax)
	}
}

func TestPinnedPreferenceCue_NilLister(t *testing.T) {
	if cue := PinnedPreferenceCue(context.Background(), nil, "a", "u"); cue != "" {
		t.Fatalf("nil lister must yield empty cue, got %q", cue)
	}
}

func TestPinnedPreferenceCue_ErrorDegrades(t *testing.T) {
	stub := &preferenceListerStub{err: errors.New("db down")}
	if cue := PinnedPreferenceCue(context.Background(), stub, "a", "u"); cue != "" {
		t.Fatalf("lister error must degrade to empty cue, got %q", cue)
	}
}

func TestPinnedPreferenceCue_TruncatesLongStatement(t *testing.T) {
	long := strings.Repeat("偏", 300)
	stub := &preferenceListerStub{rows: [][]byte{pinnedRow("f1", "preference", long)}}
	cue := PinnedPreferenceCue(context.Background(), stub, "a", "u")
	if !strings.Contains(cue, "…") {
		t.Fatalf("long statement must be truncated with ellipsis: %s", cue)
	}
	if strings.Contains(cue, long) {
		t.Fatal("full 300-rune statement must not appear verbatim")
	}
}

func TestPinnedPreferenceCue_SkipsMalformedAndEmpty(t *testing.T) {
	stub := &preferenceListerStub{rows: [][]byte{
		[]byte("{not json"),
		pinnedRow("f1", "preference", "   "),
		pinnedRow("f2", "preference", "Valid statement"),
	}}
	cue := PinnedPreferenceCue(context.Background(), stub, "a", "u")
	if !strings.Contains(cue, "Valid statement") {
		t.Fatalf("valid row must survive: %s", cue)
	}
	if strings.Count(cue, "- [") != 1 {
		t.Fatalf("malformed/empty rows must be skipped: %s", cue)
	}
}

func TestPinnedPreferenceCue_NoRows(t *testing.T) {
	stub := &preferenceListerStub{}
	if cue := PinnedPreferenceCue(context.Background(), stub, "a", "u"); cue != "" {
		t.Fatalf("no rows must yield empty cue, got %q", cue)
	}
}
