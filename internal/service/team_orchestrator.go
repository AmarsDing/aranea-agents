package service

import (
	"context"

	"aranea-agents/internal/biz"
)

// OrchestrateResult bundles the artifacts produced by Orchestrate so the
// caller (PlanExecutor.dispatchStep) can update the TeamStage with the real
// team ID + members and wire the GraphNode to the correct TeamStageID.
//
// 2026-07-04 问题 4 修复：原先 Orchestrate 只返回一个 channel，导致
// dispatchStep 创建的 TeamStage 一直带着空 TeamID 和空 Members，
// 前端 TeamStagePanel 显示 "Team: " 而不是真实 Team ID，且成员列表为空。
type OrchestrateResult struct {
	// Team is the assembled team (with ID, DisplayName, etc.).
	Team biz.Team
	// TeamSession is the team's parent session.
	TeamSession biz.Session
	// MemberSessions maps agentKey → member session ID.
	MemberSessions map[string]string
	// TeamStageID is the deterministic ID derived from team.ID via
	// agent.NewTeamStageActivityID. Identical to the ID used by
	// publishSpiritTeamAssembled + publishV2TeamRunAndMemberSessions,
	// so dispatchStep can update the same TeamStage record (created inside
	// Orchestrate) instead of creating a conflicting one with uuid.
	//
	// 2026-07-04 问题 4 修复：原先 dispatchStep 用 uuid.NewString() 创建
	// TeamStage，与 publishSpiritTeamAssembled 的派生 ID 不一致，导致同一
	// team 出现两条 TeamStage 记录且 TeamRun/MemberSession 的 TeamStageID
	// 关联到错误记录。
	TeamStageID string
	// CompletionChan emits exactly one TeamCompleteEvent when the team_run
	// reaches a terminal status, then closes.
	CompletionChan <-chan biz.TeamCompleteEvent
}

// TeamOrchestrator dispatches a team_run for a PlanStep and reports its
// completion via the returned channel. The channel must emit exactly one
// TeamCompleteEvent and then close.
//
// Implementations are expected to:
//  1. Build and start the team_run (creating MemberSessions, etc.)
//  2. Wait for the team_run to reach a terminal status
//  3. Send a single TeamCompleteEvent on the channel and close it
//
// Stability: evolving
type TeamOrchestrator interface {
	Orchestrate(ctx context.Context, step biz.PlanStep, ts biz.TeamStage) (*OrchestrateResult, error)
}
