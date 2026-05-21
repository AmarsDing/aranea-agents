/**
 * Event 回放：**`createEventService()`** → **`GET /v1/events`**。
 */
import { createEventService } from "../../services";
import { asRecord, pickI32, pickStr } from "../../shared/wireJson";
import type { Envelope } from "../chat/envelope";

const eventApi = createEventService();

export type ListSessionEventsParams = {
  sessionId: string;
  since?: string;
  until?: string;
  type?: string;
  limit?: number;
  offset?: number;
};

export type ListSessionEventsResult = {
  items: Envelope[];
  total: number;
};

export async function listSessionEvents(params: ListSessionEventsParams): Promise<ListSessionEventsResult> {
  const data = await eventApi.ListEvents({
    sessionId: params.sessionId,
    since: params.since,
    until: params.until,
    type: params.type,
    limit: params.limit ?? 200,
    offset: params.offset ?? 0,
  });
  const items: Envelope[] = [];
  for (const row of data.items ?? []) {
    const parsed = parseEnvelopeRecord(row);
    if (parsed) items.push(parsed);
  }
  return { items, total: data.total ?? items.length };
}

function parseEnvelopeRecord(row: unknown): Envelope | null {
  const r = asRecord(row);
  const raw = pickStr(r, "envelope_json", "envelopeJson")?.trim();
  if (raw) {
    try {
      return JSON.parse(raw) as Envelope;
    } catch {
      return null;
    }
  }
  const id = pickStr(r, "id", "id");
  const type = pickStr(r, "type", "type");
  if (!id || !type) return null;
  return {
    id,
    type: type as Envelope["type"],
    author: pickStr(r, "author", "author") ?? "",
    session_id: pickStr(r, "session_id", "sessionId") ?? "",
    timestamp: pickStr(r, "created_at", "createdAt") ?? "",
    version: 1,
    channel: pickStr(r, "channel", "channel"),
  };
}

export function mapListEventsTotal(raw: unknown): number {
  const r = asRecord(raw);
  return pickI32(r, "total", "total") ?? 0;
}
