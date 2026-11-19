package approval

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bridge "aranea-agents/internal/biz/agentbridge"
)

// ============================================================================
// 测试桩
// ============================================================================

// fakeTaskReader 记录 workspaceID 入参（租户过滤断言用）并返回固定任务集。
type fakeTaskReader struct {
	mu          sync.Mutex
	gotWorkspce []string
	tasks       []*bridge.CodingTask
	total       int
}

func (f *fakeTaskReader) ListPendingReview(_ context.Context, workspaceID string, limit int) ([]*bridge.CodingTask, int, error) {
	f.mu.Lock()
	f.gotWorkspce = append(f.gotWorkspce, workspaceID)
	f.mu.Unlock()
	out := f.tasks
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, f.total, nil
}

type fakeEvolutionReader struct {
	rows  []biz.UnifiedEvolutionSuggestion
	total int
	// gotStatus 记录状态过滤（恒应为 pending）。
	gotStatus []string
	gotTarget []string
}

func (f *fakeEvolutionReader) ListByTarget(_ context.Context, targetType, _, _, status string, limit, _ int) ([]biz.UnifiedEvolutionSuggestion, error) {
	f.gotStatus = append(f.gotStatus, status)
	f.gotTarget = append(f.gotTarget, targetType)
	out := f.rows
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeEvolutionReader) CountByTarget(_ context.Context, targetType, _, _, status string) (int, error) {
	f.gotStatus = append(f.gotStatus, status)
	f.gotTarget = append(f.gotTarget, targetType)
	return f.total, nil
}

// ============================================================================
// 五身份子集零泄漏（需求文档 §5.4 验收 1）
// ============================================================================

func permViewer(userID int64, workspace string, perms ...string) Viewer {
	set := make(map[string]bool, len(perms))
	for _, p := range perms {
		set[p] = true
	}
	return Viewer{UserID: userID, WorkspaceID: workspace, has: func(code string) bool { return set[code] }}
}

func newTestAggregator(t *testing.T) (*Aggregator, *fakeTaskReader, *fakeEvolutionReader) {
	t.Helper()
	tasks := &fakeTaskReader{
		tasks: []*bridge.CodingTask{
			{ID: "ct1", Prompt: "修复登录回归", Summary: "done", AgentID: "agent-a", Status: bridge.StatusDone, CreatedAt: "2026-11-09T10:00:00Z"},
		},
		total: 1,
	}
	evo := &fakeEvolutionReader{
		rows: []biz.UnifiedEvolutionSuggestion{
			{ID: "evo1", DraftName: "优化检索", Priority: 2},
		},
		total: 1,
	}
	agg := NewAggregator(
		NewTaskReviewSource(tasks),
		NewEvolutionSuggestionSource(evo),
		NewSkillApprovalSource(evo),
	)
	return agg, tasks, evo
}

func kindsOf(items []Item) map[string]int {
	out := map[string]int{}
	for _, it := range items {
		out[it.Kind]++
	}
	return out
}

func TestAggregatorFivePersonas(t *testing.T) {
	agg, tasks, evo := newTestAggregator(t)
	ctx := context.Background()

	// sysadmin：coding.dispatch + system.self_improve → 三类全见。
	sysadmin := permViewer(1, "default", PermCodingDispatch, PermSelfImprovement)
	items, total, err := agg.List(ctx, sysadmin, "", "", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("sysadmin must see 3 gated kinds, got total=%d items=%d", total, len(items))
	}
	got := kindsOf(items)
	for _, k := range []string{KindTaskReview, KindEvolutionSuggestion, KindSkillApproval} {
		if got[k] != 1 {
			t.Fatalf("sysadmin missing kind %s: %v", k, got)
		}
	}
	sum, sumTotal, err := agg.Summary(ctx, sysadmin)
	if err != nil {
		t.Fatal(err)
	}
	if sumTotal != 3 || sum[KindTaskReview] != 1 || sum[KindEvolutionSuggestion] != 1 || sum[KindSkillApproval] != 1 {
		t.Fatalf("sysadmin summary wrong: %v total=%d", sum, sumTotal)
	}

	// ceo/user：只有 approval.review/review.* —— 三类闸门类目一个都不可见
	// （零泄漏：连计数都是 0）。
	for name, v := range map[string]Viewer{
		"ceo":  permViewer(2, "default", "approval.review", "review.view", "review.decide"),
		"user": permViewer(3, "default", "approval.review", "review.view", "review.decide", "approval.submit"),
	} {
		items, total, err := agg.List(ctx, v, "", "", 1, 50)
		if err != nil {
			t.Fatal(err)
		}
		if total != 0 || len(items) != 0 {
			t.Fatalf("%s must see zero gated items, got total=%d items=%v", name, total, items)
		}
		sum, sumTotal, err := agg.Summary(ctx, v)
		if err != nil {
			t.Fatal(err)
		}
		if sumTotal != 0 {
			t.Fatalf("%s summary must total 0, got %v", name, sum)
		}
		for k, n := range sum {
			if n != 0 {
				t.Fatalf("%s summary leaks kind %s count=%d", name, k, n)
			}
		}
	}

	// viewer 角色（无 approval.review）与零值 Viewer（无 JWT 主体）：同样全空。
	for name, v := range map[string]Viewer{
		"viewer-role": permViewer(4, "default", "ceoboard.view", "system.monitor"),
		"anonymous":   {},
	} {
		_, total, err := agg.List(ctx, v, "", "", 1, 50)
		if err != nil {
			t.Fatal(err)
		}
		if total != 0 {
			t.Fatalf("%s must see nothing, got %d", name, total)
		}
	}

	// 部分持有：只有 coding.dispatch → 只见任务验收。
	dispatcher := permViewer(5, "default", PermCodingDispatch)
	items, total, err = agg.List(ctx, dispatcher, "", "", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].Kind != KindTaskReview {
		t.Fatalf("dispatcher must see only task_review, got total=%d items=%v", total, items)
	}
	_ = tasks
	_ = evo
}

// TestAggregatorWorkspaceScoping 租户过滤在源内完成：viewer 的 WorkspaceID
// 必须透传到 reader 查询入参。
func TestAggregatorWorkspaceScoping(t *testing.T) {
	agg, tasks, _ := newTestAggregator(t)
	v := permViewer(1, "ws-42", PermCodingDispatch, PermSelfImprovement)
	if _, _, err := agg.List(context.Background(), v, KindTaskReview, "", 1, 50); err != nil {
		t.Fatal(err)
	}
	if len(tasks.gotWorkspce) == 0 || tasks.gotWorkspce[0] != "ws-42" {
		t.Fatalf("workspace not propagated: %v", tasks.gotWorkspce)
	}
}

// TestAggregatorUnknownKind 未知类别 → IsUnknownKind（service 映 400）。
func TestAggregatorUnknownKind(t *testing.T) {
	agg, _, _ := newTestAggregator(t)
	_, _, err := agg.List(context.Background(), permViewer(1, "default", PermCodingDispatch), "nope", "", 1, 50)
	if !IsUnknownKind(err) {
		t.Fatalf("expected unknown-kind error, got %v", err)
	}
	if IsUnknownKind(fmt.Errorf("wrap: %w", errors.New("other"))) {
		t.Fatal("IsUnknownKind false positive")
	}
}

// TestAggregatorMergePagination 跨源合并：CreatedAt 倒序全局排 + 同刻 Kind/ID
// 字典序稳定排 + 分页切片正确 + total=各源计数和。
func TestAggregatorMergePagination(t *testing.T) {
	tasks := &fakeTaskReader{
		tasks: []*bridge.CodingTask{
			{ID: "ct-old", Prompt: "旧任务", AgentID: "a", Status: bridge.StatusDone, CreatedAt: "2026-11-08T10:00:00Z"},
			{ID: "ct-tie", Prompt: "同刻任务", AgentID: "a", Status: bridge.StatusDone, CreatedAt: "2026-11-09T10:00:00Z"},
		},
		total: 2,
	}
	evo := &fakeEvolutionReader{
		rows: []biz.UnifiedEvolutionSuggestion{
			{ID: "evo-new", DraftName: "最新建议", CreatedAt: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)},
			{ID: "evo-tie", DraftName: "同刻建议", CreatedAt: time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)},
		},
		total: 2,
	}
	// 只注册两个源，便于精确断言。
	agg := NewAggregator(NewTaskReviewSource(tasks), NewEvolutionSuggestionSource(evo))
	v := permViewer(1, "default", PermCodingDispatch, PermSelfImprovement)

	page1, total, err := agg.List(context.Background(), v, "", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Fatalf("total must be 4, got %d", total)
	}
	if len(page1) != 2 || page1[0].ID != "evo-new" || page1[1].ID != "evo-tie" {
		// evo-new(12:00) > evo-tie(10:00, kind=evolution_suggestion) > ct-tie(10:00, kind=task_review)
		t.Fatalf("page1 order wrong: %v", idsOf(page1))
	}
	page2, _, err := agg.List(context.Background(), v, "", "", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 || page2[0].ID != "ct-tie" || page2[1].ID != "ct-old" {
		t.Fatalf("page2 order wrong: %v", idsOf(page2))
	}
	page3, _, err := agg.List(context.Background(), v, "", "", 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page3) != 0 {
		t.Fatalf("page3 must be empty, got %v", idsOf(page3))
	}
}

func idsOf(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

// TestEvolutionSourceFilters 进化源过滤语义：状态恒 pending、target 按源区分。
func TestEvolutionSourceFilters(t *testing.T) {
	_, _, evo := newTestAggregator(t)
	agg := NewAggregator(NewEvolutionSuggestionSource(evo), NewSkillApprovalSource(evo))
	v := permViewer(1, "default", PermSelfImprovement)
	if _, _, err := agg.List(context.Background(), v, "", "", 1, 50); err != nil {
		t.Fatal(err)
	}
	// List+Count 各触发一次查询 → 每 target 出现两次，断言去重集合。
	uniq := map[string]bool{}
	for _, tg := range evo.gotTarget {
		uniq[tg] = true
	}
	targets := make([]string, 0, len(uniq))
	for tg := range uniq {
		targets = append(targets, tg)
	}
	sort.Strings(targets)
	want := []string{string(biz.EvolutionTargetAgent), string(biz.EvolutionTargetSkill)}
	sort.Strings(want)
	if fmt.Sprint(targets) != fmt.Sprint(want) {
		t.Fatalf("targets = %v, want %v", targets, want)
	}
	for _, st := range evo.gotStatus {
		if st != string(biz.UnifiedEvolutionStatePending) {
			t.Fatalf("status filter must be pending, got %q", st)
		}
	}
}

// ============================================================================
// 分级审批矩阵（拍板 A-4）
// ============================================================================

func TestValidateRiskMatrixRows(t *testing.T) {
	valid := []RiskMatrixRow{{MatcherKey: "shell.exec", Risk: RiskHigh, DefaultMode: ModeAsk, AutoApproveTimes: 0}}
	if err := ValidateRiskMatrixRows(valid); err != nil {
		t.Fatalf("valid rows must pass, got %v", err)
	}
	cases := []struct {
		name string
		rows []RiskMatrixRow
	}{
		{"empty key", []RiskMatrixRow{{MatcherKey: "  ", Risk: RiskLow, DefaultMode: ModeAuto}}},
		{"dup key", []RiskMatrixRow{{MatcherKey: "a", Risk: RiskLow, DefaultMode: ModeAuto}, {MatcherKey: "a", Risk: RiskHigh, DefaultMode: ModeAsk}}},
		{"bad risk", []RiskMatrixRow{{MatcherKey: "a", Risk: "extreme", DefaultMode: ModeAsk}}},
		{"bad mode", []RiskMatrixRow{{MatcherKey: "a", Risk: RiskLow, DefaultMode: "yolo"}}},
		{"negative times", []RiskMatrixRow{{MatcherKey: "a", Risk: RiskLow, DefaultMode: ModeAsk, AutoApproveTimes: -1}}},
	}
	for _, tc := range cases {
		if err := ValidateRiskMatrixRows(tc.rows); err == nil {
			t.Fatalf("%s: expected validation error", tc.name)
		}
	}
}

type fakeMatrixRepo struct {
	putRows      []RiskMatrixRow
	putUpdatedBy string
	listRows     []RiskMatrixRow
}

func (f *fakeMatrixRepo) ListRiskMatrix(context.Context) ([]RiskMatrixRow, error) {
	return f.listRows, nil
}

func (f *fakeMatrixRepo) ReplaceRiskMatrix(_ context.Context, rows []RiskMatrixRow, updatedBy string) error {
	f.putRows = rows
	f.putUpdatedBy = updatedBy
	return nil
}

type fakeAuditSink struct {
	actor string
	rows  []RiskMatrixRow
	calls int
}

func (f *fakeAuditSink) RecordMatrixChange(_ context.Context, actor string, rows []RiskMatrixRow) error {
	f.calls++
	f.actor = actor
	f.rows = rows
	return nil
}

// TestMatrixPutAudited 矩阵变更留审计（验收 6）：actor=JWT 主体 id 字符串，
// 全量行集透传，仓储 updatedBy 同源。
func TestMatrixPutAudited(t *testing.T) {
	repo := &fakeMatrixRepo{}
	sink := &fakeAuditSink{}
	m := NewMatrix(repo, sink)
	rows := []RiskMatrixRow{{MatcherKey: "shell.exec", Risk: RiskHigh, DefaultMode: ModeAsk}}
	v := permViewer(42, "default", "approval.policy")
	if err := m.Put(context.Background(), v, rows); err != nil {
		t.Fatal(err)
	}
	if repo.putUpdatedBy != "42" {
		t.Fatalf("updatedBy = %q, want 42", repo.putUpdatedBy)
	}
	if len(repo.putRows) != 1 || repo.putRows[0].MatcherKey != "shell.exec" {
		t.Fatalf("repo rows = %+v", repo.putRows)
	}
	if sink.calls != 1 || sink.actor != "42" || len(sink.rows) != 1 {
		t.Fatalf("audit not recorded: %+v", sink)
	}

	// 校验失败：仓储与审计均不得触达。
	bad := []RiskMatrixRow{{MatcherKey: "", Risk: RiskLow, DefaultMode: ModeAsk}}
	if err := m.Put(context.Background(), v, bad); err == nil {
		t.Fatal("invalid rows must fail")
	}
	if len(repo.putRows) != 1 || sink.calls != 1 {
		t.Fatal("invalid put must not touch repo/audit")
	}
}
