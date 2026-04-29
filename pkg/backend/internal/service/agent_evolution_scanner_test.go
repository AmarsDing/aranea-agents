// agent_evolution_scanner_test.go 覆盖属于第四阶段 EvolutionWorker 的 §13 验收项：
//
//   - AggregateSkillStats 将原始 tool_invocations 转为带合理偏好分的 agent_skill_stats 行。
//   - RunEvolutionScan 遵守 evo_enabled，在活跃量触发条件下放行，否则当遥测显示
//     明确信号（failure_rate > 0.3 或 success_rate > 0.85）时至少产出一则提案。
//   - evo_auto_apply=true 时低风险提案直接经 Approve+Apply。
//   - 在节流窗口内重跑扫描则新提案为 `superseded` 而非重复。
package service

import (
	mem "arenea/backend/internal/memory/domain"

	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// newScannerHarness 为扫描器测试接内存 SQLite + AgentEvolutionService，并提供写入 tool_invocations 的辅助函数。
func newScannerHarness(t *testing.T) (*AgentEvolutionService, repository.Store, func(domain.ToolInvocation)) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "scanner.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewAgentEvolutionService(repo)
	insert := func(ti domain.ToolInvocation) {
		t.Helper()
		if _, err := repo.InsertToolInvocation(ti); err != nil {
			t.Fatalf("insert tool invocation: %v", err)
		}
	}
	return svc, repo, insert
}

func TestScannerAggregateSkillStatsBuildsRowsFromInvocations(t *testing.T) {
	svc, _, insert := newScannerHarness(t)
	now := time.Now().UTC()
	for i := 0; i < 8; i++ {
		insert(domain.ToolInvocation{
			AgentID:    "agent-A",
			ToolKey:    "shell",
			Status:     "success",
			DurationMS: 120 + i,
			StartedAt:  now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
		})
	}
	for i := 0; i < 4; i++ {
		insert(domain.ToolInvocation{
			AgentID:    "agent-A",
			ToolKey:    "browser",
			Status:     "error",
			DurationMS: 200 + i,
			StartedAt:  now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
		})
	}

	stats, err := svc.AggregateSkillStats(context.Background(), "agent-A", "")
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	byTool := map[string]domain.AgentSkillStat{}
	for _, s := range stats {
		byTool[s.ToolKey] = s
	}
	shell, ok := byTool["shell"]
	if !ok {
		t.Fatalf("expected shell stat, got %v", byTool)
	}
	if shell.Invocations != 8 || shell.Successes != 8 || shell.Failures != 0 {
		t.Fatalf("shell stat unexpected: %#v", shell)
	}
	if shell.PreferenceScore <= 0.7 {
		t.Fatalf("shell preference too low: %v", shell.PreferenceScore)
	}
	browser, ok := byTool["browser"]
	if !ok {
		t.Fatalf("expected browser stat, got %v", byTool)
	}
	if browser.Failures != 4 || browser.Successes != 0 {
		t.Fatalf("browser stat unexpected: %#v", browser)
	}
	if browser.PreferenceScore >= 0.5 {
		t.Fatalf("browser preference should be low after only-failure runs: %v", browser.PreferenceScore)
	}
}

func TestScannerRunEvolutionScanRespectsEvoDisabled(t *testing.T) {
	svc, _, _ := newScannerHarness(t)
	report, err := svc.RunEvolutionScan(context.Background(), "agent-disabled")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if report.Note != "evo_enabled=false" {
		t.Fatalf("expected disabled note, got %#v", report)
	}
}

func TestScannerRunEvolutionScanGatesWhenNoTriggers(t *testing.T) {
	svc, repo, insert := newScannerHarness(t)
	enableEvo(t, repo, "agent-quiet", false)
	insert(domain.ToolInvocation{
		AgentID:   "agent-quiet",
		ToolKey:   "shell",
		Status:    "success",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	})
	report, err := svc.RunEvolutionScan(context.Background(), "agent-quiet")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if report.NewProposals != 0 {
		t.Fatalf("expected zero proposals, got %#v", report)
	}
	if report.Note != "trigger conditions not met" {
		t.Fatalf("expected gating note, got %#v", report)
	}
}

func TestScannerRunEvolutionScanProposesBlacklistOnFailingTool(t *testing.T) {
	svc, repo, insert := newScannerHarness(t)
	enableEvo(t, repo, "agent-flaky", false)
	for i := 0; i < 7; i++ {
		insert(domain.ToolInvocation{
			AgentID:   "agent-flaky",
			ToolKey:   "browser",
			Status:    "error",
			StartedAt: time.Now().UTC().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
	}
	for i := 0; i < 2; i++ {
		insert(domain.ToolInvocation{
			AgentID:   "agent-flaky",
			ToolKey:   "browser",
			Status:    "success",
			StartedAt: time.Now().UTC().Add(-time.Duration(i+10) * time.Minute).Format(time.RFC3339),
		})
	}

	report, err := svc.RunEvolutionScan(context.Background(), "agent-flaky")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if report.NewProposals == 0 {
		t.Fatalf("expected at least one proposal, got %#v", report)
	}
	props, _, err := repo.ListEvolutionProposals(repository.EvolutionProposalQuery{
		AgentID: "agent-flaky", Limit: 10,
	})
	if err != nil {
		t.Fatalf("list proposals: %v", err)
	}
	found := false
	for _, p := range props {
		if p.TargetField == "strategy.tool_blacklist" && p.Status == domain.EvoProposalPending {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pending blacklist proposal, got %#v", props)
	}
}

func TestScannerAutoApplyAppliesLowRiskProposals(t *testing.T) {
	svc, repo, insert := newScannerHarness(t)
	enableEvo(t, repo, "agent-auto", true)
	for i := 0; i < 8; i++ {
		insert(domain.ToolInvocation{
			AgentID:   "agent-auto",
			ToolKey:   "browser",
			Status:    "error",
			StartedAt: time.Now().UTC().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
	}
	report, err := svc.RunEvolutionScan(context.Background(), "agent-auto")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if report.AutoApplied == 0 {
		t.Fatalf("expected auto-applied proposal, got %#v", report)
	}
	strat, err := svc.GetStrategy(context.Background(), "agent-auto")
	if err != nil {
		t.Fatalf("get strategy: %v", err)
	}
	contains := false
	for _, k := range strat.ToolBlacklist {
		if k == "browser" {
			contains = true
		}
	}
	if !contains {
		t.Fatalf("expected browser in blacklist after auto-apply, got %v", strat.ToolBlacklist)
	}
}

func TestScannerThrottlesDuplicateRuns(t *testing.T) {
	svc, repo, insert := newScannerHarness(t)
	enableEvo(t, repo, "agent-throttle", false)
	for i := 0; i < 7; i++ {
		insert(domain.ToolInvocation{
			AgentID:   "agent-throttle",
			ToolKey:   "browser",
			Status:    "error",
			StartedAt: time.Now().UTC().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
	}
	first, err := svc.RunEvolutionScan(context.Background(), "agent-throttle")
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if first.NewProposals == 0 {
		t.Fatalf("first scan should emit proposals, got %#v", first)
	}
	second, err := svc.RunEvolutionScan(context.Background(), "agent-throttle")
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if second.ThrottledProposals == 0 {
		t.Fatalf("second scan should record throttled proposals, got %#v", second)
	}
}

func enableEvo(t *testing.T, repo repository.Store, agentID string, autoApply bool) {
	t.Helper()
	settings, _ := repo.GetAgentRuntimeSettings(agentID)
	settings.AgentID = agentID
	settings.EvoEnabled = true
	settings.EvoAutoApply = autoApply
	settings.EvoMinEpisodes = 100
	settings.EvoMinNegativeFeedback = 3
	settings.EvoThrottleHours = 24
	if _, err := repo.UpsertAgentRuntimeSettings(settings); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
}

// TestScannerPersistsLastScanAtCheckpoint — 成功扫描后策略行带 `stats.last_scan_at`，
// 下次扫描自该点继续。覆盖 §5.5「since=last_scan_at」交接。
func TestScannerPersistsLastScanAtCheckpoint(t *testing.T) {
	svc, repo, insert := newScannerHarness(t)
	enableEvo(t, repo, "agent-checkpoint", false)
	for i := 0; i < 7; i++ {
		insert(domain.ToolInvocation{
			AgentID:   "agent-checkpoint",
			ToolKey:   "browser",
			Status:    "error",
			StartedAt: time.Now().UTC().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
	}
	if _, err := svc.RunEvolutionScan(context.Background(), "agent-checkpoint"); err != nil {
		t.Fatalf("scan: %v", err)
	}
	strat, err := svc.GetStrategy(context.Background(), "agent-checkpoint")
	if err != nil {
		t.Fatalf("get strategy: %v", err)
	}
	rawAt, _ := strat.Stats["last_scan_at"].(string)
	if rawAt == "" {
		t.Fatalf("expected last_scan_at, got %#v", strat.Stats)
	}
	if _, err := time.Parse(time.RFC3339, rawAt); err != nil {
		t.Fatalf("last_scan_at not RFC3339: %v", err)
	}
	report, ok := strat.Stats["last_scan_report"].(map[string]any)
	if !ok {
		t.Fatalf("expected last_scan_report map, got %T", strat.Stats["last_scan_report"])
	}
	if _, ok := report["new_proposals"]; !ok {
		t.Fatalf("expected new_proposals in report snapshot, got %#v", report)
	}
}

// TestScannerLastScanAtNarrowsTriggerWindow — 检查点之后立即第二次扫描无新 episode/反馈，
// 应落入触发门限之外，即使仍有历史失败工具（因*触发*窗口内技能统计无新失败）。
// 确认 `triggerSince` 与 `aggregationSince` 解耦：扫描器仍按满 30 天聚合技能统计
//（故 `hasFailingTool` 仍可因旧数据为真），但 episode 与反馈计数仅来自检查点之后。
func TestScannerLastScanAtNarrowsTriggerWindow(t *testing.T) {
	svc, repo, _ := newScannerHarness(t)
	enableEvo(t, repo, "agent-narrow", false)
	// 种入较新的 `last_scan_at`：仅统计检查点后的 episode/反馈为空，技能统计仍按 30 天聚合。
	strat, _ := svc.GetStrategy(context.Background(), "agent-narrow")
	if strat.Stats == nil {
		strat.Stats = map[string]any{}
	}
	strat.Stats["last_scan_at"] = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if _, err := repo.UpsertAgentStrategyProfile(strat); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	report, err := svc.RunEvolutionScan(context.Background(), "agent-narrow")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if report.Note != "trigger conditions not met" {
		t.Fatalf("expected trigger gating after fresh checkpoint, got %#v", report)
	}
}

// TestScannerNegativeFeedbackTriggersScan — 累积 `EvoMinNegativeFeedback` 条 reject/refine 后
// 即进入提案循环，即便无失败工具或足够 episode。覆盖 §5.5 第 3 步负反馈分支。
func TestScannerNegativeFeedbackTriggersScan(t *testing.T) {
	svc, repo, _ := newScannerHarness(t)
	enableEvo(t, repo, "agent-rejected", false)
	for i := 0; i < 4; i++ {
		_, err := repo.InsertFactFeedback(mem.FactFeedback{
			ID:        fmt.Sprintf("fb-%d", i),
			FactID:    fmt.Sprintf("fact-%d", i),
			AgentID:   "agent-rejected",
			Type:      mem.FactFeedbackReject,
			Source:    "user",
			CreatedAt: time.Now().UTC().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
		if err != nil {
			t.Fatalf("insert feedback: %v", err)
		}
	}
	report, err := svc.RunEvolutionScan(context.Background(), "agent-rejected")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if report.Note == "trigger conditions not met" {
		t.Fatalf("expected scan to be triggered by negative feedback, got %#v", report)
	}
}

// TestScannerRollbackAlarmDisablesAutoApply — 智能体近期 EvolutionEvents 中撤销超过 20% 时
// 扫描器将 `evo_auto_apply` 关并记审计。覆盖 §13 刹车。
func TestScannerRollbackAlarmDisablesAutoApply(t *testing.T) {
	svc, repo, insert := newScannerHarness(t)
	enableEvo(t, repo, "agent-quarantine", true)

	// 种 5 条已应用事件；2 条已撤销 (40%) — 远高于 20% 门限。
	for i := 0; i < 5; i++ {
		ev := domain.EvolutionEvent{
			ID:          fmt.Sprintf("evt-%d", i),
			AgentID:     "agent-quarantine",
			Kind:        domain.EvoKindToolDisable,
			TargetField: "strategy.tool_blacklist",
			TriggerKind: domain.EvoTriggerAuto,
			Applied:     true,
			Reverted:    i < 2,
			CreatedAt:   time.Now().UTC().Add(-time.Duration(i+1) * time.Hour).Format(time.RFC3339),
		}
		if _, err := repo.InsertEvolutionEvent(ev); err != nil {
			t.Fatalf("insert event: %v", err)
		}
	}
	// 种足够失败工具遥测使扫描仍进提案环，以验证无自动应用。
	for i := 0; i < 7; i++ {
		insert(domain.ToolInvocation{
			AgentID:   "agent-quarantine",
			ToolKey:   "shell",
			Status:    "error",
			StartedAt: time.Now().UTC().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
	}

	report, err := svc.RunEvolutionScan(context.Background(), "agent-quarantine")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if report.AutoApplied != 0 {
		t.Fatalf("expected zero auto-applied after alarm trip, got %#v", report)
	}
	settings, _ := repo.GetAgentRuntimeSettings("agent-quarantine")
	if settings.EvoAutoApply {
		t.Fatalf("expected evo_auto_apply=false after alarm, got true")
	}
}

// TestScannerRollbackAlarmRequiresMinEvents — 小样本 100% 撤销率不得触发刹车；需 ≥ 5 条事件。
func TestScannerRollbackAlarmRequiresMinEvents(t *testing.T) {
	svc, repo, _ := newScannerHarness(t)
	enableEvo(t, repo, "agent-small-sample", true)
	for i := 0; i < 2; i++ {
		ev := domain.EvolutionEvent{
			ID:          fmt.Sprintf("evt-small-%d", i),
			AgentID:     "agent-small-sample",
			Kind:        domain.EvoKindToolDisable,
			TargetField: "strategy.tool_blacklist",
			TriggerKind: domain.EvoTriggerAuto,
			Applied:     true,
			Reverted:    true,
			CreatedAt:   time.Now().UTC().Add(-time.Duration(i+1) * time.Hour).Format(time.RFC3339),
		}
		if _, err := repo.InsertEvolutionEvent(ev); err != nil {
			t.Fatalf("insert event: %v", err)
		}
	}
	if _, err := svc.RunEvolutionScan(context.Background(), "agent-small-sample"); err != nil {
		t.Fatalf("scan: %v", err)
	}
	settings, _ := repo.GetAgentRuntimeSettings("agent-small-sample")
	if !settings.EvoAutoApply {
		t.Fatalf("expected evo_auto_apply=true under min-events threshold, got false")
	}
}
