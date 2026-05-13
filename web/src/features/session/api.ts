
import { createSessionService } from "../../services/index";
import type {
  ChatMessageRow,
  Session as KratosSession,
  SessionTimeline as KratosSessionTimeline,
  SessionTimelineItem as KratosTimelineItem,
  SessionTimelineSummary as KratosTimelineSummary
} from "../../services/kratos/session/v1/index";
import { asRecord, pickStr } from "../../shared/wireJson";
import type { Message } from "../chat/types";

const sessionApi = createSessionService();

export type Session = {
  id: string;
  owner_type: string;
  agent_id: string;
  team_id: string;
  title: string;
  summary: string;
  context_used_ratio: number;
  max_context_used_ratio: number;
  context_status: string;
  dialog_mode: string;
  provider: string;
  model: string;
  status: string;
  message_count: number;
  run_count: number;
  model_call_count: number;
  tool_call_count: number;
  skill_call_count: number;
  mcp_call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  last_message_at: string;
  created_at: string;
  updated_at: string;
  archived_at: string;
  deleted_at: string;
  context_used_tokens?: number;
  last_context_window_tokens?: number;
};

export type SessionSearchQuery = {
  owner_type?: string;
  agent_id?: string;
  team_id?: string;
  status?: string;
  context_status?: string;
  keyword?: string;
  limit?: number;
  offset?: number;
  page?: number;
  page_size?: number;
};

export type SessionListResult = {
  items: Session[];
  total: number;
  limit: number;
  offset: number;
};

export type SessionTimelineItem = {
  id: string;
  kind: "message" | "tool" | "skill" | "mcp" | string;
  side: "left" | "right" | string;
  title: string;
  subtitle: string;
  actor_id: string;
  actor_name: string;
  status: string;
  occurred_at: string;
  duration_ms: number;
  content_markdown: string;
  preview: string;
  detail_json: string;
  tags: string[];
};

export type SessionTimelineSummary = {
  total: number;
  message_count: number;
  tool_count: number;
  skill_count: number;
  mcp_count: number;
};

export type SessionTimeline = {
  session_id: string;
  items: SessionTimelineItem[];
  summary: SessionTimelineSummary;
};

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
    pageSize: query.page_size
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

export async function getSessionTimeline(id: string): Promise<SessionTimeline> {
  const data = await sessionApi.GetSessionTimeline({ id });
  return kratosSessionTimelineToLegacy(data);
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
    userId: payload.user_id
  });
  return kratosSessionToLegacy(data);
}

export async function deleteSession(id: string): Promise<void> {
  await sessionApi.DeleteSession({ id });
}

export async function archiveSession(id: string): Promise<void> {
  await sessionApi.ArchiveSession({ id });
}

export async function updateSessionTitle(id: string, title: string): Promise<Session> {
  const data = await sessionApi.UpdateSession({ id, title });
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
    turn_index: row.turnIndex ?? 0,
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
    created_at: row.createdAt ?? ""
  };
}

/** Kratos `GET /v1/sessions/{id}/messages`（替代遗留 `/api/v1/chat/messages` 列表）。 */
export async function listSessionChatMessages(sessionID: string): Promise<Message[]> {
  const data = await sessionApi.ListSessionMessages({ id: sessionID });
  return (data.items ?? []).map(kratosChatRowToMessage);
}
