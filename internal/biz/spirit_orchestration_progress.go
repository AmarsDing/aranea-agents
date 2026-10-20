package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func (o *SpiritOrchestration) BuildCascadeBlockedResults(ctx context.Context, teams []Team) []TeamSynthesisResult {
	var results []TeamSynthesisResult
	for i := range teams {
		t := teams[i]
		summary, keyFindings, extractErr := o.delivery.ExtractTeamOutput(ctx, t.ID)
		if extractErr != nil {
			o.lg.Warn("提取团队输出失败",
				loggateway.StepID("spirit.extract_output_err"),
				loggateway.Str("team_id", t.ID),
				loggateway.Err(extractErr),
			)
		}
		result := TeamSynthesisResult{
			TeamID:      t.ID,
			TeamName:    t.DisplayName,
			TaskName:    t.TaskDescription,
			Status:      t.Status,
			Summary:     summary,
			KeyFindings: keyFindings,
		}
		if t.Status == TeamStatusFailed {
			result.Summary = "[执行失败] " + result.Summary
		}
		results = append(results, result)
	}
	failedDagNodes := make(map[string]string)
	for i := range teams {
		if teams[i].Status == TeamStatusFailed && teams[i].DagNodeID != "" {
			failedDagNodes[teams[i].DagNodeID] = teams[i].DisplayName
		}
	}
	for i := range teams {
		if teams[i].Status != TeamStatusPending {
			continue
		}
		for _, depID := range teams[i].DependsOn {
			if failedName, ok := failedDagNodes[depID]; ok {
				results = append(results, TeamSynthesisResult{
					TeamID:   teams[i].ID,
					TeamName: teams[i].DisplayName,
					TaskName: teams[i].TaskDescription,
					Status:   TeamStatusBlocked,
					Summary:  fmt.Sprintf("被失败团队 %s 阻塞", failedName),
				})
				break
			}
		}
	}
	return results
}

// Domain: Orchestration — auto-archive completed/failed teams past threshold.
func (o *SpiritOrchestration) AutoArchiveCompletedTeams(ctx context.Context, spiritSessionID string) {
	cfg := o.resolveParallelConfig(ctx, spiritSessionID)
	if cfg.AutoArchiveSeconds <= 0 {
		return
	}

	if o.graphSweeper != nil {
		cutoff := time.Now().Add(-cfg.AutoArchiveAfter())
		if _, err := o.graphSweeper.SweepTerminalAutoTeamGraphs(ctx, cutoff); err != nil {
			o.lg.Warn("终态自动团队图资产清扫失败",
				loggateway.StepID("spirit.auto_archive.sweep_err"),
				loggateway.Str("spirit_session_id", spiritSessionID),
				loggateway.Err(err),
			)
		}
	}
	teams, err := o.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		o.lg.Warn("查询精灵团队列表失败，跳过自动归档",
			loggateway.StepID("spirit.auto_archive.list_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return
	}
	threshold := time.Now().Add(-cfg.AutoArchiveAfter())
	var archiveIDs []string
	for _, t := range teams {
		if t.Status != TeamStatusCompleted && t.Status != TeamStatusFailed && t.Status != TeamStatusCancelled {
			continue
		}
		updatedAt, parseErr := parseTimeFlexible(t.UpdatedAt)
		if parseErr != nil {
			o.lg.Warn("解析团队更新时间失败，使用兜底策略（视为可归档）",
				loggateway.StepID("spirit.auto_archive.parse_err"),
				loggateway.Str("team_id", t.ID),
				loggateway.Str("updated_at", t.UpdatedAt),
				loggateway.Err(parseErr),
			)

			updatedAt = time.Now().Add(-cfg.AutoArchiveAfter())
		}
		if updatedAt.Before(threshold) {
			archiveIDs = append(archiveIDs, t.ID)
		}
	}
	if len(archiveIDs) == 0 {
		return
	}
	archived, archiveErr := o.teamUC.BatchArchiveTeams(ctx, archiveIDs)
	if archiveErr != nil {
		o.lg.Warn("批量归档团队失败",
			loggateway.StepID("spirit.auto_archive.batch_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(archiveErr),
		)
		return
	}
	if archived > 0 {
		o.lg.Info("批量归档团队完成",
			loggateway.StepID("spirit.auto_archive.batch_done"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Int("archived_count", archived),
		)
	}
}

func (o *SpiritOrchestration) CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]TeamProgress, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	teams, err := o.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	out := make([]TeamProgress, 0, len(teams))
	for i := range teams {
		tp := TeamProgress{
			TeamID:   teams[i].ID,
			TeamName: teams[i].DisplayName,
			Status:   teams[i].Status,
		}
		switch teams[i].Status {
		case TeamStatusCompleted:
			tp.ProgressPct = 100
			tp.CurrentStep = "已完成"
		case TeamStatusFailed:
			tp.ProgressPct = 0
			tp.CurrentStep = "执行失败"
		case TeamStatusCancelled:
			tp.ProgressPct = 0
			tp.CurrentStep = "已取消"
		case TeamStatusPending:
			tp.ProgressPct = 0
			tp.CurrentStep = "等待执行"
		default:
		}
		runs, runErr := o.teamUC.ListRuns(ctx, teams[i].ID, SpiritRecentRunCount)
		if runErr == nil && len(runs) > 0 {
			latestRun := runs[0]
			tp.DurationMs = int64(latestRun.DurationMS)
			if IsTeamStatusActive(tp.Status) {
				completedRuns := 0
				for _, r := range runs {
					if r.Status == TeamRunStatusSuccess {
						completedRuns++
					}
				}
				if len(runs) > 0 {
					tp.ProgressPct = float64(completedRuns) / float64(len(runs)) * 100
				}
				if tp.ProgressPct >= 100 {
					tp.ProgressPct = 99
				}
				tp.CurrentStep = fmt.Sprintf("执行中 (已完成 %d/%d 轮)", completedRuns, len(runs))
			}
		}
		out = append(out, tp)
	}
	return out, nil
}

// RecordTeamCompletion records DQ Score, infers topology, and creates evolution suggestions
// for a completed team. Returns the computed DQ Score and inferred topology.
// Domain: Orchestration — record DQ score and create evolution suggestions on team completion.
func (o *SpiritOrchestration) RecordTeamCompletion(ctx context.Context, team Team, durationMs int64) (dqScore float64, topology TopologyType) {

	o.CancelTimeoutTimer(team.ID)

	if team.DagNodeID != "" {
		if werr := o.delivery.WriteDeliverablesToSession(ctx, team.ID); werr != nil {
			if errors.Is(werr, ErrNoRealDeliverable) {

				o.lg.Info("团队无真实交付物，跳过交付物落库",
					loggateway.StepID("spirit.team.completion.deliverables"),
					loggateway.Str("team_id", team.ID),
				)
			} else {
				o.lg.Warn("交付物落库失败（下游团队将无注入输入）",
					loggateway.StepID("spirit.team.completion.deliverables"),
					loggateway.Str("team_id", team.ID),
					loggateway.Err(werr),
				)
			}
		}
	}

	if o.orchCache == nil || team.DagNodeID == "" {
		return 0, ""
	}
	dqScore = ComputeDQScore(TeamSynthesisResult{
		TeamID:   team.ID,
		TeamName: team.DisplayName,
		TaskName: team.TaskDescription,

		Status: TeamStatusCompleted,
	}, durationMs)
	taskPattern := ExtractTaskPattern(team.TaskDescription)
	topology = InferTopologyFromTeam(team, o.lg)
	o.orchCache.RecordCompletion(ctx, taskPattern, topology, dqScore, 1, durationMs)
	o.lg.Info("精灵团队完成，记录 DQ Score",
		loggateway.StepID("spirit.team.completion"),
		loggateway.Str("team_id", team.ID),
		loggateway.Str("task_pattern", taskPattern),
		loggateway.Float64("dq_score", dqScore),
	)

	if dqScore < DQEvolutionThreshold && o.evolutionSugg != nil && team.SpiritSessionID != "" {
		altTopology, altFound := o.orchCache.SuggestBestAlternativeTopology(team.TaskDescription, topology)
		content := fmt.Sprintf("团队 %q 的 DQ Score 为 %.2f（低于阈值 %.1f），当前拓扑 %s 执行效果不佳。", team.DisplayName, dqScore, DQEvolutionThreshold, topology)
		if altFound {
			content += fmt.Sprintf("建议尝试 %s 拓扑。", altTopology)
		} else {
			content += "暂无历史数据推荐替代拓扑，建议调整任务描述或减少团队数量。"
		}
		_, suggErr := o.evolutionSugg.CreateSuggestion(ctx, EvolutionSuggestion{
			AgentID: team.SpiritSessionID,
			Type:    "orchestration_optimization",
			Title:   fmt.Sprintf("编排优化建议: %s", TruncateRunes(team.TaskDescription, MaxSuggestionTitleLen)),
			Content: content,
			Status:  "pending",
		})
		if suggErr != nil {
			o.lg.Warn("创建编排优化建议失败",
				loggateway.StepID("spirit.evolution_suggestion_err"),
				loggateway.Str("team_id", team.ID),
				loggateway.Err(suggErr),
			)
		}
	}
	return dqScore, topology
}
