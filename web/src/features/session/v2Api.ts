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
import type { Step, StepKind, StepStatus, Task, TaskStatus, Turn, TurnStatus } from '../chat/v2Types';

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
function decodeBytesJson(v: string | null | undefined): unknown | null {
  if (v === undefined || v === null || v === '') return null;
  // Already-decoded object (defensive — gateway may pre-parse).
  if (typeof v === 'object') return v;
  // Try base64 → UTF-8 string → JSON.parse.
  try {
    const decoded = atob(v);
    if (!decoded) return null;
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
