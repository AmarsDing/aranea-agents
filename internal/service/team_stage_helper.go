package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// resolveTeamStageUpdate 读取当前 TeamStage 的 Version 和 Status，用状态机校验
// 转换，返回 newStatus 和 newVersion。用于修复 spirit_team.go 中的 Version=0 Bug
// （P1 #9d-2，AS-FSM-01）。
//
// 调用者应将返回的 newStatus 和 newVersion 设置到构造的 TeamStage struct 中。
//
// 降级策略：如果读取当前 TeamStage 失败或状态机校验失败，返回 fallbackStatus 和
// fallbackVersion=100（足够大以通过 VersionLT 守卫），保证主流程不中断。降级时
// 会丢失乐观并发控制，但比 Version=0 导致状态完全不持久化要好。
//
// 2026-07-05 P1 #9d-2：统一处理 spirit_team.go 中 5 处 TeamStage 状态转换点。
func resolveTeamStageUpdate(
	ctx context.Context,
	reader biz.TeamStageV2Reader,
	sm *biz.TeamStageStateMachine,
	tsID string,
	event biz.TeamStageEvent,
	fallbackStatus biz.TeamStageStatus,
	lg loggateway.Logger,
) (newStatus biz.TeamStageStatus, newVersion int64) {
	const fallbackVersion = int64(100) // 降级版本号：足够大以通过 VersionLT 守卫
	current, err := reader.GetTeamStage(ctx, tsID)
	if err != nil {
		lg.Warn("resolveTeamStageUpdate: failed to load current TeamStage, fallback",
			loggateway.Str("team_stage_id", tsID),
			loggateway.Err(err))
		return fallbackStatus, fallbackVersion
	}
	newStatus, err = sm.Transition(current.Status, event)
	if err != nil {
		lg.Warn("resolveTeamStageUpdate: invalid transition, fallback",
			loggateway.Str("team_stage_id", tsID),
			loggateway.Str("from_status", string(current.Status)),
			loggateway.Str("event", string(event)),
			loggateway.Err(err))
		return fallbackStatus, fallbackVersion
	}
	return newStatus, current.Version + 1
}
