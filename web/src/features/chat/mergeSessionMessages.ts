import type { Message } from "./types";

function isEphemeralMessage(message: Message): boolean {
  const id = message.id || "";
  return (
    id.startsWith("ws-stream-") ||
    id.startsWith("ws-team-stream-") ||
    id.startsWith("pending-user-") ||
    id.startsWith("member-")
  );
}

function messageTime(message: Message): string {
  return message.created_at || "";
}

function messageOrder(message: Message): [number, string] {
  return [message.turn_index ?? 0, messageTime(message)];
}

function isInFlightLocalRow(message: Message): boolean {
  if (isEphemeralMessage(message)) return true;
  const status = message.status || "";
  return status === "tool_running" || status === "streaming" || status === "tool_blocked";
}

/** Merge server history with in-flight WS rows (streaming text, running tool cards). */
export function mergeSessionMessages(server: Message[], local: Message[]): Message[] {
  if (local.length === 0) return server;
  const serverById = new Map(server.map((m) => [m.id, m]));
  const merged = [...server];
  for (const row of local) {
    if (serverById.has(row.id)) continue;
    if (!isInFlightLocalRow(row)) continue;
    merged.push(row);
  }
  merged.sort((a, b) => {
    const [turnA, timeA] = messageOrder(a);
    const [turnB, timeB] = messageOrder(b);
    if (turnA !== turnB) return turnA - turnB;
    return timeA.localeCompare(timeB);
  });
  return merged;
}

export function isActivityMessage(message: Message): boolean {
  try {
    const raw = JSON.parse(message.options_json || "{}") as { schema?: string; tool_event?: unknown };
    return raw.schema === "chat.activity/v1" || Boolean(raw.tool_event);
  } catch {
    return message.status.startsWith("tool_");
  }
}
