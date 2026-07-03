// web/src/features/chat/v2Types.ts

// === Status / Kind string-literal unions ===

export type TaskStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
export type TurnStatus = 'running' | 'completed' | 'failed';
export type StepKind = 'thinking' | 'action' | 'reply' | 'notice' | 'confirm' | 'error';
export type StepStatus = 'pending' | 'running' | 'tool_running' | 'tool_blocked' | 'completed' | 'failed' | 'cancelled';
export type TeamStageStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'waiting_human';
export type TeamStageStage = 'assembled' | 'planning' | 'executing' | 'completed' | 'failed';
export type TeamRunStatus = 'running' | 'completed' | 'failed' | 'cancelled';
export type MemberSessionStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped';
export type PlanStrategy = 'sequential' | 'parallel' | 'dag' | 'coordinator';
export type PlanStatus = 'planning' | 'executing' | 'completed' | 'failed' | 'partial_failure';
export type PlanStepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'partial_failure';

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
  | 'plan_step.updated';

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
  | PlanStepSkippedPayload;

// === WS envelope ===

export interface V2WsEnvelope {
  type: 'v2_event';
  kind: EventKind;
  payload: V2Event;
}
