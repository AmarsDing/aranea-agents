package service

import (
	"context"
	"sort"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/encoding/protojson"
)

// team_stats.go — 79-runtime-governance R7：run stats API + JSONL 导出。
// 全部数据来自既有记账点（model_token_usage_events / team_run_steps /
// decision_records system_guard），本文件只做装配，不落新表。任一统计源
// 降级（无数据/窄能力缺失）按零值透出，不让 stats 端点因旁路缺失 500。

// unknownMemberKey 是空 agent_key 用量行的合流桶（记账缺陷不静默吞账，
// 见 data.usageRepo.RunMemberUsageStats 注释）。
const unknownMemberKey = "unknown"

// buildRunStats 聚合单 run 的 stats。steps 由调用方提供（导出路径批量
// 复用同一次查询）。usage/decision 源的读取错误降级为零值 + warn 日志。
func (s *TeamService) buildRunStats(ctx context.Context, run biz.TeamRunRecord, steps []biz.TeamRunStep) *v1.TeamRunStats {
	stats := &v1.TeamRunStats{
		RunId:     run.ID,
		TeamId:    run.TeamID,
		SessionId: run.SessionID,
		Status:    run.Status,
		CreatedAt: run.CreatedAt,
		Turns:     int32(len(steps)),
	}
	memberSteps := make(map[string]int32, len(steps))
	for _, st := range steps {
		stats.ToolCalls += int32(st.ToolCallCount)
		key := st.AgentKey
		if key == "" {
			key = unknownMemberKey
		}
		memberSteps[key]++
	}

	if s.usageUC != nil {
		if hit, err := s.usageUC.RunCacheHitRatio(ctx, run.ID); err != nil {
			s.lg.Warn("run stats: cache hit ratio degraded", loggateway.Err(err), loggateway.Str("run_id", run.ID))
		} else if hit.Found {
			stats.PromptTokens = hit.PromptTok
			stats.CompletionTokens = hit.CompletionTok
			stats.CachedTokens = hit.CachedTok
			stats.CacheHitRatio = hit.Ratio
		}
		if peak, err := s.usageUC.RunTurnPeak(ctx, run.ID); err != nil {
			s.lg.Warn("run stats: turn peak degraded", loggateway.Err(err), loggateway.Str("run_id", run.ID))
		} else if peak.Found {
			stats.MaxTurnInputTokens = peak.MaxInputTokens
		}
		if members, err := s.usageUC.RunMemberUsageStats(ctx, run.ID); err != nil {
			// 用量源故障回落到 step 维度成员行（P5.1 M3）——members 段不应
			// 因旁路读取失败整体消失（与 usageUC 未装配的降级语义一致）。
			s.lg.Warn("run stats: member usage degraded, fallback to step rows", loggateway.Err(err), loggateway.Str("run_id", run.ID))
			appendStepOnlyMembers(stats, memberSteps, nil)
		} else {
			seen := make(map[string]bool, len(members))
			for _, m := range members {
				key := m.AgentKey
				if key == "" {
					key = unknownMemberKey
				}
				seen[key] = true
				stats.Members = append(stats.Members, &v1.TeamRunMemberStats{
					AgentKey:         key,
					PromptTokens:     m.PromptTok,
					CompletionTokens: m.CompletionTok,
					CachedTokens:     m.CachedTok,
					Calls:            int32(m.Calls),
					Steps:            memberSteps[key],
				})
			}
			// 有 step 但无用量行的成员（如全缓存/记账缺失）也要列出。
			appendStepOnlyMembers(stats, memberSteps, seen)
		}
	} else {
		// 无 usage 源（单测/精简装配）：仍透出 step 维度成员行。
		appendStepOnlyMembers(stats, memberSteps, nil)
	}
	// members 输出序按 agent_key 排序（P5.1 排序稳定）——step 行来自 Go map
	// 遍历顺序随机，JSONL 导出对账要求同输入必同字节。
	sort.Slice(stats.Members, func(i, j int) bool {
		return stats.Members[i].GetAgentKey() < stats.Members[j].GetAgentKey()
	})

	if s.decisionQuery != nil {
		if gates, err := s.decisionQuery.RunGateStats(ctx, run.ID); err != nil {
			s.lg.Warn("run stats: gate stats degraded", loggateway.Err(err), loggateway.Str("run_id", run.ID))
		} else {
			stats.LoopGuardBlocks = int32(gates.LoopGuardBlocks)
			stats.PruneCount = int32(gates.PruneCount)
			stats.PruneBytes = gates.PruneBytes
			stats.CompactCount = int32(gates.CompactCount)
			stats.BudgetTripped = gates.BudgetTripped
			stats.NoProgressTripped = gates.NoProgressTripped
			stats.ParamRuleDenies = int32(gates.ParamRuleDenies)
		}
	}
	return stats
}

// appendStepOnlyMembers 把 memberSteps 中不在 seen 的成员按纯 step 行追加
// （token/calls 零值）。seen nil = 全部追加。调用方负责后续统一排序。
func appendStepOnlyMembers(stats *v1.TeamRunStats, memberSteps map[string]int32, seen map[string]bool) {
	for key, n := range memberSteps {
		if seen[key] {
			continue
		}
		stats.Members = append(stats.Members, &v1.TeamRunMemberStats{AgentKey: key, Steps: n})
	}
}

// GetTeamRunStats 返回单 run 的聚合 stats（R7）。读权限同 run 详情
// （assertRunTeamAccess：team 可见性 = own workspace + shared）。
func (s *TeamService) GetTeamRunStats(ctx context.Context, req *v1.GetTeamRunStatsRequest) (*v1.GetTeamRunStatsResponse, error) {
	runID := strings.TrimSpace(req.GetId())
	if runID == "" {
		return nil, apierror.BadRequest("TEAM", "run id is required")
	}
	if err := s.assertRunTeamAccess(ctx, runID); err != nil {
		return nil, err
	}
	run, err := s.uc.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	steps, err := s.uc.ListRunSteps(ctx, runID)
	if err != nil {
		return nil, err
	}
	return &v1.GetTeamRunStatsResponse{Stats: s.buildRunStats(ctx, run, steps)}, nil
}

// ExportTeamRunStats 按 created_at 窗口导出 run stats JSONL（R7）。system
// 调用方跨全量；租户调用方过滤到本 workspace 可见 team（own + shared，
// 同 run 读语义）——可见性过滤随查询下推 SQL（P5.1 M1：Go 侧后过滤会让
// limit 截断先于租户过滤发生，导出被他人 run 挤占配额）。limit 服务端
// 单点收口于 repo（默认 500 / 硬上限 1000）。
func (s *TeamService) ExportTeamRunStats(ctx context.Context, req *v1.ExportTeamRunStatsRequest) (*v1.ExportTeamRunStatsResponse, error) {
	from, err := parseStatsExportTime(req.GetFrom(), "from")
	if err != nil {
		return nil, err
	}
	to, err := parseStatsExportTime(req.GetTo(), "to")
	if err != nil {
		return nil, err
	}

	// teamIDs nil = system 全量；空非 nil = 租户无可见 team（短路空导出）。
	var teamIDs []string
	if !workspace.IsSystem(ctx) {
		visible, verr := s.uc.ListTeamsByWorkspace(ctx, workspace.IDFromContext(ctx))
		if verr != nil {
			return nil, verr
		}
		teamIDs = make([]string, 0, len(visible))
		for _, t := range visible {
			teamIDs = append(teamIDs, t.ID)
		}
		if len(teamIDs) == 0 {
			return &v1.ExportTeamRunStatsResponse{}, nil
		}
	}

	runs, err := s.uc.ListRunsForStatsExport(ctx, from, to, strings.TrimSpace(req.GetSessionId()), teamIDs, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	count := 0
	for _, run := range runs {
		steps, serr := s.uc.ListRunSteps(ctx, run.ID)
		if serr != nil {
			s.lg.Warn("run stats export: steps degraded", loggateway.Err(serr), loggateway.Str("run_id", run.ID))
			steps = nil
		}
		line, merr := protojson.Marshal(s.buildRunStats(ctx, run, steps))
		if merr != nil {
			s.lg.Warn("run stats export: marshal degraded", loggateway.Err(merr), loggateway.Str("run_id", run.ID))
			continue
		}
		sb.Write(line)
		sb.WriteByte('\n')
		count++
	}
	return &v1.ExportTeamRunStatsResponse{Jsonl: sb.String(), Count: int32(count)}, nil
}

// parseStatsExportTime 解析 RFC3339 窗口边界；空串 = 零值（不限）。
func parseStatsExportTime(raw, field string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, apierror.BadRequest("TEAM", "stats export "+field+" must be RFC3339")
	}
	return t, nil
}
