package agent

import (
	"context"

	"github.com/google/uuid"
)

// Phase 5 (typed ID consistency): distinct string types for each Activity ID
// flavor. Constructor functions return typed values; callers must explicitly
// convert to `string` when assigning to polymorphic string fields (e.g.
// `Activity.ID`, `Activity.ParentActivityID`). This provides compile-time
// safety against mixing up ID-generating functions — passing a
// TeamStageActivityID where GraphStageActivityID is expected is now a
// compile error instead of a silent runtime bug.
//
// The underlying type is `string` so the conversion is zero-cost; the
// safety is purely at the API boundary.

// RootTaskActivityID is the activity ID of the root task (user message).
// Sourced from ctx via RootTaskActivityIDFromCtx.
type RootTaskActivityID string

// GraphStageActivityID is the deterministic graph_stage activity ID derived
// from a spirit session ID. Shared between spirit_team (service) and the
// team runner so both compute the same ID for parent-child nesting.
type GraphStageActivityID string

// TeamStageActivityID is the deterministic team_stage activity ID derived
// from a team ID. Shared between spirit_team assembler and team runner.
type TeamStageActivityID string

// SessionActivityID is the deterministic member-session activity ID derived
// from the v2 team run ID + agent key. Shared between the spirit_team service
// and the team runner so both writers converge on ONE member_sessions_v2 row
// per member per run (persistence is upsert-by-ID).
type SessionActivityID string

// rootTaskActivityIDKey is the context key for the root task activity ID.
// The ActivityProjector sets this in OnTurnStart so that downstream
// business orchestrators (spirit_team, team runner) can use it as the
// ParentActivityID for direct-publish events (team_stage, graph_stage,
// session), ensuring the frontend activity tree is correctly nested.
type rootTaskActivityIDKey struct{}

// ContextWithRootTaskActivityID returns a new context with the root task
// activity ID stored.
func ContextWithRootTaskActivityID(ctx context.Context, id RootTaskActivityID) context.Context {
	return context.WithValue(ctx, rootTaskActivityIDKey{}, id)
}

// RootTaskActivityIDFromCtx extracts the root task activity ID from the
// context. Returns zero value if not set.
func RootTaskActivityIDFromCtx(ctx context.Context) RootTaskActivityID {
	if v, ok := ctx.Value(rootTaskActivityIDKey{}).(RootTaskActivityID); ok {
		return v
	}
	return ""
}

// graphStageNamespace is a fixed UUID used as the namespace for deterministic
// graph_stage Activity IDs. Using deterministic IDs allows the team runner and
// spirit_team assembler to compute the same graph_stage activity ID and use it
// as ParentActivityID for team_stage events, ensuring the frontend activity
// tree has the correct 3-level nesting: graph_stage → team_stage → session.
var graphStageNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v1"))

// NewGraphStageActivityID returns the deterministic graph_stage Activity ID
// for a given spirit session ID. Both spirit_team (service) and team runner
// use this to compute the same ID, enabling team_stage events to be
// correctly parented under graph_stage.
func NewGraphStageActivityID(spiritSessionID string) GraphStageActivityID {
	return GraphStageActivityID(uuid.NewSHA1(graphStageNamespace, []byte(spiritSessionID)).String())
}

// teamStageNamespace is a fixed UUID used as the namespace for deterministic
// team_stage Activity IDs. Using deterministic IDs allows the team runner to
// compute the same team_stage activity ID and pass it to the child session's
// ActivityProjector as the ParentActivityID for member thinking/action/reply
// activities.
var teamStageNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.team_stage.v1"))

// NewTeamStageActivityID returns the deterministic team_stage Activity ID
// for a given team ID + root task ID (run dimension). Both spirit_team
// (service) and team runner (team package) use this to compute the same ID.
//
// S-3（2026-08-05）：run 维度修复 ID 碰撞。此前公式只含 teamID，同团队
// 第二轮 turn 复用第一轮的 team_stages_v2/team_runs_v2/member_sessions_v2
// 行——FSM completed→running 转换被拒、outcome 哨兵版本带（1<<40）阻塞
// created（V=1）写入，状态永久冻结。rootTaskID 每轮用户输入唯一（Mode A
// 由 executeTeamTurnViaHooks 注入新 UUID；Mode B 继承 plan 根 Task），
// 下游 NewTeamRunV2ID/NewMemberSessionActivityID 从 TeamStageID 派生，
// 自动继承 run 隔离。
//
// rootTaskID 为空时降级为 legacy teamID-only 公式：仅用于无 turn ctx 的
// 特殊路径（如 recovery），保证其写入自洽；正常 turn 链路永不为空。
func NewTeamStageActivityID(teamID, rootTaskID string) TeamStageActivityID {
	key := teamID
	if rootTaskID != "" {
		key = teamID + ":" + rootTaskID
	}
	return TeamStageActivityID(uuid.NewSHA1(teamStageNamespace, []byte(key)).String())
}

// NewTeamRunV2ID returns the deterministic v2 team run ID for a given
// team_stage activity ID. Single source for the formula previously inlined
// in service/spirit_team.go and service/team_pause.go:
// uuid.NewSHA1(NameSpaceDNS, "aranea.team_run.v2:"+teamStageID).
//
// Being derived from the (team-deterministic) teamStageID, the v2 team run
// ID is stable for a given team within a spirit session — every writer
// (service, runner, pause path) computes the same value.
func NewTeamRunV2ID(teamStageID string) string {
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.team_run.v2:"+teamStageID)).String()
}

// NewMemberSessionActivityID returns the deterministic member_session Activity
// ID for a given v2 team run ID and agent key. Single source for the formula:
// uuid.NewSHA1(NameSpaceDNS, "aranea.member_session.v2:"+teamRunID+":"+agentKey).
//
// 2026-07-25 重复行修复：runner 与 service 曾使用两套公式（teamID 作用域 vs
// teamRunID 作用域），upsert-by-ID 无法收敛导致 member_sessions_v2 每个成员
// 两行。统一为本函数后双方写入同一行。run 作用域同时保证同一团队重跑时
// 不与上一次运行的成员行冲突。
func NewMemberSessionActivityID(teamRunID, agentKey string) SessionActivityID {
	return SessionActivityID(uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.member_session.v2:"+teamRunID+":"+agentKey)).String())
}

// sessionActivityIDKey is the context key for the session activity ID.
// The team runner sets this in the context before starting a member session,
// so the child session's ActivityProjector can use it as the ParentActivityID
// for member thinking/action/reply activities.
type sessionActivityIDKey struct{}

// ContextWithSessionActivityID returns a new context with the session
// activity ID stored.
func ContextWithSessionActivityID(ctx context.Context, id SessionActivityID) context.Context {
	return context.WithValue(ctx, sessionActivityIDKey{}, id)
}

// SessionActivityIDFromCtx extracts the session activity ID from the context.
// Returns zero value if not set.
func SessionActivityIDFromCtx(ctx context.Context) SessionActivityID {
	if v, ok := ctx.Value(sessionActivityIDKey{}).(SessionActivityID); ok {
		return v
	}
	return ""
}
