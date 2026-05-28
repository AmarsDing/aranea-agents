
import { createSessionService } from "../../services/index";
import type {
  ChatMessageRow,
  Session as KratosSession,
  SessionTimeline as KratosSessionTimeline,
  SessionTimelineItem as KratosTimelineItem,
  SessionTimelineSummary as KratosTimelineSummary,
  SessionTurn as KratosSessionTurn
} from "../../services/kratos/session/v1/index";
import { asRecord, pickStr } from "../../shared/wireJson";
import type {
  BatchOperationResult,
  BatchPreviewResult,
  Session,
  SessionBatchScope,
  SessionListResult,
  SessionParticipant,
  SessionRunRecord,
  SessionSearchQuery,
  SessionTimeline,
  SessionTimelineItem,
  SessionTimelineSummary,
  SessionTurn
} from "./types";
import type { Message } from "../../domain/types";
import { parseMessageOptions } from "../chat/parseMessageOptions";

export type {
  Session,
  SessionSearchQuery,
  SessionListResult,
  SessionTimelineItem,
  SessionTimelineSummary,
  SessionTimeline,
  SessionTurn,
  SessionRunRecord,
  SessionParticipant,
  SessionBatchScope,
  BatchPreviewResult,
  BatchOperationResult
} from "./types";

const sessionApi = createSessionService();

function kratosSessionToLegacy(s: KratosSession): Session {
  const r = asRecord(s as unknown);
  const title = pickStr(r, "title", "title") || (s.title ?? "");
  return {
    id: pickStr(r, "id", "id") || s.id || "",
    owner_type: pickStr(r, "owner_type", "ownerType") || s.ownerType || "",
    agent_id: pickStr(r, "agent_id", "agentId") || s.agentId || "",
    team_id: pickStr(r, "team_id", "teamId") || s.teamId || "",
    title,
    summary: pickStr(r, "summary", "summary") || s.summary || "",
    context_used_ratio: s.contextUsedRatio ?? 0,
    max_context_used_ratio: s.maxContextUsedRatio ?? 0,
    context_status: s.contextStatus ?? "",
    dialog_mode: s.dialogMode ?? "",
    provider: s.defaultProvider ?? "",
    model: s.defaultModel ?? "",
    status: s.status ?? "",
    message_count: s.messageCount ?? 0,
    run_count: s.runCount ?? 0,
    model_call_count: s.modelCallCount ?? 0,
    tool_call_count: s.toolCallCount ?? 0,
    skill_call_count: s.skillCallCount ?? 0,
    mcp_call_count: s.mcpCallCount ?? 0,
    input_tokens: s.inputTokens ?? 0,
    output_tokens: s.outputTokens ?? 0,
    total_tokens: s.totalTokens ?? 0,
    total_cost_micro_usd: Number(s.totalCostMicroUsd ?? 0),
    last_message_at: s.lastMessageAt ?? "",
    created_at: s.createdAt ?? "",
    updated_at: s.updatedAt ?? "",
    archived_at: s.archivedAt ?? "",
    deleted_at: s.deletedAt ?? "",
    pinned_at: s.pinnedAt ?? "",
    metadata_json: s.metadataJson ?? "",
    context_used_tokens: s.contextUsedTokens,
    last_context_window_tokens: s.lastContextWindowTokens
  };
}

function kratosTimelineSummaryToLegacy(sum?: KratosTimelineSummary): SessionTimelineSummary {
  return {
    total: sum?.total ?? 0,
    message_count: sum?.messageCount ?? 0,
    tool_count: sum?.toolCount ?? 0,
    skill_count: sum?.skillCount ?? 0,
    mcp_count: sum?.mcpCount ?? 0
  };
}

function kratosTimelineItemToLegacy(it: KratosTimelineItem): SessionTimelineItem {
  return {
    id: it.id ?? "",
    kind: (it.kind ?? "message") as SessionTimelineItem["kind"],
    side: (it.side ?? "left") as SessionTimelineItem["side"],
    title: it.title ?? "",
    subtitle: it.subtitle ?? "",
    actor_id: it.actorId ?? "",
    actor_name: it.actorName ?? "",
    status: it.status ?? "",
    occurred_at: it.occurredAt ?? "",
    duration_ms: it.durationMs ?? 0,
    content_markdown: it.contentMarkdown ?? "",
    preview: it.preview ?? "",
    detail_json: it.detailJson ?? "",
    tags: it.tags ?? []
  };
}

function kratosSessionTimelineToLegacy(data: KratosSessionTimeline): SessionTimeline {
  const items = (data.items ?? []).map(kratosTimelineItemToLegacy);
  return {
    session_id: data.sessionId ?? "",
    items,
    summary: kratosTimelineSummaryToLegacy(data.summary)
  };
}

export async function listSessions(agentID: string): Promise<Session[]> {
  const data = await searchSessions({ agent_id: agentID, limit: 200 });
  return data.items;
}

export async function listTeamSessions(teamID: string): Promise<Session[]> {
  const data = await searchSessions({ team_id: teamID, limit: 200 });
  return data.items;
}

export async function searchSessions(query: SessionSearchQuery = {}): Promise<SessionListResult> {
  const data = await sessionApi.SearchSessions({
    ownerType: query.owner_type,
    agentId: query.agent_id,
    teamId: query.team_id,
    status: query.status,
    contextStatus: query.context_status,
    keyword: query.keyword,
    limit: query.limit,
    offset: query.offset,
    page: query.page,
    pageSize: query.page_size,
    userId: undefined,
    sortBy: undefined,
    sortOrder: undefined
  });
  const items = (data.items ?? []).map(kratosSessionToLegacy);
  return {
    items,
    total: data.total ?? items.length,
    limit: data.limit ?? query.limit ?? query.page_size ?? items.length,
    offset: data.offset ?? query.offset ?? 0
  };
}

export async function getSession(id: string): Promise<Session> {
  const data = await sessionApi.GetSession({ id });
  return kratosSessionToLegacy(data);
}

export async function getSessionTimeline(
  id: string,
  params?: { limit?: number; offset?: number; kind_filter?: string; sort_order?: string }
): Promise<SessionTimeline> {
  const data = await sessionApi.GetSessionTimeline({
    id,
    limit: params?.limit,
    offset: params?.offset,
    kindFilter: params?.kind_filter,
    sortOrder: params?.sort_order,
  });
  return kratosSessionTimelineToLegacy(data);
}

export async function exportSession(id: string, format: "markdown" | "json"): Promise<{ content: string; filename: string; content_type: string }> {
  const data = await sessionApi.ExportSession({ id, format });
  return {
    content: data.content ?? "",
    filename: data.filename ?? `session.${format === "json" ? "json" : "md"}`,
    content_type: data.contentType ?? (format === "json" ? "application/json" : "text/markdown"),
  };
}

export async function listSessionRuns(
  sessionId: string,
  limit = 20,
  offset = 0
): Promise<{ items: SessionRunRecord[]; total: number }> {
  const data = await sessionApi.ListSessionRuns({ sessionId, limit, offset });
  const items = (data.items ?? []).map((row) => ({
    id: row.id ?? "",
    session_id: row.sessionId ?? "",
    turn_id: row.turnId ?? "",
    runtime_run_id: row.runtimeRunId ?? "",
    source: row.source ?? "",
    phase: row.phase ?? "",
    soft_budget_sec: row.softBudgetSec ?? 0,
    hard_budget_sec: row.hardBudgetSec ?? 0,
    checkpoint_id: row.checkpointId ?? "",
    workflow_job_id: row.workflowJobId ?? "",
    agent_id: row.agentId ?? "",
    error_message: row.errorMessage ?? "",
    started_at: row.startedAt ?? "",
    phase_changed_at: row.phaseChangedAt ?? "",
    finished_at: row.finishedAt ?? "",
    created_at: row.createdAt ?? "",
    updated_at: row.updatedAt ?? "",
  }));
  return { items, total: data.total ?? items.length };
}

export async function listSessionParticipants(sessionId: string): Promise<SessionParticipant[]> {
  const data = await sessionApi.ListSessionParticipants({ sessionId });
  return (data.items ?? []).map((row) => ({
    id: row.id ?? "",
    session_id: row.sessionId ?? "",
    participant_type: row.participantType ?? "",
    participant_id: row.participantId ?? "",
    display_name: row.displayName ?? "",
    role_in_session: row.roleInSession ?? "",
    status: row.status ?? "",
    first_active_at: row.firstActiveAt ?? "",
    last_active_at: row.lastActiveAt ?? "",
    message_count: row.messageCount ?? 0,
    run_step_count: row.runStepCount ?? 0,
    input_tokens: row.inputTokens ?? 0,
    output_tokens: row.outputTokens ?? 0,
    context_used_ratio: row.contextUsedRatio ?? 0,
    metadata_json: row.metadataJson ?? "",
    created_at: row.createdAt ?? "",
    updated_at: row.updatedAt ?? "",
  }));
}

export async function createSession(payload: {
  owner_type?: string;
  agent_id?: string;
  team_id?: string;
  title: string;
  dialog_mode?: string;
  default_provider?: string;
  default_model?: string;
  workspace_id?: string;
  user_id?: string;
}): Promise<Session> {
  const data = await sessionApi.CreateSession({
    ownerType: payload.owner_type,
    agentId: payload.agent_id,
    teamId: payload.team_id,
    title: payload.title,
    dialogMode: payload.dialog_mode,
    defaultProvider: payload.default_provider,
    defaultModel: payload.default_model,
    workspaceId: payload.workspace_id,
    userId: payload.user_id,
    tagsJson: undefined,
    metadataJson: undefined
  });
  return kratosSessionToLegacy(data);
}

export async function deleteSession(id: string): Promise<void> {
  await sessionApi.DeleteSession({ id });
}

export async function archiveSession(id: string): Promise<void> {
  await sessionApi.ArchiveSession({ id });
}

function toBatchScope(scope: SessionBatchScope = {}) {
  return {
    ownerType: scope.owner_type,
    agentId: scope.agent_id,
    teamId: scope.team_id,
    status: scope.status,
    contextStatus: scope.context_status,
    keyword: scope.keyword,
    userId: undefined
  };
}

export async function previewSessionBatch(payload: {
  mode: "archive" | "delete";
  ids?: string[];
  older_than_days?: number;
  scope?: SessionBatchScope;
  include_archived?: boolean;
}): Promise<BatchPreviewResult> {
  const data = await sessionApi.BatchPreviewSessions({
    mode: payload.mode,
    ids: payload.ids,
    olderThanDays: payload.older_than_days,
    scope: toBatchScope(payload.scope),
    includeArchived: payload.include_archived
  });
  return {
    matched: data.matched ?? 0,
    skipped_running: data.skippedRunning ?? 0,
    skipped_not_found: data.skippedNotFound ?? 0,
    truncated: data.truncated ?? false,
    sample_ids: data.sampleIds ?? []
  };
}

export async function batchArchiveSessions(payload: {
  ids?: string[];
  older_than_days?: number;
  scope?: SessionBatchScope;
}): Promise<BatchOperationResult> {
  const data = await sessionApi.BatchArchiveSessions({
    ids: payload.ids,
    olderThanDays: payload.older_than_days,
    scope: toBatchScope(payload.scope)
  });
  return {
    matched: data.matched ?? 0,
    processed: data.processed ?? 0,
    skipped_running: data.skippedRunning ?? 0,
    skipped_not_found: data.skippedNotFound ?? 0,
    truncated: data.truncated ?? false,
    failed_ids: data.failedIds ?? []
  };
}

export async function batchDeleteSessions(payload: {
  ids?: string[];
  older_than_days?: number;
  scope?: SessionBatchScope;
  include_archived?: boolean;
}): Promise<BatchOperationResult> {
  const data = await sessionApi.BatchDeleteSessions({
    ids: payload.ids,
    olderThanDays: payload.older_than_days,
    scope: toBatchScope(payload.scope),
    includeArchived: payload.include_archived
  });
  return {
    matched: data.matched ?? 0,
    processed: data.processed ?? 0,
    skipped_running: data.skippedRunning ?? 0,
    skipped_not_found: data.skippedNotFound ?? 0,
    truncated: data.truncated ?? false,
    failed_ids: data.failedIds ?? []
  };
}

export async function updateSessionTitle(id: string, title: string): Promise<Session> {
  const data = await sessionApi.UpdateSession({ id, title, tagsJson: undefined, visibility: undefined, metadataJson: undefined, dialogMode: undefined, defaultProvider: undefined, defaultModel: undefined });
  return kratosSessionToLegacy(data);
}

export async function clearAgentSessions(agentID: string): Promise<void> {
  await sessionApi.DeleteSessionsByAgent({ agentId: agentID });
}

function kratosChatRowToMessage(row: ChatMessageRow): Message {
  return {
    id: row.id ?? "",
    session_id: row.sessionId ?? "",
    parent_message_id: row.parentMessageId ?? "",
    turn_id: row.turnId ?? "",
    turn_number: row.turnNumber ?? 0,
    seq_in_turn: row.seqInTurn ?? 0,
    role: row.role ?? "",
    content_markdown: row.contentMarkdown ?? "",
    model_name: row.modelName ?? "",
    token_in: row.tokenIn ?? 0,
    token_out: row.tokenOut ?? 0,
    latency_ms: row.latencyMs ?? 0,
    status: row.status ?? "",
    attachments_count: row.attachmentsCount ?? 0,
    options_json: row.optionsJson ?? "",
    error_message: row.errorMessage ?? "",
    created_at: row.createdAt ?? "",
    ...parseMessageOptions(row.optionsJson ?? ""),
  };
}

/** Kratos `GET /v1/sessions/{id}/messages`（替代遗留 `/api/v1/chat/messages` 列表）。 */
export async function listSessionChatMessages(
  sessionID: string
): Promise<{ items: Message[]; currentRevision: number }> {
  const data = await sessionApi.ListSessionMessages({ id: sessionID, limit: undefined, offset: undefined });
  return {
    items: (data.items ?? []).map(kratosChatRowToMessage),
    currentRevision: Number(data.currentRevision ?? 0),
  };
}

/** Incremental sync: messages after session_revision (M55 CC-B-05). */
export async function listSessionChatMessagesAfterRevision(
  sessionID: string,
  afterRevision: number
): Promise<{ items: Message[]; currentRevision: number }> {
  const data = await sessionApi.ListSessionMessages({
    id: sessionID,
    afterRevision,
    limit: undefined,
    offset: undefined,
  });
  return {
    items: (data.items ?? []).map(kratosChatRowToMessage),
    currentRevision: Number(data.currentRevision ?? afterRevision),
  };
}

export type MessageSearchResult = {
  id: string;
  session_id: string;
  role: string;
  content_markdown: string;
  highlight: string;
  created_at: string;
};

/** `GET /v1/sessions/messages/search` — FTS5 全文检索（需 messages_fts 表）。 */
export async function searchSessionMessages(params: {
  sessionId?: string;
  keyword: string;
  limit?: number;
  offset?: number;
}): Promise<{ items: MessageSearchResult[]; total: number }> {
  const data = await sessionApi.SearchSessionMessages({
    sessionId: params.sessionId,
    keyword: params.keyword,
    limit: params.limit,
    offset: params.offset
  });
  const items = (data.items ?? []).map((row) => ({
    id: String(row.id ?? ""),
    session_id: String(row.sessionId ?? ""),
    role: String(row.role ?? ""),
    content_markdown: String(row.contentMarkdown ?? ""),
    highlight: String(row.highlight ?? ""),
    created_at: String(row.createdAt ?? "")
  }));
  return { items, total: Number(data.total ?? items.length) };
}

export async function restoreSession(id: string): Promise<Session> {
  const data = await sessionApi.RestoreSession({ id });
  return kratosSessionToLegacy(data);
}

export async function pinSession(id: string): Promise<Session> {
  const data = await sessionApi.PinSession({ id });
  return kratosSessionToLegacy(data);
}

export async function unpinSession(id: string): Promise<Session> {
  const data = await sessionApi.UnpinSession({ id });
  return kratosSessionToLegacy(data);
}

export async function updateSession(id: string, fields: { title?: string; tags_json?: string; visibility?: string; dialog_mode?: string; default_provider?: string; default_model?: string }): Promise<Session> {
  const data = await sessionApi.UpdateSession({
    id,
    title: fields.title,
    tagsJson: fields.tags_json,
    visibility: fields.visibility,
    dialogMode: fields.dialog_mode,
    defaultProvider: fields.default_provider,
    defaultModel: fields.default_model,
    metadataJson: undefined
  });
  return kratosSessionToLegacy(data);
}

function kratosSessionTurnToLegacy(t: KratosSessionTurn): SessionTurn {
  return {
    id: t.id ?? "",
    session_id: t.sessionId ?? "",
    run_id: t.runId ?? "",
    turn_number: t.turnNumber ?? 0,
    user_message_id: t.userMessageId ?? "",
    assistant_message_id: t.assistantMessageId ?? "",
    owner_type: t.ownerType ?? "",
    agent_id: t.agentId ?? "",
    team_id: t.teamId ?? "",
    status: t.status ?? "",
    started_at: t.startedAt ?? "",
    ended_at: t.endedAt ?? "",
    duration_ms: t.durationMs ?? 0,
    first_token_ms: t.firstTokenMs ?? 0,
    model_call_count: t.modelCallCount ?? 0,
    tool_call_count: t.toolCallCount ?? 0,
    skill_call_count: t.skillCallCount ?? 0,
    mcp_call_count: t.mcpCallCount ?? 0,
    input_tokens: t.inputTokens ?? 0,
    output_tokens: t.outputTokens ?? 0,
    total_tokens: t.totalTokens ?? 0,
    total_cost_micro_usd: Number(t.totalCostMicroUsd ?? 0),
    final_provider: t.finalProvider ?? "",
    final_model: t.finalModel ?? "",
    final_content_preview: t.finalContentPreview ?? "",
    error_code: t.errorCode ?? "",
    error_message: t.errorMessage ?? "",
    metadata_json: t.metadataJson ?? "",
    created_at: t.createdAt ?? "",
    updated_at: t.updatedAt ?? ""
  };
}

export async function listSessionTurns(sessionID: string, limit = 20, offset = 0): Promise<{ items: SessionTurn[]; total: number }> {
  const data = await sessionApi.ListSessionTurns({ sessionId: sessionID, limit, offset });
  return {
    items: (data.items ?? []).map(kratosSessionTurnToLegacy),
    total: data.total ?? 0
  };
}
