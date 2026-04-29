// Evolution 扫描器（与 AgentEvolutionService 同包）。
package application

import (
	mem "arenea/backend/internal/memory/domain"

	"context"
	"fmt"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// scanWindow 为尚无 `last_scan_at` 时的默认回溯窗口。足够长以收集
// 有统计意义的遥测，又足够短使逐渐失效的失败最终被滤出。
const scanWindow = 30 * 24 * time.Hour

// scanMinInvocations 为每工具样本下限，低于则*不*产生任何启发式提案。
const scanMinInvocations = 5

// scanFailureThreshold 与 scanSuccessThreshold 为触发黑名单/提偏好提案的启发式比率。刻意保守，工作线程宁静默不噪。
const (
	scanFailureThreshold = 0.30
	scanSuccessThreshold = 0.85
)

// 按 §13 的回滚告警调参：
//
//   "回滚率 > 20% 后，evo_auto_apply 自动转为 0 并发出告警"
//
// 另需最小样本量，避免单次不稳定提案误触刹车。
const (
	rollbackAlarmThreshold   = 0.20
	rollbackAlarmMinEvents   = 5
	rollbackAlarmWindowHours = 24 * 30
)

// statsLastScanAtKey 为 AgentStrategyProfile.Stats 中记录某智能体上次扫描完成时间的键。*非*正式进化事件——仅簿记。
const statsLastScanAtKey = "last_scan_at"

// statsLastScanReportKey 快照智能体最近 ScanReport，主要供 UI 查看/调试。
const statsLastScanReportKey = "last_scan_report"

// negativeFeedbackTypes 列出扫描器视为「用户反对」的 FactFeedback。确认/已用/未用排除——仅含暗示智能体有误的信号。
var negativeFeedbackTypes = []string{
	mem.FactFeedbackReject,
	mem.FactFeedbackRefine,
}

// AggregateSkillStats 拉取自 `since` 起 `agentID` 的 tool_invocations，
// 按 tool_key 分组，每工具 upsert 一行 `agent_skill_stats`。返回已 upsert 切片供调用方（扫描器）立即使用而无需再读。空 `since` 默认为 `now-scanWindow`。
func (s *AgentEvolutionService) AggregateSkillStats(ctx context.Context, agentID, since string) ([]domain.AgentSkillStat, error) {
	if agentID == "" {
		return nil, validationError("agent id is required")
	}
	if strings.TrimSpace(since) == "" {
		since = time.Now().UTC().Add(-scanWindow).Format(time.RFC3339)
	}

	runs, err := s.repo.SearchToolInvocations(domain.ToolRunQuery{
		AgentID: agentID,
		From:    since,
		Limit:   1000,
	})
	if err != nil {
		return nil, err
	}

	type acc struct {
		invocations  int
		successes    int
		failures     int
		userOverride int
		latencyTotal float64
		tokenTotal   float64
		lastUsedAt   string
	}
	bucket := map[string]*acc{}
	for _, r := range runs.Items {
		if r.ToolKey == "" {
			continue
		}
		a, ok := bucket[r.ToolKey]
		if !ok {
			a = &acc{}
			bucket[r.ToolKey] = a
		}
		a.invocations++
		switch r.Status {
		case "success":
			a.successes++
		case "error", "failure":
			a.failures++
		case "blocked":
			a.userOverride++
		}
		a.latencyTotal += float64(r.DurationMS)
		// 用输出预览长度/4 近似 token 成本——排序足够；精确核算在聊天用量管线，非扫描器。
		a.tokenTotal += float64(len(r.OutputPreview)) / 4
		if r.StartedAt > a.lastUsedAt {
			a.lastUsedAt = r.StartedAt
		}
	}

	out := make([]domain.AgentSkillStat, 0, len(bucket))
	for toolKey, a := range bucket {
		stat := domain.AgentSkillStat{
			AgentID:         agentID,
			Scope:           "overall",
			ScopeValue:      "",
			ToolKey:         toolKey,
			Invocations:     a.invocations,
			Successes:       a.successes,
			Failures:        a.failures,
			UserOverrides:   a.userOverride,
			AvgLatencyMS:    safeAvg(a.latencyTotal, a.invocations),
			AvgTokens:       safeAvg(a.tokenTotal, a.invocations),
			PreferenceScore: skillPreferenceScore(a.successes, a.failures, a.invocations),
			LastUsedAt:      a.lastUsedAt,
			Metadata: map[string]any{
				"window_since": since,
				"computed_at":  s.now(),
			},
		}
		stored, err := s.repo.UpsertAgentSkillStat(stat)
		if err != nil {
			return out, err
		}
		out = append(out, stored)
	}
	return out, nil
}

// RunEvolutionScan 实现 §5.5。按遥测刷新各工具技能统计，评估 §13
// 回滚率安全刹车，在 `evo_enabled` 及活跃量/负向反馈
// 触发条件满足时放行，每则明确信号产出一个 ProposalInput。当
// `evo_auto_apply=true` 且提案为 `low` 风险时也会自动
// 批准并应用。
//
// 节流由 `Propose` 处理，在节流窗口内重跑扫描是安全的（新提案会变为 `superseded`）。
func (s *AgentEvolutionService) RunEvolutionScan(ctx context.Context, agentID string) (ScanReport, error) {
	if agentID == "" {
		return ScanReport{}, validationError("agent id is required")
	}
	scanStart := time.Now().UTC()
	settings, _ := s.repo.GetAgentRuntimeSettings(agentID)
	if !settings.EvoEnabled {
		return ScanReport{Note: "evo_enabled=false"}, nil
	}

	current, err := s.GetStrategy(ctx, agentID)
	if err != nil {
		return ScanReport{}, err
	}

	// §13 回滚率刹车 — 在产生新提案*之前*执行，
	// 防止异常自动应用流程对已隔离智能体再叠加新损害。
	if disabled, rate, total := s.evaluateRollbackAlarm(ctx, agentID, settings); disabled {
		// 重载设置使后续步骤看到 evo_auto_apply=false，
		// 即使本轮本就不会自动应用。
		settings.EvoAutoApply = false
		_ = s.audit("agent.evolution.scanner.rollback_alarm",
			"agent_runtime_settings", agentID, map[string]any{
				"rollback_rate": rate,
				"event_total":   total,
				"threshold":     rollbackAlarmThreshold,
				"action":        "evo_auto_apply=false",
			})
	}

	// §5.5 第 2 步 — episode 与反馈使用增量的
	// `last_scan_at` 窗口，仅统计*自上次扫描以来*。相对地，`agent_skill_stats`
	// 聚合始终为滚动 30 天窗口，使连续扫描间趋势稳定（见规范：「skill_stats = AgentSkillStat 最近聚合」）。
	triggerSince := s.scanWindowSince(current, scanStart)
	aggregationSince := scanStart.Add(-scanWindow).Format(time.RFC3339)

	episodes, err := s.repo.CountAgentEpisodesSince(agentID, triggerSince)
	if err != nil {
		// Episode 计数仅作参考；此处失败不得阻塞
		// 其余扫描，因仅凭工具遥测即可驱动提案。
		episodes = 0
	}
	negFeedback, _ := s.repo.CountAgentFactFeedbackSince(agentID, negativeFeedbackTypes, triggerSince)

	stats, err := s.AggregateSkillStats(ctx, agentID, aggregationSince)
	if err != nil {
		return ScanReport{Errors: 1, EpisodesScanned: episodes}, err
	}

	minEpisodes := settings.EvoMinEpisodes
	if minEpisodes <= 0 {
		minEpisodes = 20
	}
	minNegFeedback := settings.EvoMinNegativeFeedback
	if minNegFeedback <= 0 {
		minNegFeedback = 3
	}
	hasFailingTool := false
	for _, st := range stats {
		if st.Invocations >= scanMinInvocations && failureRate(st) > scanFailureThreshold {
			hasFailingTool = true
			break
		}
	}
	triggered := episodes >= minEpisodes ||
		hasFailingTool ||
		negFeedback >= minNegFeedback

	if !triggered {
		report := ScanReport{
			EpisodesScanned: episodes,
			Note:            "trigger conditions not met",
		}
		s.persistScanCheckpoint(ctx, agentID, scanStart, report)
		return report, nil
	}

	report := ScanReport{EpisodesScanned: episodes}
	currentBlacklist := map[string]bool{}
	for _, k := range current.ToolBlacklist {
		currentBlacklist[k] = true
	}

	for _, st := range stats {
		if st.Invocations < scanMinInvocations {
			continue
		}
		if failureRate(st) > scanFailureThreshold && !currentBlacklist[st.ToolKey] {
			next := append([]string(nil), current.ToolBlacklist...)
			next = append(next, st.ToolKey)
			prop, err := s.Propose(ctx, ProposalInput{
				AgentID:        agentID,
				Kind:           domain.EvoKindToolDisable,
				TargetField:    "strategy.tool_blacklist",
				ProposedValue:  next,
				CurrentValue:   current.ToolBlacklist,
				Rationale:      fmt.Sprintf("auto-scan: tool %s failure_rate=%.2f over %d invocations", st.ToolKey, failureRate(st), st.Invocations),
				ExpectedImpact: "Reduce repeated failures by removing the tool from the agent's whitelist.",
				RiskLevel:      domain.EvoRiskLow,
				Source:         domain.EvoSourceRuntimeSignal,
			})
			if err != nil {
				report.Errors++
				continue
			}
			s.handleScanProposalLifecycle(ctx, settings, prop, &report)
		}

		if successRate(st) > scanSuccessThreshold {
			currentPref := current.ToolPreference[st.ToolKey]
			if currentPref >= 0.7 {
				continue
			}
			merged := map[string]float64{}
			for k, v := range current.ToolPreference {
				merged[k] = v
			}
			merged[st.ToolKey] = 0.8
			prop, err := s.Propose(ctx, ProposalInput{
				AgentID:        agentID,
				Kind:           domain.EvoKindToolPrefUpdate,
				TargetField:    "strategy.tool_preference",
				ProposedValue:  merged,
				CurrentValue:   current.ToolPreference,
				Rationale:      fmt.Sprintf("auto-scan: tool %s success_rate=%.2f over %d invocations", st.ToolKey, successRate(st), st.Invocations),
				ExpectedImpact: "Promote a reliably-used tool so it ranks higher in the prompt's tool list.",
				RiskLevel:      domain.EvoRiskLow,
				Source:         domain.EvoSourceRuntimeSignal,
			})
			if err != nil {
				report.Errors++
				continue
			}
			s.handleScanProposalLifecycle(ctx, settings, prop, &report)
		}
	}

	s.persistScanCheckpoint(ctx, agentID, scanStart, report)
	return report, nil
}

// scanWindowSince 取扫描窗口下界。优先 `strategy.stats` 中
// 存储的 `last_scan_at` 检查点；否则回退到 `now - scanWindow`。早于
// `scanWindow` 的检查点也会钳位，避免长期闲置智能体
// 重新启用后首次扫描突然聚合整年噪声数据。
func (s *AgentEvolutionService) scanWindowSince(strat domain.AgentStrategyProfile, now time.Time) string {
	fallback := now.Add(-scanWindow)
	if strat.Stats == nil {
		return fallback.Format(time.RFC3339)
	}
	raw, ok := strat.Stats[statsLastScanAtKey].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback.Format(time.RFC3339)
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fallback.Format(time.RFC3339)
	}
	if parsed.Before(fallback) {
		return fallback.Format(time.RFC3339)
	}
	return parsed.UTC().Format(time.RFC3339)
}

// persistScanCheckpoint 将 `last_scan_at` 与最近 ScanReport 的小快照
// 写入 `strategy.stats`。先重读当前策略，使同次扫描中已自动应用的提案
// 在簿记写入后仍保留（否则会用手动缓存的旧快照盖掉
// 刚写上的新黑名单）。此处失败仅记日志 — 扫描本身已成功。
func (s *AgentEvolutionService) persistScanCheckpoint(ctx context.Context, agentID string, scanStart time.Time, report ScanReport) {
	live, err := s.repo.GetAgentStrategyProfile(agentID)
	if err != nil {
		return
	}
	if live.Stats == nil {
		live.Stats = map[string]any{}
	}
	live.Stats[statsLastScanAtKey] = scanStart.Format(time.RFC3339)
	live.Stats[statsLastScanReportKey] = map[string]any{
		"episodes_scanned":    report.EpisodesScanned,
		"new_proposals":       report.NewProposals,
		"auto_applied":        report.AutoApplied,
		"throttled_proposals": report.ThrottledProposals,
		"errors":              report.Errors,
		"note":                report.Note,
		"recorded_at":         scanStart.Format(time.RFC3339),
	}
	_, _ = s.repo.UpsertAgentStrategyProfile(live)
}

// evaluateRollbackAlarm 检查 §13 刹车。当率超过
// `rollbackAlarmThreshold` 且智能体当前为自动应用配置时返回 `disabled=true`。
// 本函数就地将运行时设置标志翻转，使后续调用不会因同一阈限
// 突破再次触发审计日志。
func (s *AgentEvolutionService) evaluateRollbackAlarm(ctx context.Context, agentID string, settings domain.AgentRuntimeSettings) (bool, float64, int) {
	if !settings.EvoAutoApply {
		return false, 0, 0
	}
	since := time.Now().UTC().Add(-rollbackAlarmWindowHours * time.Hour).Format(time.RFC3339)
	events, _, err := s.repo.ListEvolutionEvents(repository.EvolutionEventQuery{
		AgentID: agentID,
		Limit:   500,
	})
	if err != nil {
		return false, 0, 0
	}
	total, reverted := 0, 0
	for _, ev := range events {
		if ev.CreatedAt < since {
			continue
		}
		if ev.Kind == domain.EvoKindRollback {
			// 回滚是先前被撤销之事件的结果；若与撤销都计
			// 会使率被重复加算。
			continue
		}
		total++
		if ev.Reverted {
			reverted++
		}
	}
	if total < rollbackAlarmMinEvents {
		return false, 0, total
	}
	rate := float64(reverted) / float64(total)
	if rate <= rollbackAlarmThreshold {
		return false, rate, total
	}
	settings.EvoAutoApply = false
	if _, err := s.repo.UpsertAgentRuntimeSettings(settings); err != nil {
		return false, rate, total
	}
	return true, rate, total
}

// handleScanProposalLifecycle 根据提案状态更新对应
// ScanReport 计数。当 `evo_auto_apply` 开启且提案为低风险的 pending 时自动批准。
func (s *AgentEvolutionService) handleScanProposalLifecycle(ctx context.Context, settings domain.AgentRuntimeSettings, prop domain.EvolutionProposal, report *ScanReport) {
	switch prop.Status {
	case domain.EvoProposalSuperseded:
		report.ThrottledProposals++
		return
	case domain.EvoProposalPending:
		report.NewProposals++
	default:
		report.NewProposals++
	}
	if !settings.EvoAutoApply || prop.RiskLevel != domain.EvoRiskLow || prop.Status != domain.EvoProposalPending {
		return
	}
	if _, err := s.Approve(ctx, prop.ID, "auto_scanner"); err != nil {
		report.Errors++
		return
	}
	report.AutoApplied++
}

func failureRate(s domain.AgentSkillStat) float64 {
	if s.Invocations <= 0 {
		return 0
	}
	return float64(s.Failures) / float64(s.Invocations)
}

func successRate(s domain.AgentSkillStat) float64 {
	if s.Invocations <= 0 {
		return 0
	}
	return float64(s.Successes) / float64(s.Invocations)
}

func safeAvg(total float64, n int) float64 {
	if n <= 0 {
		return 0
	}
	return total / float64(n)
}

// skillPreferenceScore 将 {成功, 失败, 总计} 映射为
// [0,1] 偏好分并轻度平滑，使新工具上单次失败不会把偏好拉穿。与 §3.2.5
// 中 "preference_score REAL DEFAULT 0.5" 基线一致。
func skillPreferenceScore(successes, failures, total int) float64 {
	if total <= 0 {
		return 0.5
	}
	const prior = 2.0
	num := float64(successes) + 0.5*prior
	den := float64(total) + prior
	score := num/den - 0.3*float64(failures)/(float64(total)+1)
	// 下界取 0.01 而非 0，因 upsert 路径将 0 视为
	//「未设」并回退 0.5，否则会把真正
	// 很差的分数覆盖掉。
	if score < 0.01 {
		return 0.01
	}
	if score > 1 {
		return 1
	}
	return score
}
