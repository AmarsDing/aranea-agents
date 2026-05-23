import type { Envelope } from "./envelope";
import type { ActivityKind, ToolUseEvent } from "./types";
import type { Message } from "./types";
import { classifyActivityKind } from "./activityPresentation";
import { toolEventToMessage } from "./toolEventMarkdown";

function parseJSONRecord(raw: string | undefined): Record<string, unknown> {
  if (!raw?.trim()) return {};
  try {
    const v = JSON.parse(raw) as unknown;
    return v !== null && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : { value: v };
  } catch {
    return { raw };
  }
}

function normalizeToolStatus(status: string): ToolUseEvent["status"] {
  const s = status.toLowerCase();
  if (s === "calling" || s === "running" || s === "in_progress") return "running";
  if (s === "failed" || s === "error") return "failed";
  if (s === "blocked") return "blocked";
  if (s === "cancelled") return "cancelled";
  return "success";
}

function activityMessageId(event: ToolUseEvent): string {
  if (event.id?.trim()) return `act-${event.id.trim()}`;
  return `tool-${event.agent_id || event.agent_key || "agent"}-${event.tool_name}`;
}

/** Build a {@link ToolUseEvent} from a tool_call / tool_result Envelope. */
export function envelopeToToolEvent(env: Envelope, phase: "before" | "after"): ToolUseEvent | null {
  const tc = env.tool_call;
  if (!tc?.name && !tc?.id) return null;
  const args = parseJSONRecord(tc.arguments_json);
  const result = parseJSONRecord(tc.result_json);
  const toolName = tc.name || "tool";
  const kind = (tc.activity_kind || classifyActivityKind(toolName)) as ActivityKind;
  return {
    id: tc.id || env.id,
    phase,
    status: normalizeToolStatus(tc.status || (phase === "before" ? "running" : "success")),
    agent_id: tc.agent_id || "",
    agent_key: tc.agent_key || env.author || "",
    agent_name: tc.agent_name || env.author || "Agent",
    agent_icon: "",
    tool_name: toolName,
    tool_label: tc.display_label || tc.name,
    arguments: args,
    result: phase === "after" || Object.keys(result).length > 0 ? result : undefined,
    error:
      typeof result.error === "string"
        ? result.error
        : tc.error_code && tc.status === "failed"
          ? tc.error_code
          : undefined,
    occurred_at: tc.finished_at || tc.started_at || env.timestamp || new Date().toISOString(),
    duration_ms: tc.duration_ms,
    is_long_running: tc.is_long_running,
    activity_kind: kind,
    display_label: tc.display_label,
    icon_key: tc.icon_key,
    summary: tc.summary,
    started_at: tc.started_at,
    finished_at: tc.finished_at,
    error_code: tc.error_code,
    run_id: tc.run_id,
    trace_id: tc.trace_id,
  };
}

export function mergeToolEvents(existing: ToolUseEvent, incoming: ToolUseEvent): ToolUseEvent {
  return {
    ...existing,
    ...incoming,
    id: incoming.id || existing.id,
    phase: incoming.phase || existing.phase,
    status: incoming.status || existing.status,
    tool_name: incoming.tool_name || existing.tool_name,
    tool_label: incoming.display_label || incoming.tool_label || existing.tool_label,
    display_label: incoming.display_label || existing.display_label,
    icon_key: incoming.icon_key || existing.icon_key,
    summary: incoming.summary || existing.summary,
    activity_kind: incoming.activity_kind || existing.activity_kind,
    arguments:
      Object.keys(incoming.arguments ?? {}).length > 0 ? incoming.arguments : existing.arguments,
    result: incoming.result ?? existing.result,
    error: incoming.error || existing.error,
    duration_ms: incoming.duration_ms ?? existing.duration_ms,
    is_long_running: incoming.is_long_running ?? existing.is_long_running,
    started_at: incoming.started_at || existing.started_at,
    finished_at: incoming.finished_at || existing.finished_at,
    error_code: incoming.error_code || existing.error_code,
    run_id: incoming.run_id || existing.run_id,
    trace_id: incoming.trace_id || existing.trace_id,
    agent_key: incoming.agent_key || existing.agent_key,
    agent_id: incoming.agent_id || existing.agent_id,
    agent_name: incoming.agent_name || existing.agent_name,
    occurred_at: incoming.occurred_at || existing.occurred_at,
    expanded: existing.expanded,
  };
}

export function upsertToolMessage(
  messages: Message[],
  sessionId: string,
  env: Envelope,
  phase: "before" | "after"
): Message[] {
  const event = envelopeToToolEvent(env, phase);
  if (!event) return messages;
  const row = toolEventToMessage(sessionId, event);
  const messageId = activityMessageId(event);
  row.id = messageId;
  const idx = messages.findIndex((m) => m.id === messageId || (event.id && m.id === `act-${event.id}`));
  if (idx >= 0) {
    const priorMeta = toolEventFromMessage(messages[idx]);
    const merged = priorMeta ? mergeToolEvents(priorMeta, event) : event;
    const nextRow = toolEventToMessage(sessionId, merged);
    nextRow.id = messageId;
    const next = [...messages];
    next[idx] = { ...messages[idx], ...nextRow, id: messageId };
    return next;
  }
  return [...messages, row];
}

/** Mark in-flight tool activity rows as cancelled when run_status=cancelled. */
export function cancelRunningToolMessages(messages: Message[], reason = "用户已停止生成"): Message[] {
  return patchOrphanToolMessages(messages, reason, "cancelled", "tool_cancelled", ["tool_running", "tool_blocked"]);
}

/** Mark orphan in-flight tool rows when a turn ends without tool_result. */
export function finalizeOrphanToolMessages(
  messages: Message[],
  reason = "Turn 已完成，未收到工具结果",
  statuses: string[] = ["tool_running", "tool_blocked"]
): Message[] {
  return patchOrphanToolMessages(messages, reason, "failed", "tool_failed", statuses);
}

function patchOrphanToolMessages(
  messages: Message[],
  reason: string,
  eventStatus: ToolUseEvent["status"],
  messageStatus: string,
  statuses: string[]
): Message[] {
  const allowed = new Set(statuses);
  let changed = false;
  const next = messages.map((msg) => {
    if (!allowed.has(msg.status || "")) return msg;
    const event = toolEventFromMessage(msg);
    if (!event) return msg;
    changed = true;
    const patched: ToolUseEvent = {
      ...event,
      status: eventStatus,
      phase: "after",
      error: event.error || reason,
      finished_at: new Date().toISOString(),
    };
    const row = toolEventToMessage(msg.session_id, patched);
    row.id = msg.id;
    return { ...msg, ...row, id: msg.id, status: messageStatus };
  });
  return changed ? next : messages;
}

export function toolEventFromMessage(message: Message): ToolUseEvent | null {
  try {
    const raw = JSON.parse(message.options_json || "{}") as { tool_event?: ToolUseEvent };
    return raw.tool_event ?? null;
  } catch {
    return null;
  }
}

export { activityMessageId };
