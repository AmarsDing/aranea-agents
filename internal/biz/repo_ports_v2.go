package biz

import (
	"context"
	"time"
)

// v2 Repo 窄接口（每个接口方法 ≤ 5，按读写职责拆分）。
// 复合接口仅用于 Wire 绑定。
//
// Stability:evolving
//
// 命名说明：使用 SpiritSession / SpiritSessionStatus / TeamRunV2Status 等
// Tier 0 命名（避免与 v1 的 Session 别名和 TeamRunStatus 字符串常量冲突）。

// === Session ===

type SessionV2Reader interface {
	GetSession(ctx context.Context, id string) (SpiritSession, error)
}

type SessionV2Writer interface {
	CreateSession(ctx context.Context, s SpiritSession) (SpiritSession, error)
	UpdateSessionStatus(ctx context.Context, id string, status SpiritSessionStatus) error
}

type SessionV2Repo interface {
	SessionV2Reader
	SessionV2Writer
}

// === Task ===

type TaskV2Reader interface {
	GetTask(ctx context.Context, id string) (Task, error)
	ListTasksBySession(ctx context.Context, sessionID string) ([]Task, error)
}

type TaskV2Writer interface {
	CreateTask(ctx context.Context, t Task) (Task, error)
	UpdateTask(ctx context.Context, t Task) (Task, error)
	UpsertTask(ctx context.Context, t Task) (Task, error)
	// ResumeInterruptedTask atomically transitions a task from interrupted
	// back to running (L3, 2026-07-22). Returns ok=false when the task is not
	// currently interrupted (already resumed by a concurrent click, or
	// terminal otherwise) — callers must treat !ok as a conflict, not an error.
	ResumeInterruptedTask(ctx context.Context, id string, resumeAt time.Time) (t Task, ok bool, err error)
	// CompleteTaskTerminal unconditionally terminalizes a task (L3, 2026-07-22):
	// sets status/completed_at and bumps version from the DB value, ignoring
	// t.Version. Terminal events are inherently monotonic, so they must not
	// depend on the caller supplying a correct version — the synthesis turn's
	// OnTurnEnd hardcodes Version=2, which the UpsertTask VersionLT guard
	// would reject after a resume CAS pushed the version higher, leaving the
	// task running forever. Already-terminal tasks (completed/failed/cancelled)
	// are NOT overwritten (idempotent); interrupted IS overwritable because it
	// is a recovery placeholder, not a true terminal state.
	CompleteTaskTerminal(ctx context.Context, t Task) (Task, error)
}

type TaskV2Repo interface {
	TaskV2Reader
	TaskV2Writer
}

// === Turn ===

type TurnV2Reader interface {
	GetTurn(ctx context.Context, id string) (Turn, error)
	ListTurnsByTask(ctx context.Context, taskID string) ([]Turn, error)
	// MaxSeqBySession returns MAX(seq) of turns_v2 for the session, or 0 if
	// none. R4-Q3: turns_v2.seq is a per-SESSION turn counter (member turns
	// count in their own member session); used to restore the per-session
	// turn counter after process restart.
	MaxSeqBySession(ctx context.Context, sessionID string) (int64, error)
}

type TurnV2Writer interface {
	CreateTurn(ctx context.Context, t Turn) (Turn, error)
	UpdateTurn(ctx context.Context, t Turn) (Turn, error)
	UpsertTurn(ctx context.Context, t Turn) (Turn, error)
}

type TurnV2Repo interface {
	TurnV2Reader
	TurnV2Writer
}

// === Step ===

// StepListOptions controls paged listing of steps within a session.
// Limit<=0 means no pagination (full list, legacy semantics);
// BeforeSeq>0 returns the page with seq < BeforeSeq (walking towards older).
// Chat history lazy load (2026-07-23 design) Phase 1 uses a Limit window.
type StepListOptions struct {
	Limit     int
	BeforeSeq int64
}

type StepV2Reader interface {
	GetStep(ctx context.Context, id string) (Step, error)
	ListStepsByTurn(ctx context.Context, turnID string) ([]Step, error)
	ListStepsByTask(ctx context.Context, taskID string) ([]Step, error)
	ListStepsBySession(ctx context.Context, sessionID string) ([]Step, error) // filters steps.session_id
	// ListStepsBySessionPaged returns steps of a session with pagination,
	// always ordered by seq asc; hasMore reports whether older steps remain.
	// Limit<=0 degrades to the full list (hasMore=false), ordered by started_at asc.
	ListStepsBySessionPaged(ctx context.Context, sessionID string, opts StepListOptions) (steps []Step, hasMore bool, err error)
	// ListStepsBySpiritSession returns all steps under a spirit root (steps.spirit_session_id).
	// Used by StopGeneration cancel to mark in-flight member steps as cancelled.
	ListStepsBySpiritSession(ctx context.Context, spiritSessionID string) ([]Step, error)
	// ListStepsBySessionID returns steps whose session_id equals sessionID only (exact match).
	ListStepsBySessionID(ctx context.Context, sessionID string) ([]Step, error)
	// MaxSeqBySpiritSession returns MAX(seq) for the spirit session, or 0 if none.
	// B-06: used to restore SeqAssigner after process restart.
	MaxSeqBySpiritSession(ctx context.Context, spiritSessionID string) (int64, error)
}

type StepV2Writer interface {
	CreateStep(ctx context.Context, s Step) (Step, error)
	UpdateStep(ctx context.Context, s Step) (Step, error)
	UpsertStep(ctx context.Context, s Step) (Step, error)
}

type StepV2Repo interface {
	StepV2Reader
	StepV2Writer
}

// === TeamStage ===

type TeamStageV2Reader interface {
	GetTeamStage(ctx context.Context, id string) (TeamStage, error)
	ListTeamStagesByTask(ctx context.Context, taskID string) ([]TeamStage, error)
	// GetLatestTeamStageByTeam returns the team's most recent stage (seq desc).
	// S-3 后 TeamStage 按 (teamID, rootTaskID) 每轮一行，无 turn ctx 的路径
	// （cancel/pause/sidebar 读取）无法重放 ID 公式，必须经此查询定位当前行。
	GetLatestTeamStageByTeam(ctx context.Context, teamID string) (TeamStage, error)
}

type TeamStageV2Writer interface {
	CreateTeamStage(ctx context.Context, ts TeamStage) (TeamStage, error)
	UpdateTeamStage(ctx context.Context, ts TeamStage) (TeamStage, error)
	UpsertTeamStage(ctx context.Context, ts TeamStage) (TeamStage, error)
}

type TeamStageV2Repo interface {
	TeamStageV2Reader
	TeamStageV2Writer
}

// === TeamRun ===

type TeamRunV2Reader interface {
	GetTeamRun(ctx context.Context, id string) (TeamRun, error)
	// ListTeamRunsByStage 返回指定 stage 的全部 run，按 seq 升序。
	// 契约：无 run 返回空切片（非 nil、非 error）；调用方不得假定至多一条
	// （假启动对账与重试会产生同 stage 多 run）。
	ListTeamRunsByStage(ctx context.Context, stageID string) ([]TeamRun, error)
}

type TeamRunV2Writer interface {
	CreateTeamRun(ctx context.Context, tr TeamRun) (TeamRun, error)
	UpdateTeamRun(ctx context.Context, tr TeamRun) (TeamRun, error)
	UpsertTeamRun(ctx context.Context, tr TeamRun) (TeamRun, error)
}

type TeamRunV2Repo interface {
	TeamRunV2Reader
	TeamRunV2Writer
}

// TeamRunHeartbeatWriter 团队运行心跳写入端口（P2-1，2026-09-03）。
// 单列 UPDATE heartbeat_at，无版本守卫，last-writer-wins——心跳允许丢序，
// 不与 sequencer 的全行 Upsert 竞争。runner 流式消费节流调用。
// Stability:evolving（节流参数与写路径随 idle 探测调优可能变化）。
type TeamRunHeartbeatWriter interface {
	TouchTeamRunHeartbeat(ctx context.Context, runID string, at time.Time) error
}

// SpiritTeamRunHeartbeatReader idle 探测心跳读取端口（P2-1）。返回指定团队
// 会话下最近一次心跳时间；无心跳记录返回零值（探测回退 steps.started_at
// 语义，行为与 P2-1 之前一致）。
// Stability:evolving。
type SpiritTeamRunHeartbeatReader interface {
	LatestTeamRunHeartbeat(ctx context.Context, sessionID string) (time.Time, error)
}

// === MemberSession ===

type MemberSessionV2Reader interface {
	GetMemberSession(ctx context.Context, id string) (MemberSession, error)
	ListMemberSessionsByRun(ctx context.Context, runID string) ([]MemberSession, error)
	// GetMemberSessionByChatSessionID looks up the MemberSession whose SessionID
	// equals the member's own chat session (used by PauseSession / ResumeSession).
	GetMemberSessionByChatSessionID(ctx context.Context, chatSessionID string) (MemberSession, error)
	// ListOrphanMemberSessionsBySpiritSession returns Mode B member sessions
	// (empty TeamRunID) for a spirit root session, ordered by seq asc.
	ListOrphanMemberSessionsBySpiritSession(ctx context.Context, spiritSessionID string) ([]MemberSession, error)
}

type MemberSessionV2Writer interface {
	CreateMemberSession(ctx context.Context, ms MemberSession) (MemberSession, error)
	UpdateMemberSession(ctx context.Context, ms MemberSession) (MemberSession, error)
	UpsertMemberSession(ctx context.Context, ms MemberSession) (MemberSession, error)
}

type MemberSessionV2Repo interface {
	MemberSessionV2Reader
	MemberSessionV2Writer
}

// === Recovery ===（2026-07-21 P1-5：v2 实体 orphaned-recovery）

// V2RecoveryStats reports how many in-flight rows per v2 table were
// terminalized by FailOrphanedInFlight.
type V2RecoveryStats struct {
	Tasks          int
	Turns          int
	Steps          int
	TeamStages     int
	TeamRuns       int
	MemberSessions int
}

// Total returns the total number of recovered rows across all v2 tables.
func (s V2RecoveryStats) Total() int {
	return s.Tasks + s.Turns + s.Steps + s.TeamStages + s.TeamRuns + s.MemberSessions
}

// InterruptedTaskRef identifies one task terminalized to interrupted by
// startup recovery (L3, 2026-07-22). The startup guard uses these refs to
// notify affected sessions that a resumable task exists.
type InterruptedTaskRef struct {
	TaskID      string
	SessionID   string
	UserMessage string
}

// V2RecoveryRepo terminalizes v2 entities left in-flight by a process
// restart (orphaned). Called once at startup before traffic is accepted.
//
// In-flight = statuses that require the dead process to make progress.
// Human-waiting states are NOT orphaned and must be preserved:
//   - steps_v2.tool_blocked   (tool confirmation, resumable via approve path)
//   - team_stages_v2.waiting_human (HITL, resumable via resume event)
//
// Stability:evolving
type V2RecoveryRepo interface {
	// FailOrphanedInFlight batch-transitions all in-flight rows to terminal
	// states (tasks → interrupted, other entities → failed), incrementing
	// version and stamping completed_at/finished_at with recoverAt. Terminal
	// rows are untouched. Returns per-table counts plus the refs of tasks
	// transitioned to interrupted (L3: startup guard publishes per-session
	// resumable-task notices from them).
	FailOrphanedInFlight(ctx context.Context, recoverAt time.Time) (V2RecoveryStats, []InterruptedTaskRef, error)
}

// === PlanBoard ===

type PlanBoardV2Reader interface {
	GetPlanBoard(ctx context.Context, id string) (PlanBoard, error)
	ListPlanBoardsByTask(ctx context.Context, taskID string) ([]PlanBoard, error)
	// ListPlanBoardsByStatuses lists boards in the given statuses (startup DAG recover).
	ListPlanBoardsByStatuses(ctx context.Context, statuses []PlanStatus) ([]PlanBoard, error)
}

type PlanBoardV2Writer interface {
	CreatePlanBoard(ctx context.Context, pb PlanBoard) (PlanBoard, error)
	UpdatePlanBoard(ctx context.Context, pb PlanBoard) (PlanBoard, error)
	UpsertPlanBoard(ctx context.Context, pb PlanBoard) (PlanBoard, error)
}

type PlanBoardV2Repo interface {
	PlanBoardV2Reader
	PlanBoardV2Writer
}

// === PlanStep ===

type PlanStepV2Reader interface {
	GetPlanStep(ctx context.Context, id string) (PlanStep, error)
	ListPlanStepsByPlan(ctx context.Context, planID string) ([]PlanStep, error)
	ListPlanStepsByTask(ctx context.Context, taskID string) ([]PlanStep, error)
}

type PlanStepV2Writer interface {
	CreatePlanStep(ctx context.Context, ps PlanStep) (PlanStep, error)
	UpdatePlanStep(ctx context.Context, ps PlanStep) (PlanStep, error)
	UpsertPlanStep(ctx context.Context, ps PlanStep) (PlanStep, error)
}

type PlanStepV2Repo interface {
	PlanStepV2Reader
	PlanStepV2Writer
}

// === GraphStage ===（2026-07-04 补齐：与 PlanBoard 一对一关联）

type GraphStageV2Reader interface {
	GetGraphStage(ctx context.Context, id string) (GraphStage, error)
	ListGraphStagesByTask(ctx context.Context, taskID string) ([]GraphStage, error)
	GetGraphStageByPlanBoard(ctx context.Context, planBoardID string) (GraphStage, error)
}

type GraphStageV2Writer interface {
	CreateGraphStage(ctx context.Context, gs GraphStage) (GraphStage, error)
	UpdateGraphStage(ctx context.Context, gs GraphStage) (GraphStage, error)
	UpsertGraphStage(ctx context.Context, gs GraphStage) (GraphStage, error)
}

type GraphStageV2Repo interface {
	GraphStageV2Reader
	GraphStageV2Writer
}

// === GraphNode ===（GraphNode 独立存储，便于 team_stage 创建时回填 team_stage_id）

type GraphNodeV2Reader interface {
	GetGraphNode(ctx context.Context, id string) (GraphNode, error)
	ListGraphNodesByStage(ctx context.Context, graphStageID string) ([]GraphNode, error)
}

type GraphNodeV2Writer interface {
	CreateGraphNode(ctx context.Context, gn GraphNode) (GraphNode, error)
	UpdateGraphNode(ctx context.Context, gn GraphNode) (GraphNode, error)
	UpsertGraphNode(ctx context.Context, gn GraphNode) (GraphNode, error)
}

type GraphNodeV2Repo interface {
	GraphNodeV2Reader
	GraphNodeV2Writer
}
