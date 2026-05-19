import type { Envelope } from "./envelope";
import type { ToolUseEvent } from "./types";
import type { Message } from "./types";
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
  return "success";
}

/** Build a {@link ToolUseEvent} from a tool_call / tool_result Envelope. */
export function envelopeToToolEvent(env: Envelope, phase: "before" | "after"): ToolUseEvent | null {
  const tc = env.tool_call;
  if (!tc?.name && !tc?.id) return null;
  const args = parseJSONRecord(tc.arguments_json);
  const result = parseJSONRecord(tc.result_json);
  return {
    id: tc.id || env.id,
    phase,
    status: normalizeToolStatus(tc.status || (phase === "before" ? "running" : "success")),
    agent_id: "",
    agent_key: env.author || "",
    agent_name: env.author || "Agent",
    agent_icon: "",
    tool_name: tc.name,
    tool_label: tc.name,
    arguments: args,
    result: phase === "after" ? result : undefined,
    error: typeof result.error === "string" ? result.error : undefined,
    occurred_at: env.timestamp || new Date().toISOString(),
    duration_ms: tc.duration_ms,
    is_long_running: tc.is_long_running,
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
  const idx = messages.findIndex((m) => m.id === row.id);
  if (idx >= 0) {
    const next = [...messages];
    next[idx] = { ...next[idx], ...row };
    return next;
  }
  return [...messages, row];
}
