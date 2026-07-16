// web/src/features/chat/v2Types.ts

// === Status / Kind string-literal unions ===

export type TaskStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
export type TurnStatus = 'running' | 'completed' | 'failed';
export type StepKind = 'thinking' | 'action' | 'reply' | 'notice' | 'confirm' | 'error';
export type StepStatus = 'pending' | 'running' | 'tool_running' | 'tool_blocked' | 'completed' | 'failed' | 'cancelled';
export type TeamRunStatus = 'running' | 'paused' | 'completed' | 'failed' | 'cancelled';
export type MemberSessionStatus = 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'skipped';
export type TeamStageStatus = 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled' | 'waiting_human';
export type TeamStageStage = 'assembled' | 'planning' | 'executing' | 'completed' | 'failed';
export type PlanStrategy = 'sequential' | 'parallel' | 'dag' | 'coordinator';
export type PlanStatus = 'planning' | 'executing' | 'completed' | 'failed' | 'partial_failure';
export type PlanStepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'partial_failure';

export type GraphStageStatus = 'running' | 'completed' | 'failed' | 'interrupted';
export type GraphNodeStatus = 'pending' | 'running' | 'completed' | 'failed' | 'interrupted';

// === Entity structs (PascalCase JSON keys — no json tags on backend) ===

export interface Task {
  ID: string;
  SessionID: string;
  UserMessage: string;
  Status: TaskStatus;
  Seq: number;
  Version: number;
  CreatedAt: string;
  UpdatedAt: string;
  CompletedAt: string | null;
}

export interface Turn {
  ID: string;
  TaskID: string;
  SessionID: string;
  SpiritSessionID: string;
  ParentTurnID: string;
  AgentKey: string;
  TeamID: string;
  TeamStageID: string;
  Seq: number;
  Version: number;
  Status: TurnStatus;
  StartedAt: string;
  CompletedAt: string | null;
}

export interface Step {
  ID: string;
  TurnID: string;
  TaskID: string;
  SessionID: string;
  SpiritSessionID: string;
  Kind: StepKind;
  AuthorAgentKey: string;
  Seq: number;
  Version: number;
  Content: string;
  Reasoning: string;
  ToolName: string;
  ToolCallID: string;
  ToolArgs: unknown | null; // json.RawMessage → nested JSON value
  ToolResult: unknown | null;
  ToolDurationMs: number;
  ToolErrorCode: string;
  NoticeType?: string; // kind=notice: notification type (e.g. "model_router", "cost_guard")
  Status: StepStatus;
  IsFinal: boolean;
  StartedAt: string;
  CompletedAt: string | null;
}

export interface MemberInfo {
  AgentKey: string;
  AgentName: string;
  AvatarURL: string;
  ChildSessionID: string;
  Status: string;
}

export interface TeamStage {
  ID: string;
  TaskID: string;
  TurnID: string;
  SessionID: string;
  TeamID: string;
  TeamName: string;
  DagNodeID: string;
  DependsOn: string[];
  Status: TeamStageStatus;
  Stage: TeamStageStage;
  Members: MemberInfo[];
  Strategy: string;
  StartedAt: string;
  CompletedAt: string | null;
  Seq: number;
  Version: number;
}

export interface TeamRun {
  ID: string;
  TeamStageID: string;
  TaskID: string;
  SessionID: string;
  SpiritSessionID: string;
  DagNodeID: string;
  DependsOn: string[];
  Status: TeamRunStatus;
  StartedAt: string;
  CompletedAt: string | null;
  Seq: number;
  Version: number;
  Error: string;
}

export interface MemberSession {
  ID: string;
  TeamRunID: string;
  TeamStageID: string;
  TaskID: string;
  SessionID: string;
  SpiritSessionID: string;
  AgentKey: string;
  AgentName: string;
  AvatarURL: string;
  Status: MemberSessionStatus;
  Seq: number;
  Version: number;
  StartedAt: string;
  FinishedAt: string | null;
  Error: string;
}

export interface TokenUsage {
  PromptTokens: number;
  CompletionTokens: number;
  TotalTokens: number;
}

export interface MemberReport {
  AgentKey: string;
  AgentName: string;
  Output: string;
  TokensUsed: TokenUsage;
  DurationMs: number;
  Error: string;
}

export interface StepResult {
  Output: string;
  MemberReports: MemberReport[];
  TokensUsed: TokenUsage;
  DurationMs: number;
}

export interface StepError {
  Code: string;
  Message: string;
  Retryable: boolean;
  FailedMember: MemberReport | null;
}

export interface PlanStep {
  ID: string;
  PlanID: string;
  TaskID: string;
  Label: string;
  Description: string;
  DependsOn: string[];
  MappedTeamStageID: string;
  Status: PlanStepStatus;
  AutoSynthesis: boolean;
  StartedAt: string;
  CompletedAt: string | null;
  Seq: number;
  Version: number;
  Result: StepResult | null;
  Error: StepError | null;
}

export interface PlanBoard {
  ID: string;
  TaskID: string;
  TurnID: string;
  SessionID: string;
  Strategy: PlanStrategy;
  Status: PlanStatus;
  Steps: PlanStep[];
  StartedAt: string;
  CompletedAt: string | null;
  Seq: number;
  Version: number;
}

// 设计：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.2.2 / §3.7.5
export interface GraphNode {
  ID: string;
  GraphStageID: string;
  Label: string;
  DagNodeID: string; // 对应 plan_step.id
  TeamStageID: string; // 关联的 team_stage（如已创建，否则空）
  Status: GraphNodeStatus;
  DependsOn: string[]; // 派生自 plan_step.depends_on，不持久化
}

export interface GraphStage {
  ID: string;
  TaskID: string;
  TurnID: string;
  SessionID: string;
  PlanBoardID: string; // 一对一关联 PlanBoard
  Nodes: GraphNode[];
  Status: GraphStageStatus;
  StartedAt: string;
  CompletedAt: string | null;
  Seq: number;
  Version: number;
}

// === EventKind string literals ===

export type EventKind =
  | 'task.created'
  | 'task.updated'
  | 'task.completed'
  | 'task.failed'
  | 'turn.started'
  | 'turn.completed'
  | 'turn.failed'
  | 'step.created'
  | 'step.streaming'
  | 'step.updated'
  | 'step.completed'
  | 'step.failed'
  | 'team_stage.created'
  | 'team_stage.updated'
  | 'team_stage.completed'
  | 'team_stage.failed'
  | 'team_run.started'
  | 'team_run.completed'
  | 'team_run.failed'
  | 'member_session.created'
  | 'member_session.updated'
  | 'plan_board.created'
  | 'plan_board.updated'
  | 'plan_step.started'
  | 'plan_step.completed'
  | 'plan_step.failed'
  | 'plan_step.skipped'
  | 'plan_step.updated'
  | 'graph_stage.created'
  | 'graph_stage.updated'
  | 'graph_stage.completed'
  | 'graph_stage.failed'
  | 'graph_stage.interrupted'
  | 'graph_node.updated'
  // Phase 3b-D Task 12: system-domain events (no entity table).
  // Backend structs: biz.RunStatusEvent / HeartbeatEvent / SystemNoticeEvent.
  // Note: sessionID is unexported on the backend → NOT in JSON payload;
  // routing to the correct WS client is done server-side via SpiritSessionID().
  | 'system.run_status'
  | 'system.heartbeat'
  | 'system.notice';

// === Event payload shapes (what's inside envelope.payload) ===
// Note: backend event structs have NO json tags, so exported fields
// (PascalCase) are the JSON keys. Unexported fields (taskID, spiritSessionID,
// occurredAt) are NOT serialized.

export interface TaskEventPayload {
  Task: Task;
}
export interface TurnEventPayload {
  TurnID: string;
  Turn: Turn;
}
export interface StepCreatedPayload {
  Step: Step;
}
export interface StepStreamingPayload {
  StepID: string;
  DeltaField: string;
  DeltaChunk: string;
}
export interface StepUpdatedPayload {
  Step: Step;
}
export interface TeamStageEventPayload {
  TeamStage: TeamStage;
}
export interface TeamRunEventPayload {
  TeamRun: TeamRun;
}
export interface MemberSessionEventPayload {
  MemberSession: MemberSession;
}
export interface PlanBoardEventPayload {
  PlanBoard: PlanBoard;
}
export interface PlanStepEventPayload {
  PlanStep: PlanStep;
}
export interface PlanStepSkippedPayload {
  PlanStep: PlanStep;
  Reason: string;
}

export interface GraphStageEventPayload {
  GraphStage: GraphStage;
}
export interface GraphNodeEventPayload {
  GraphNode: GraphNode;
}

// Phase 3b-D Task 12: system-domain event payloads.
// Backend structs: biz.RunStatusEvent / HeartbeatEvent / SystemNoticeEvent.
// Only exported (PascalCase) fields are serialized; sessionID is unexported.
export interface RunStatusEventPayload {
  RunID: string;
  Status: string;
  Meta: Record<string, unknown> | null;
}
export interface HeartbeatEventPayload {
  Message: string;
  Meta: Record<string, unknown> | null;
}
export interface SystemNoticeEventPayload {
  NoticeType: string;
  Message: string;
  Meta: Record<string, unknown> | null;
}

// Discriminated union of all v2 events
export type V2Event =
  | TaskEventPayload
  | TurnEventPayload
  | StepCreatedPayload
  | StepStreamingPayload
  | StepUpdatedPayload
  | TeamStageEventPayload
  | TeamRunEventPayload
  | MemberSessionEventPayload
  | PlanBoardEventPayload
  | PlanStepEventPayload
  | PlanStepSkippedPayload
  | GraphStageEventPayload
  | GraphNodeEventPayload
  | RunStatusEventPayload
  | HeartbeatEventPayload
  | SystemNoticeEventPayload;

// === WS envelope ===

export interface V2WsEnvelope {
  type: 'v2_event';
  kind: EventKind;
  /** Spirit session id (envelope root) for global WS consumers. */
  session_id?: string;
  /** B-06 durable outbox cursor for last_event_id reconnect replay. */
  event_id?: string;
  payload: V2Event;
}
