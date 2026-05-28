import type { Envelope } from "../../realtime/envelope";
import { resolveEnvelopeTurnId, resolveEnvelopeSource, resolveEnvelopeRevision } from "../../realtime/envelope";
import {
  type ConversationSource,
  type ConversationTurnStatus,
  type DeliveryStatus,
  type DeliveryTarget,
  runStatusToTurnStatus,
  deliveryStatusFromChannelStatus,
} from "../../domain/conversation";

export type ConversationEventScope = "current-session" | "inbox";

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

export function projectConversationEnvelope(
  env: Envelope,
  options: ConversationEventDispatcherOptions = {}
): ConversationEventProjection | null {
  const sessionId = (env.session_id ?? "").trim();
  if (!sessionId) return null;

  const source = conversationSourceFromEnvelope(env);
  const revision = resolveEnvelopeRevision(env);
  const status = turnStatusFromEnvelope(env);
  const delivery = deliveryTargetFromEnvelope(env);
  const turnId = resolveEnvelopeTurnId(env);

  return {
    key: conversationEventKey(env, turnId, revision),
    scope: sessionId === options.currentSessionId ? "current-session" : "inbox",
    sessionId,
    turnId,
    source,
    revision,
    status,
    delivery,
    hydrate: shouldHydrateAfterEnvelope(env, status, revision),
    stream: isStreamEnvelope(env),
  };
}

export function conversationEventKey(env: Envelope, turnId?: string, revision = 0): string {
  const tid = turnId || resolveEnvelopeTurnId(env);
  return [env.session_id, tid, env.type, revision || "", env.id].filter(Boolean).join(":");
}

export function conversationSourceFromEnvelope(env: Envelope): ConversationSource {
  const raw = resolveEnvelopeSource(env).trim().toLowerCase();
  switch (raw) {
    case "channel":
      return "channel";
    case "cron":
      return "cron";
    case "a2a":
      return "a2a";
    case "durable":
    case "job":
    case "background":
      return "durable";
    case "ws":
      return "ws";
    default:
      return "web";
  }
}

export function turnStatusFromEnvelope(env: Envelope): ConversationTurnStatus | undefined {
  if (env.type === "runner_completion") return "completed";
  if (env.type === "error") return "failed";
  const raw =
    stringValue(metadataValue(env, "status")) ||
    stringValue(metadataValue(env, "phase")) ||
    stringValue(metadataValue(env, "run_status"));
  return raw ? runStatusToTurnStatus(raw) : undefined;
}

export function deliveryTargetFromEnvelope(env: Envelope): DeliveryTarget | undefined {
  const md = env.metadata;
  const status = deliveryStatusFromMetadata(md);
  if (!status) return undefined;
  return {
    kind: "channel",
    channelId: stringValue(metadataValue(env, "channel_id")),
    platform: stringValue(metadataValue(env, "platform")),
    recipientId: stringValue(metadataValue(env, "recipient_id")) || stringValue(metadataValue(env, "peer_id")),
    status,
    error: stringValue(metadataValue(env, "error")) || stringValue(metadataValue(env, "error_message")),
    updatedAt: env.timestamp,
  };
}

function shouldHydrateAfterEnvelope(env: Envelope, status: ConversationTurnStatus | undefined, revision: number): boolean {
  if (env.type === "runner_completion") return true;
  if (status === "completed" || status === "failed" || status === "cancelled") return true;
  return revision > 0 && env.type === "run_status";
}

function isStreamEnvelope(env: Envelope): boolean {
  return (
    env.type === "text_delta" ||
    env.type === "text_done" ||
    env.type === "tool_call" ||
    env.type === "tool_result" ||
    env.type === "member_delta"
  );
}

function deliveryStatusFromMetadata(md: Record<string, unknown> | undefined): DeliveryStatus | undefined {
  const explicit = stringValue(md?.delivery_status);
  if (explicit) return deliveryStatusFromChannelStatus(explicit);
  const channelStatus = stringValue(md?.channel_delivery_status);
  if (channelStatus) return deliveryStatusFromChannelStatus(channelStatus);
  return undefined;
}

function metadataValue(env: Envelope, key: string): unknown {
  return env.metadata?.[key];
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}
