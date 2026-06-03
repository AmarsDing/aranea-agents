// Shared domain types - the single source of truth for types that cross
// feature boundaries (chat, session, graph, teams).
//
// Previously these types lived in features/chat/types.ts; they have been
// lifted to this shared location so that other features don't need to
// reach into the chat domain for shared domain models.
//
// Feature-specific types should stay in their own features/*/types.ts.

/** Agent metadata embedded in message options_json. */
export type MessageAgentRef = {
  id: string;
  agent_key: string;
  name: string;
  icon: string;
};

/** Team member metadata embedded in message options_json. */
export type MessageTeamMemberRef = {
  agent_id: string;
  name: string;
  role: string;
  icon?: string;
};

/** Message source metadata for UserBubble badges (M55 CC-B-07). */
export type MessageSourceMeta = {
  source: 'web' | 'channel' | 'cron' | 'a2a' | 'api' | '';
  platform?: string;
  channelKey?: string;
};

/** Artifact attachment ref from user message (ART-01). */
export type MessageAttachmentRef = {
  id: string;
  name: string;
  mime_type: string;
  size?: number;
};

/** Message origin — replaces ID prefix conventions for lifecycle tracking. */
export type MessageOrigin =
  | { kind: 'persisted' }
  | { kind: 'pending_user'; localId: string }
  | { kind: 'streaming'; sessionId: string }
  | { kind: 'team_member'; agentKey: string }
  | { kind: 'tool_activity'; toolEventId: string };

/** Message status values — replaces bare string comparisons. */
export type MessageStatus =
  | 'ok'
  | 'streaming'
  | 'tool_running'
  | 'tool_blocked'
  | 'tool_success'
  | 'tool_failed'
  | 'tool_cancelled'
  | 'failed'
  | 'pending'
  | 'queued';

export const MESSAGE_STATUS = {
  OK: 'ok' as const,
  STREAMING: 'streaming' as const,
  TOOL_RUNNING: 'tool_running' as const,
  TOOL_BLOCKED: 'tool_blocked' as const,
  TOOL_SUCCESS: 'tool_success' as const,
  TOOL_FAILED: 'tool_failed' as const,
  TOOL_CANCELLED: 'tool_cancelled' as const,
  FAILED: 'failed' as const,
  PENDING: 'pending' as const,
  QUEUED: 'queued' as const,
};

export function isInFlightStatus(status: string): boolean {
  return (
    status === MESSAGE_STATUS.STREAMING ||
    status === MESSAGE_STATUS.TOOL_RUNNING ||
    status === MESSAGE_STATUS.TOOL_BLOCKED
  );
}

export function isToolStatus(status: string): boolean {
  return status.startsWith('tool_');
}

/** Core message model shared across chat, session, graph, and teams. */
export type Message = {
  id: string;
  session_id: string;
  parent_message_id: string;
  turn_id: string;
  turn_number: number;
  seq_in_turn: number;
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

  origin?: MessageOrigin;
  agent_ref?: MessageAgentRef | null;
  team_member?: MessageTeamMemberRef | null;
  source_meta?: MessageSourceMeta | null;
  reasoning_markdown?: string;
  dialog_mode?: string;
  provider?: string;
  model?: string;
  attachments?: MessageAttachmentRef[];
  tool_event?: unknown;
};

/** Run status values shared across chat, graph, and teams. */
export type RunStatusValue =
  | 'idle'
  | 'pending'
  | 'running'
  | 'awaiting_user'
  | 'sync'
  | 'completed'
  | 'failed'
  | 'cancelled';

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
