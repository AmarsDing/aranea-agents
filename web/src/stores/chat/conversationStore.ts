import { computed, ref } from "vue";
import { defineStore } from "pinia";
import type {
  ConversationSession,
  ConversationTarget,
  ConversationTurnSummary,
  DeliveryTarget,
} from "../../domain/conversation";
import type { ConversationEventProjection } from "../../features/chat/conversationEventDispatcher";

export const useChatConversationStore = defineStore("chatConversation", () => {
  const currentTarget = ref<ConversationTarget | null>(null);
  const sessionsById = ref<Record<string, ConversationSession>>({});
  const turnsById = ref<Record<string, ConversationTurnSummary>>({});
  const sessionTurnIds = ref<Record<string, string[]>>({});
  const inboxSessionIds = ref<string[]>([]);

  const inboxSessions = computed(() =>
    inboxSessionIds.value
      .map((id) => sessionsById.value[id])
      .filter((session): session is ConversationSession => Boolean(session))
  );

  function setCurrentTarget(target: ConversationTarget | null) {
    currentTarget.value = target;
  }

  function upsertSession(session: ConversationSession) {
    sessionsById.value[session.id] = {
      ...sessionsById.value[session.id],
      ...session,
    };
  }

  function applyProjection(projection: ConversationEventProjection) {
    const prev = turnsById.value[projection.turnId];
    const deliveryTargets = mergeDeliveryTargets(prev?.deliveryTargets ?? [], projection.delivery);
    const next: ConversationTurnSummary = {
      id: projection.turnId,
      sessionId: projection.sessionId,
      status: projection.status ?? prev?.status ?? "running",
      source: projection.source,
      revision: Math.max(projection.revision, prev?.revision ?? 0),
      deliveryTargets,
      updatedAt: new Date().toISOString(),
    };
    turnsById.value[projection.turnId] = next;
    rememberSessionTurn(projection.sessionId, projection.turnId);

    const existing = sessionsById.value[projection.sessionId];
    sessionsById.value[projection.sessionId] = {
      id: projection.sessionId,
      title: existing?.title ?? "Untitled session",
      target: existing?.target ?? {
        type: "agent",
        id: "",
        source: projection.source,
      },
      unreadCount:
        projection.scope === "inbox"
          ? (existing?.unreadCount ?? 0) + 1
          : existing?.unreadCount ?? 0,
      pinnedAt: existing?.pinnedAt,
      source: existing?.source ?? projection.source,
      lastTurn: next,
    };

    if (projection.scope === "inbox") {
      addInboxSession(projection.sessionId);
    }
  }

  function markSessionRead(sessionId: string) {
    const session = sessionsById.value[sessionId];
    if (session) {
      sessionsById.value[sessionId] = { ...session, unreadCount: 0 };
    }
    inboxSessionIds.value = inboxSessionIds.value.filter((id) => id !== sessionId);
  }

  function clear() {
    currentTarget.value = null;
    sessionsById.value = {};
    turnsById.value = {};
    sessionTurnIds.value = {};
    inboxSessionIds.value = [];
  }

  function rememberSessionTurn(sessionId: string, turnId: string) {
    const ids = sessionTurnIds.value[sessionId] ?? [];
    if (ids.includes(turnId)) return;
    sessionTurnIds.value[sessionId] = [...ids, turnId];
  }

  function addInboxSession(sessionId: string) {
    inboxSessionIds.value = [
      sessionId,
      ...inboxSessionIds.value.filter((id) => id !== sessionId),
    ];
  }

  return {
    currentTarget,
    sessionsById,
    turnsById,
    sessionTurnIds,
    inboxSessionIds,
    inboxSessions,
    setCurrentTarget,
    upsertSession,
    applyProjection,
    markSessionRead,
    clear,
  };
});

function mergeDeliveryTargets(
  current: DeliveryTarget[],
  next?: DeliveryTarget
): DeliveryTarget[] {
  if (!next) return current;
  const key = deliveryKey(next);
  const without = current.filter((item) => deliveryKey(item) !== key);
  return [...without, next];
}

function deliveryKey(target: DeliveryTarget): string {
  return [
    target.kind,
    target.channelId ?? "",
    target.platform ?? "",
    target.recipientId ?? "",
  ].join(":");
}
