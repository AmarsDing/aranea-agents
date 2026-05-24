// Shared domain types - the single source of truth for types that cross
// feature boundaries (chat, session, graph, teams).
//
// Previously these types lived in features/chat/types.ts; they have been
// lifted to this shared location so that other features don't need to
// reach into the chat domain for shared domain models.
//
// Feature-specific types should stay in their own features/*/types.ts.

/** Core message model shared across chat, session, graph, and teams. */
export type Message = {
  id: string;
  session_id: string;
  parent_message_id: string;
  turn_index: number;
  role: string;
  content_markdown: string;
  model_name: string;
  token_in: number;
  token_out: number;
  latency_ms: number;
  status: string;
  attachments_count: number;
  options_json: string;
  error_message: string;
  created_at: string;
};

/** Run status values shared across chat, graph, and teams. */
export type RunStatusValue =
  | "idle"
  | "pending"
  | "running"
  | "awaiting_user"
  | "sync"
  | "completed"
  | "failed"
  | "cancelled";

/** Run status metadata shared across chat, graph, and teams. */
export interface RunStatus {
  runId: string;
  status: RunStatusValue;
  errorMessage: string;
  updatedAt: string;
  invocationId?: string;
  agentName?: string;
  startedAt?: string;
  lastEventAt?: string;
  eventCount?: number;
  awaitKind?: string;
  awaitToolKey?: string;
  awaitToolCallId?: string;
}
