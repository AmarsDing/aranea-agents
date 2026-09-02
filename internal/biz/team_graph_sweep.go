package biz

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"
)

// teamGraphSweepBatchLimit 单次清扫上限：防止历史积压一次性风暴删除，
// 事件驱动触发（每个团队生命周期事件）下多轮即可收敛。
const teamGraphSweepBatchLimit = 100

// SpiritTeamGraphSweeper sweeps graph assets owned by terminal auto-created
// (spirit/orchestration) teams. Narrow port over *TeamUsecase（O-4 窄接口）.
//
// 背景：Team×Graph 一体化（ADR-10 C1）要求每个团队物化 owned 图资产，但会话
// 一次性团队终态后其图资产永不清算（归档链路是事件驱动的 per-session 扫描，
// 会话结束后不再触发），导致 graph_definitions 被同名编排产物持续淹没。
// 删除语义对齐 ADR-10 D2：历史 run 经执行 steps 降级回放 + 「资产已删除」提示；
// graph_executions.graph_id 为纯字符串无 FK，执行历史聚合不受影响；
// linked_graph_id 保留悬空（清空反而会丢失执行历史聚合键）。
// Stability:evolving
type SpiritTeamGraphSweeper interface {
	SweepTerminalAutoTeamGraphs(ctx context.Context, cutoff time.Time) (int, error)
}

// SweepTerminalAutoTeamGraphs deletes owned graph assets of auto-created teams
// that reached a terminal status before cutoff. Manual teams、linked_external
// 图、非终态团队一律不动（deleteOwnedGraphAsset 内含 owned 归属校验）。
// 幂等；单图失败记 warn 继续，不让历史脏数据阻塞后续清扫。
func (u *TeamUsecase) SweepTerminalAutoTeamGraphs(ctx context.Context, cutoff time.Time) (int, error) {
	if u.graphAssets == nil || u.graphReader == nil {
		return 0, nil
	}
	teams, err := u.reader.ListTeams(ctx)
	if err != nil {
		return 0, err
	}
	swept := 0
	for i := range teams {
		if swept >= teamGraphSweepBatchLimit {
			break
		}
		t := &teams[i]
		if !t.AutoCreated || t.LinkedGraphID == "" {
			continue
		}
		// 终态口径对齐 BatchArchiveTeams + archived（归档即终态沉降点）；
		// interrupted 不排除在归档外但为防 resume 边缘场景不回收其图。
		switch t.Status {
		case TeamStatusCompleted, TeamStatusFailed, TeamStatusCancelled, TeamStatusPartialFailure, TeamStatusArchived:
		default:
			continue
		}
		updatedAt, perr := parseTimeFlexible(t.UpdatedAt)
		if perr != nil {
			// 删除操作采取保守策略：时间不可解析则跳过（与归档的「视为可归档」
			// 兜底相反——归档可逆，删除不可逆）。
			u.lg.Warn("清扫跳过：团队更新时间不可解析",
				loggateway.StepID("team.graph_sweep.parse_err"),
				loggateway.Str("team_id", t.ID),
				loggateway.Str("updated_at", t.UpdatedAt),
			)
			continue
		}
		if !updatedAt.Before(cutoff) {
			continue
		}
		// owned 归属校验前置（与 deleteOwnedGraphAsset 同规）：external/他队图
		// 一律不动；已不存在的图幂等跳过。前置读取使 swept 只计真实删除。
		g, gerr := u.graphReader.GetDefinition(ctx, t.LinkedGraphID)
		if gerr != nil || g == nil || !isTeamOwnedGraph(g) || g.TeamID != t.ID {
			continue
		}
		if derr := u.graphAssets.DeleteOwnedGraph(ctx, t.LinkedGraphID); derr != nil {
			u.lg.Warn("清扫删除 owned 图失败",
				loggateway.StepID("team.graph_sweep.delete_err"),
				loggateway.Str("team_id", t.ID),
				loggateway.Str("graph_id", t.LinkedGraphID),
				loggateway.Err(derr),
			)
			continue
		}
		swept++
	}
	if swept > 0 {
		u.lg.Info("终态自动团队 owned 图清扫完成",
			loggateway.StepID("team.graph_sweep.done"),
			loggateway.Int("swept_count", swept),
		)
	}
	return swept, nil
}
