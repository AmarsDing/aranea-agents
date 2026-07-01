import type { ActivityEvent } from '../../realtime/activityEvent';
import {
  type ConversationSource,
  type ConversationTurnStatus,
  type DeliveryStatus,
  type DeliveryTarget,
  runStatusToTurnStatus,
  deliveryStatusFromChannelStatus,
} from '../../domain/conversation';
import { activitySource, activitySessionRevision } from './inboundSyncEnvelope';

export type ConversationEventScope = 'current-session' | 'inbox';

export type ConversationEventProjection = {
  key: string;
  scope: ConversationEventScope;
  sessionId: string;
  turnId: string;
  source: ConversationSource;
  revision: number;
  status?: ConversationTurnStatus;
  delivery?: DeliveryTarget;
  hydrate: boolean;
  stream: boolean;
};

export type ConversationEventDispatcherOptions = {
  currentSessionId?: string | null;
};

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function deliveryStatusFromMetadata(md: Record<string, unknown> | undefined): DeliveryStatus | undefined {
  const explicit = stringValue(md?.delivery_status);
  if (explicit) return deliveryStatusFromChannelStatus(explicit);
  const channelStatus = stringValue(md?.channel_delivery_status);
  if (channelStatus) return deliveryStatusFromChannelStatus(channelStatus);
  return undefined;
}

// ── ActivityEvent-based projection ─────────────────────────────────────
// The backend sends ALL chat/system events as ActivityEvent on the WS
// chat channel.

/**
 * Resolve a stable turn ID from an ActivityEvent.
 *
 * Field mapping (envelope → activity):
 *   env.turn_id                         → ev.activity.turn_id
 *   env.metadata.turn_id                → ev.activity.meta.turn_id
 *   env.metadata.run_id                 → ev.activity.meta.run_id
 *   env.request_id                      → ev.activity.meta.request_id
 *   env.id                              → ev.activity.id
 */
export function resolveActivityTurnId(ev: ActivityEvent): string {
  const meta = ev.activity.meta ?? {};
  const tid = typeof ev.activity.turn_id === 'string' ? ev.activity.turn_id.trim() : '';
  if (tid) return tid;
  const metaTurnId = stringValue(meta.turn_id);
  if (metaTurnId) return metaTurnId;
  const metaRunId = stringValue(meta.run_id);
  if (metaRunId) return metaRunId;
  const metaRequestId = stringValue(meta.request_id);
  if (metaRequestId) return metaRequestId;
  return ev.activity.id;
}

/**
 * Project an ActivityEvent into a {@link ConversationEventProjection}.
 *
 * Field mapping (envelope → activity):
 *   env.session_id                      → ev.activity.session_id
 *   env.type                            → ev.activity.stage + ev.event + ev.activity.kind
 *   env.metadata.status                 → ev.activity.meta.status
 *   env.metadata.*                      → ev.activity.meta.*
 *   env.timestamp                       → ev.activity.timestamp
 *   env.id                              → ev.activity.id
 */
export function projectConversationActivityEvent(
  ev: ActivityEvent,
  options: ConversationEventDispatcherOptions = {},
): ConversationEventProjection | null {
  const sessionId = (ev.activity.session_id ?? '').trim();
  if (!sessionId) return null;

  const source = conversationSourceFromActivity(ev);
  const revision = activitySessionRevision(ev);
  const status = turnStatusFromActivity(ev);
  const delivery = deliveryTargetFromActivity(ev);
  const turnId = resolveActivityTurnId(ev);
  const stage = ev.activity.stage;

  return {
    key: conversationActivityEventKey(ev, turnId, revision),
    scope: sessionId === options.currentSessionId ? 'current-session' : 'inbox',
    sessionId,
    turnId,
    source,
    revision,
    status,
    delivery,
    hydrate: shouldHydrateAfterActivity(ev, status, revision, stage),
    stream: isStreamActivity(ev),
  };
}

export function conversationActivityEventKey(ev: ActivityEvent, turnId?: string, revision = 0): string {
  const tid = turnId || resolveActivityTurnId(ev);
  return [ev.activity.session_id, tid, ev.activity.stage, ev.event, revision || '', ev.activity.id]
    .filter(Boolean)
    .join(':');
}

export function conversationSourceFromActivity(ev: ActivityEvent): ConversationSource {
  const raw = activitySource(ev).trim().toLowerCase();
  switch (raw) {
    case 'channel':
      return 'channel';
    case 'cron':
      return 'cron';
    case 'a2a':
      return 'a2a';
    case 'durable':
    case 'job':
    case 'background':
      return 'durable';
    case 'ws':
      return 'ws';
    default:
      return 'web';
  }
}

export function turnStatusFromActivity(ev: ActivityEvent): ConversationTurnStatus | undefined {
  if (ev.activity.stage === 'runner_completion') return 'completed';
  if (ev.event === 'failed') return 'failed';
  const meta = ev.activity.meta ?? {};
  const raw = stringValue(meta.status) || stringValue(meta.phase) || stringValue(meta.run_status);
  return raw ? runStatusToTurnStatus(raw) : undefined;
}

export function deliveryTargetFromActivity(ev: ActivityEvent): DeliveryTarget | undefined {
  const meta = ev.activity.meta;
  const status = deliveryStatusFromMetadata(meta);
  if (!status) return undefined;
  return {
    kind: 'channel',
    channelId: stringValue(meta?.channel_id),
    platform: stringValue(meta?.platform),
    recipientId: stringValue(meta?.recipient_id) || stringValue(meta?.peer_id),
    status,
    error: stringValue(meta?.error) || stringValue(meta?.error_message),
    updatedAt: ev.activity.timestamp,
  };
}

function shouldHydrateAfterActivity(
  ev: ActivityEvent,
  status: ConversationTurnStatus | undefined,
  revision: number,
  stage: string,
): boolean {
  if (stage === 'runner_completion') return true;
  if (status === 'completed' || status === 'failed' || status === 'cancelled') return true;
  return revision > 0 && stage === 'run_status';
}

function isStreamActivity(ev: ActivityEvent): boolean {
  // Chat-rendering streaming events: kind=task event=streaming, kind=action
  // (tool_call/tool_result). These are processed by the Activity timeline
  // directly, not by the inbound sync pipeline.
  return (
    (ev.activity.kind === 'task' && ev.event === 'streaming') ||
    ev.activity.kind === 'action' ||
    ev.activity.kind === 'thinking' ||
    ev.activity.kind === 'reply'
  );
}
