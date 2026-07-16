// web/src/features/session/v2Api.ts
//
// v2 entity read API client — calls SessionV2Service HTTP endpoints
// (GET /v2/sessions/{id}/tasks, /v2/tasks/{id}/turns, /v2/sessions/{id}/steps)
// and maps proto JSON (camelCase, base64 bytes) to v2Types.ts PascalCase shapes.
//
// Kept separate from v1 `api.ts` to avoid coupling during the Phase 3b-D
// migration; once v1 ListActivities callers are removed (Task 15), the two
// files can be reconciled if desired.

import { kratosApi } from '../../services';
import type {
  GraphNode,
  GraphNodeStatus,
  GraphStage,
  GraphStageStatus,
  MemberInfo,
  MemberReport,
  MemberSession,
  MemberSessionStatus,
  PlanBoard,
  PlanStatus,
  PlanStep,
  PlanStepStatus,
  PlanStrategy,
  Step,
  StepKind,
  StepStatus,
  StepError,
  StepResult,
  Task,
  TaskStatus,
  TeamRun,
  TeamRunStatus,
  TeamStage,
  TeamStageStage,
  TeamStageStatus,
  TokenUsage,
  Turn,
  TurnStatus,
} from '../chat/v2Types';

// === Proto JSON DTOs (camelCase, mirrors session_v2.proto messages) ===

interface TaskV2Dto {
  id?: string;
  sessionId?: string;
  userMessage?: string;
  status?: string;
  seq?: number | string;
  version?: number | string;
  createdAt?: string;
  updatedAt?: string;
  completedAt?: string | null;
}

interface TurnV2Dto {
  id?: string;
  taskId?: string;
  sessionId?: string;
  spiritSessionId?: string;
  parentTurnId?: string;
  agentKey?: string;
  teamId?: string;
  teamStageId?: string;
  seq?: number | string;
  version?: number | string;
  status?: string;
  startedAt?: string;
  completedAt?: string | null;
}

interface StepV2Dto {
  id?: string;
  turnId?: string;
  taskId?: string;
  sessionId?: string;
  spiritSessionId?: string;
  kind?: string;
  authorAgentKey?: string;
  seq?: number | string;
  version?: number | string;
  content?: string;
  reasoning?: string;
  toolName?: string;
  toolCallId?: string;
  // proto `bytes` serializes as base64-encoded string in JSON.
  toolArgs?: string | null;
  toolResult?: string | null;
  toolDurationMs?: number | string;
  toolErrorCode?: string;
  noticeType?: string;
  status?: string;
  isFinal?: boolean;
  startedAt?: string;
  completedAt?: string | null;
}

interface ListTasksV2Response {
  tasks?: TaskV2Dto[];
}
interface ListTurnsV2Response {
  turns?: TurnV2Dto[];
}
interface ListStepsV2Response {
  steps?: StepV2Dto[];
}

// === Helpers ===

/** proto3 int64 may arrive as number or string; coerce to a safe number. */
function toNum(v: number | string | undefined | null): number {
  if (v === undefined || v === null) return 0;
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}

/** proto3 omits empty string; coerce undefined → ''. */
function toStr(v: string | undefined | null): string {
  return v ?? '';
}

function toNullableStr(v: string | undefined | null): string | null {
  return v === undefined ? null : v;
}

/**
 * Decode a proto `bytes` field carrying JSON. protojson serializes `bytes`
 * as base64; some gateways may pre-decode to a UTF-8 JSON string. Handle both,
 * plus null / empty.
 */
export function decodeBytesJson(v: string | null | undefined): unknown | null {
  if (v === undefined || v === null || v === '') return null;
  // Already-decoded object (defensive — gateway may pre-parse).
  if (typeof v === 'object') return v;
  // Try base64 → UTF-8 bytes → JSON.parse.
  // atob() returns a Latin-1 binary string; non-ASCII (e.g. Chinese) UTF-8 bytes
  // would be misinterpreted as Latin-1 characters, causing mojibake. We must
  // re-encode to Uint8Array and decode as UTF-8 before JSON.parse.
  try {
    const binary = atob(v);
    if (!binary) return null;
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    const decoded = new TextDecoder('utf-8').decode(bytes);
    return JSON.parse(decoded);
  } catch {
    // Fall through: maybe it's a literal JSON string.
  }
  // Maybe the gateway returned the JSON literal directly.
  try {
    return JSON.parse(v);
  } catch {
    return null;
  }
}

function mapTask(dto: TaskV2Dto): Task {
  return {
    ID: toStr(dto.id),
    SessionID: toStr(dto.sessionId),
    UserMessage: toStr(dto.userMessage),
    Status: (toStr(dto.status) || 'pending') as TaskStatus,
    Seq: toNum(dto.seq),
    Version: toNum(dto.version),
    CreatedAt: toStr(dto.createdAt),
    UpdatedAt: toStr(dto.updatedAt),
    CompletedAt: toNullableStr(dto.completedAt),
  };
}

function mapTurn(dto: TurnV2Dto): Turn {
  return {
    ID: toStr(dto.id),
    TaskID: toStr(dto.taskId),
    SessionID: toStr(dto.sessionId),
    SpiritSessionID: toStr(dto.spiritSessionId),
    ParentTurnID: toStr(dto.parentTurnId),
    AgentKey: toStr(dto.agentKey),
    TeamID: toStr(dto.teamId),
    TeamStageID: toStr(dto.teamStageId),
    Seq: toNum(dto.seq),
    Version: toNum(dto.version),
    Status: (toStr(dto.status) || 'running') as TurnStatus,
    StartedAt: toStr(dto.startedAt),
    CompletedAt: toNullableStr(dto.completedAt),
  };
}

function mapStep(dto: StepV2Dto): Step {
  return {
    ID: toStr(dto.id),
    TurnID: toStr(dto.turnId),
    TaskID: toStr(dto.taskId),
    SessionID: toStr(dto.sessionId),
    SpiritSessionID: toStr(dto.spiritSessionId),
    Kind: (toStr(dto.kind) || 'reply') as StepKind,
    AuthorAgentKey: toStr(dto.authorAgentKey),
    Seq: toNum(dto.seq),
    Version: toNum(dto.version),
    Content: toStr(dto.content),
    Reasoning: toStr(dto.reasoning),
    ToolName: toStr(dto.toolName),
    ToolCallID: toStr(dto.toolCallId),
    ToolArgs: decodeBytesJson(dto.toolArgs),
    ToolResult: decodeBytesJson(dto.toolResult),
    ToolDurationMs: toNum(dto.toolDurationMs),
    ToolErrorCode: toStr(dto.toolErrorCode),
    NoticeType: dto.noticeType || undefined,
    Status: (toStr(dto.status) || 'pending') as StepStatus,
    IsFinal: Boolean(dto.isFinal),
    StartedAt: toStr(dto.startedAt),
    CompletedAt: toNullableStr(dto.completedAt),
  };
}

// === Public API ===

/**
 * List tasks for a session. Backend: GET /v2/sessions/{session_id}/tasks.
 */
export async function listTasksV2(sessionId: string): Promise<Task[]> {
  const resp = await kratosApi.get<ListTasksV2Response>(`/v2/sessions/${encodeURIComponent(sessionId)}/tasks`);
  return (resp.data?.tasks ?? []).map(mapTask);
}

/**
 * List turns for a task. Backend: GET /v2/tasks/{task_id}/turns.
 */
export async function listTurnsV2(taskId: string): Promise<Turn[]> {
  const resp = await kratosApi.get<ListTurnsV2Response>(`/v2/tasks/${encodeURIComponent(taskId)}/turns`);
  return (resp.data?.turns ?? []).map(mapTurn);
}

/**
 * List steps for a session. Backend: GET /v2/sessions/{session_id}/steps.
 * Optional `turnId` / `taskId` filters are passed as query params.
 */
export async function listStepsV2(sessionId: string, opts?: { turnId?: string; taskId?: string }): Promise<Step[]> {
  const params: Record<string, string> = {};
  if (opts?.turnId) params.turn_id = opts.turnId;
  if (opts?.taskId) params.task_id = opts.taskId;
  const resp = await kratosApi.get<ListStepsV2Response>(`/v2/sessions/${encodeURIComponent(sessionId)}/steps`, {
    params,
  });
  return (resp.data?.steps ?? []).map(mapStep);
}

// Note: GetStepV2 RPC is gRPC-only (no HTTP route in session.proto), so no
// HTTP helper is exposed here. Frontend callers needing a single step should
// use listStepsV2 + filter, or wire a gRPC client if a use case emerges.

// 对应后端 SessionV2Service 的 7 个新 List RPC。前端 fetchSessionHistory
// 刷新页面时调用这些端点重建完整活动树（Plan/Graph/Team/Member）。
//

// --- DTOs ---

interface MemberInfoV2Dto {
  agentKey?: string;
  agentName?: string;
  avatarUrl?: string;
  childSessionId?: string;
  status?: string;
}

interface TeamStageV2Dto {
  id?: string;
  taskId?: string;
  turnId?: string;
  sessionId?: string;
  teamId?: string;
  teamName?: string;
  dagNodeId?: string;
  dependsOn?: string[];
  status?: string;
  stage?: string;
  members?: MemberInfoV2Dto[];
  strategy?: string;
  startedAt?: string;
  completedAt?: string | null;
  seq?: number | string;
  version?: number | string;
}

interface TeamRunV2Dto {
  id?: string;
  teamStageId?: string;
  taskId?: string;
  sessionId?: string;
  spiritSessionId?: string;
  dagNodeId?: string;
  dependsOn?: string[];
  status?: string;
  startedAt?: string;
  completedAt?: string | null;
  seq?: number | string;
  version?: number | string;
  error?: string;
}

interface MemberSessionV2Dto {
  id?: string;
  teamRunId?: string;
  teamStageId?: string;
  taskId?: string;
  sessionId?: string;
  spiritSessionId?: string;
  agentKey?: string;
  agentName?: string;
  avatarUrl?: string;
  status?: string;
  seq?: number | string;
  version?: number | string;
  startedAt?: string;
  finishedAt?: string | null;
  error?: string;
}

interface TokenUsageV2Dto {
  promptTokens?: number | string;
  completionTokens?: number | string;
  totalTokens?: number | string;
}

interface MemberReportV2Dto {
  agentKey?: string;
  agentName?: string;
  output?: string;
  tokensUsed?: TokenUsageV2Dto;
  durationMs?: number | string;
  error?: string;
}

interface StepResultV2Dto {
  output?: string;
  memberReports?: MemberReportV2Dto[];
  tokensUsed?: TokenUsageV2Dto;
  durationMs?: number | string;
}

interface StepErrorV2Dto {
  code?: string;
  message?: string;
  retryable?: boolean;
  failedMember?: MemberReportV2Dto | null;
}

interface PlanStepV2Dto {
  id?: string;
  planId?: string;
  taskId?: string;
  label?: string;
  description?: string;
  dependsOn?: string[];
  mappedTeamStageId?: string;
  status?: string;
  autoSynthesis?: boolean;
  startedAt?: string;
  completedAt?: string | null;
  seq?: number | string;
  version?: number | string;
  result?: StepResultV2Dto | null;
  error?: StepErrorV2Dto | null;
}

interface PlanBoardV2Dto {
  id?: string;
  taskId?: string;
  turnId?: string;
  sessionId?: string;
  strategy?: string;
  status?: string;
  steps?: PlanStepV2Dto[];
  startedAt?: string;
  completedAt?: string | null;
  seq?: number | string;
  version?: number | string;
}

interface GraphNodeV2Dto {
  id?: string;
  graphStageId?: string;
  label?: string;
  dagNodeId?: string;
  teamStageId?: string;
  status?: string;
  dependsOn?: string[];
}

interface GraphStageV2Dto {
  id?: string;
  taskId?: string;
  turnId?: string;
  sessionId?: string;
  planBoardId?: string;
  nodes?: GraphNodeV2Dto[];
  status?: string;
  startedAt?: string;
  completedAt?: string | null;
  seq?: number | string;
  version?: number | string;
}

interface ListTeamStagesV2Response {
  teamStages?: TeamStageV2Dto[];
}
interface ListTeamRunsV2Response {
  teamRuns?: TeamRunV2Dto[];
}
interface ListMemberSessionsV2Response {
  memberSessions?: MemberSessionV2Dto[];
}
interface ListPlanBoardsV2Response {
  planBoards?: PlanBoardV2Dto[];
}
interface ListPlanStepsV2Response {
  planSteps?: PlanStepV2Dto[];
}
interface ListGraphStagesV2Response {
  graphStages?: GraphStageV2Dto[];
}
interface ListGraphNodesV2Response {
  graphNodes?: GraphNodeV2Dto[];
}

// --- Mappers ---

function mapMemberInfo(dto: MemberInfoV2Dto): MemberInfo {
  return {
    AgentKey: toStr(dto.agentKey),
    AgentName: toStr(dto.agentName),
    AvatarURL: toStr(dto.avatarUrl),
    ChildSessionID: toStr(dto.childSessionId),
    Status: toStr(dto.status),
  };
}

function mapTeamStage(dto: TeamStageV2Dto): TeamStage {
  return {
    ID: toStr(dto.id),
    TaskID: toStr(dto.taskId),
    TurnID: toStr(dto.turnId),
    SessionID: toStr(dto.sessionId),
    TeamID: toStr(dto.teamId),
    TeamName: toStr(dto.teamName),
    DagNodeID: toStr(dto.dagNodeId),
    DependsOn: dto.dependsOn ?? [],
    Status: (toStr(dto.status) || 'pending') as TeamStageStatus,
    Stage: (toStr(dto.stage) || 'assembled') as TeamStageStage,
    Members: (dto.members ?? []).map(mapMemberInfo),
    Strategy: toStr(dto.strategy),
    StartedAt: toStr(dto.startedAt),
    CompletedAt: toNullableStr(dto.completedAt),
    Seq: toNum(dto.seq),
    Version: toNum(dto.version),
  };
}

function mapTeamRun(dto: TeamRunV2Dto): TeamRun {
  return {
    ID: toStr(dto.id),
    TeamStageID: toStr(dto.teamStageId),
    TaskID: toStr(dto.taskId),
    SessionID: toStr(dto.sessionId),
    SpiritSessionID: toStr(dto.spiritSessionId),
    DagNodeID: toStr(dto.dagNodeId),
    DependsOn: dto.dependsOn ?? [],
    Status: (toStr(dto.status) || 'running') as TeamRunStatus,
    StartedAt: toStr(dto.startedAt),
    CompletedAt: toNullableStr(dto.completedAt),
    Seq: toNum(dto.seq),
    Version: toNum(dto.version),
    Error: toStr(dto.error),
  };
}

function mapMemberSession(dto: MemberSessionV2Dto): MemberSession {
  return {
    ID: toStr(dto.id),
    TeamRunID: toStr(dto.teamRunId),
    TeamStageID: toStr(dto.teamStageId),
    TaskID: toStr(dto.taskId),
    SessionID: toStr(dto.sessionId),
    SpiritSessionID: toStr(dto.spiritSessionId),
    AgentKey: toStr(dto.agentKey),
    AgentName: toStr(dto.agentName),
    AvatarURL: toStr(dto.avatarUrl),
    Status: (toStr(dto.status) || 'pending') as MemberSessionStatus,
    Seq: toNum(dto.seq),
    Version: toNum(dto.version),
    StartedAt: toStr(dto.startedAt),
    FinishedAt: toNullableStr(dto.finishedAt),
    Error: toStr(dto.error),
  };
}

function mapTokenUsage(dto: TokenUsageV2Dto | undefined): TokenUsage {
  if (!dto) return { PromptTokens: 0, CompletionTokens: 0, TotalTokens: 0 };
  return {
    PromptTokens: toNum(dto.promptTokens),
    CompletionTokens: toNum(dto.completionTokens),
    TotalTokens: toNum(dto.totalTokens),
  };
}

function mapMemberReport(dto: MemberReportV2Dto): MemberReport {
  return {
    AgentKey: toStr(dto.agentKey),
    AgentName: toStr(dto.agentName),
    Output: toStr(dto.output),
    TokensUsed: mapTokenUsage(dto.tokensUsed),
    DurationMs: toNum(dto.durationMs),
    Error: toStr(dto.error),
  };
}

function mapStepResult(dto: StepResultV2Dto | null | undefined): StepResult | null {
  if (!dto) return null;
  return {
    Output: toStr(dto.output),
    MemberReports: (dto.memberReports ?? []).map(mapMemberReport),
    TokensUsed: mapTokenUsage(dto.tokensUsed),
    DurationMs: toNum(dto.durationMs),
  };
}

function mapStepError(dto: StepErrorV2Dto | null | undefined): StepError | null {
  if (!dto) return null;
  return {
    Code: toStr(dto.code),
    Message: toStr(dto.message),
    Retryable: Boolean(dto.retryable),
    FailedMember: dto.failedMember ? mapMemberReport(dto.failedMember) : null,
  };
}

function mapPlanStep(dto: PlanStepV2Dto): PlanStep {
  return {
    ID: toStr(dto.id),
    PlanID: toStr(dto.planId),
    TaskID: toStr(dto.taskId),
    Label: toStr(dto.label),
    Description: toStr(dto.description),
    DependsOn: dto.dependsOn ?? [],
    MappedTeamStageID: toStr(dto.mappedTeamStageId),
    Status: (toStr(dto.status) || 'pending') as PlanStepStatus,
    AutoSynthesis: Boolean(dto.autoSynthesis),
    StartedAt: toStr(dto.startedAt),
    CompletedAt: toNullableStr(dto.completedAt),
    Seq: toNum(dto.seq),
    Version: toNum(dto.version),
    Result: mapStepResult(dto.result),
    Error: mapStepError(dto.error),
  };
}

function mapPlanBoard(dto: PlanBoardV2Dto): PlanBoard {
  return {
    ID: toStr(dto.id),
    TaskID: toStr(dto.taskId),
    TurnID: toStr(dto.turnId),
    SessionID: toStr(dto.sessionId),
    Strategy: (toStr(dto.strategy) || 'sequential') as PlanStrategy,
    Status: (toStr(dto.status) || 'planning') as PlanStatus,
    Steps: (dto.steps ?? []).map(mapPlanStep),
    StartedAt: toStr(dto.startedAt),
    CompletedAt: toNullableStr(dto.completedAt),
    Seq: toNum(dto.seq),
    Version: toNum(dto.version),
  };
}

function mapGraphNode(dto: GraphNodeV2Dto): GraphNode {
  return {
    ID: toStr(dto.id),
    GraphStageID: toStr(dto.graphStageId),
    Label: toStr(dto.label),
    DagNodeID: toStr(dto.dagNodeId),
    TeamStageID: toStr(dto.teamStageId),
    Status: (toStr(dto.status) || 'pending') as GraphNodeStatus,
    DependsOn: dto.dependsOn ?? [],
  };
}

function mapGraphStage(dto: GraphStageV2Dto): GraphStage {
  return {
    ID: toStr(dto.id),
    TaskID: toStr(dto.taskId),
    TurnID: toStr(dto.turnId),
    SessionID: toStr(dto.sessionId),
    PlanBoardID: toStr(dto.planBoardId),
    Nodes: (dto.nodes ?? []).map(mapGraphNode),
    Status: (toStr(dto.status) || 'running') as GraphStageStatus,
    StartedAt: toStr(dto.startedAt),
    CompletedAt: toNullableStr(dto.completedAt),
    Seq: toNum(dto.seq),
    Version: toNum(dto.version),
  };
}

// --- Public API ---

/** List team_stages for a task. Backend: GET /v2/tasks/{task_id}/team_stages. */
export async function listTeamStagesV2(taskId: string): Promise<TeamStage[]> {
  const resp = await kratosApi.get<ListTeamStagesV2Response>(`/v2/tasks/${encodeURIComponent(taskId)}/team_stages`);
  return (resp.data?.teamStages ?? []).map(mapTeamStage);
}

/** List team_runs for a team_stage. Backend: GET /v2/team_stages/{stage_id}/team_runs. */
export async function listTeamRunsV2(stageId: string): Promise<TeamRun[]> {
  const resp = await kratosApi.get<ListTeamRunsV2Response>(`/v2/team_stages/${encodeURIComponent(stageId)}/team_runs`);
  return (resp.data?.teamRuns ?? []).map(mapTeamRun);
}

/** List member_sessions for a team_run. Backend: GET /v2/team_runs/{run_id}/member_sessions. */
export async function listMemberSessionsV2(runId: string): Promise<MemberSession[]> {
  const resp = await kratosApi.get<ListMemberSessionsV2Response>(
    `/v2/team_runs/${encodeURIComponent(runId)}/member_sessions`,
  );
  return (resp.data?.memberSessions ?? []).map(mapMemberSession);
}

/** List Mode B orphan member_sessions for a spirit session. Backend: GET /v2/sessions/{session_id}/orphan_member_sessions. */
export async function listOrphanMemberSessionsV2(sessionId: string): Promise<MemberSession[]> {
  const resp = await kratosApi.get<ListMemberSessionsV2Response>(
    `/v2/sessions/${encodeURIComponent(sessionId)}/orphan_member_sessions`,
  );
  return (resp.data?.memberSessions ?? []).map(mapMemberSession);
}

/** List plan_boards for a task. Backend: GET /v2/tasks/{task_id}/plan_boards. */
export async function listPlanBoardsV2(taskId: string): Promise<PlanBoard[]> {
  const resp = await kratosApi.get<ListPlanBoardsV2Response>(`/v2/tasks/${encodeURIComponent(taskId)}/plan_boards`);
  return (resp.data?.planBoards ?? []).map(mapPlanBoard);
}

/** List plan_steps for a task. Backend: GET /v2/tasks/{task_id}/plan_steps. */
export async function listPlanStepsV2(taskId: string): Promise<PlanStep[]> {
  const resp = await kratosApi.get<ListPlanStepsV2Response>(`/v2/tasks/${encodeURIComponent(taskId)}/plan_steps`);
  return (resp.data?.planSteps ?? []).map(mapPlanStep);
}

/** List graph_stages for a task. Backend: GET /v2/tasks/{task_id}/graph_stages. */
export async function listGraphStagesV2(taskId: string): Promise<GraphStage[]> {
  const resp = await kratosApi.get<ListGraphStagesV2Response>(`/v2/tasks/${encodeURIComponent(taskId)}/graph_stages`);
  return (resp.data?.graphStages ?? []).map(mapGraphStage);
}

/** List graph_nodes for a graph_stage. Backend: GET /v2/graph_stages/{stage_id}/graph_nodes. */
export async function listGraphNodesV2(stageId: string): Promise<GraphNode[]> {
  const resp = await kratosApi.get<ListGraphNodesV2Response>(
    `/v2/graph_stages/${encodeURIComponent(stageId)}/graph_nodes`,
  );
  return (resp.data?.graphNodes ?? []).map(mapGraphNode);
}
