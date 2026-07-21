package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// resolveTeamStageUpdate 读取当前 TeamStage 的 Version 和 Status，用状态机校验
// 转换，返回 newStatus、newVersion 和 ok。用于修复 spirit_team.go 中的 Version=0 Bug
// （P1 #9d-2，AS-FSM-01）。
//
// 调用者应将返回的 newStatus 和 newVersion 设置到构造的 TeamStage struct 中；
// ok=false 时必须跳过事件发布。
//
// 三类结果（2026-07-21 P0-2 取消竞态修复，拆分原混为一谈的两类失败）：
//  1. 转换成功：ok=true，newStatus 为状态机目标态，newVersion=current.Version+1。
//  2. 读取失败（记录不存在/DB 错误）：ok=true，降级为 fallbackStatus + fallbackVersion=100
//     （足够大以通过 VersionLT 守卫），保证主流程不中断。降级时丢失乐观并发控制，
//     但比 Version=0 导致状态完全不持久化要好。
//  3. 状态机拒绝（当前已是终态）：ok=false。终态是权威的，调用者必须跳过发布，
//     否则迟到的完成回调会把 cancelled 覆盖成 completed（P0-2 取消竞态）。
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
) (newStatus biz.TeamStageStatus, newVersion int64, ok bool) {
	const fallbackVersion = int64(100) // 降级版本号：足够大以通过 VersionLT 守卫
	current, err := reader.GetTeamStage(ctx, tsID)
	if err != nil {
		lg.Warn("resolveTeamStageUpdate: failed to load current TeamStage, fallback",
			loggateway.Str("team_stage_id", tsID),
			loggateway.Err(err))
		return fallbackStatus, fallbackVersion, true
	}
	newStatus, err = sm.Transition(current.Status, event)
	if err != nil {
		// 状态机拒绝：当前状态（通常是终态）不允许该转换。终态权威，跳过发布，
		// 防止取消后迟到的 runner 完成回调覆盖终态（P0-2）。
		lg.Warn("resolveTeamStageUpdate: transition rejected by state machine, skip publish",
			loggateway.Str("team_stage_id", tsID),
			loggateway.Str("from_status", string(current.Status)),
			loggateway.Str("event", string(event)),
			loggateway.Err(err))
		return current.Status, current.Version, false
	}
	return newStatus, current.Version + 1, true
}
