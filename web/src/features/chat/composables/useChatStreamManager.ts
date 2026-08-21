import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../../../stores/auth';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { createChatStream, createTeamStream } from '../useEnvelopeStream';
import type { WsSessionStream } from '../../../realtime/createWsSessionStream';
import type { WsUpstream } from '../../../realtime/ws-transport';
import type { V2WsEnvelope } from '../v2Types';
import { getChannelWsCursor } from '../channelWsCursor';

export type StreamManagerDeps = {
  runtimeStore: ReturnType<typeof useChatRuntimeStore>;
  /** v2 chat events: dispatched when a v2_event WS envelope arrives. */
  onV2Event?: (envelope: V2WsEnvelope) => void;
  refreshRunStatus: (sessionId?: string) => Promise<void>;
  /**
   * B-06: authoritative v2 snapshot hydration after WS reconnect.
   * Server no longer replays events; clients must re-fetch REST history.
   */
  onReconnectHydrate?: (sessionId: string) => Promise<void>;
};

export function useChatStreamManager(deps: StreamManagerDeps) {
  const { t } = useI18n();
  const $q = useQuasar();
  const router = useRouter();

  let chatStream: WsSessionStream | null = null;
  let chatStreamSessionId: string | null = null;
  let teamStream: WsSessionStream | null = null;
  let teamStreamSessionId: string | null = null;

  /** True while reconnect snapshot hydrate is in flight (B-06). */
  const wsReplaying = ref(false);
  /** Per-session: true after a disconnect so next onConnected triggers hydrate. */
  const needsHydrateAfterReconnect = new Map<string, boolean>();

  async function hydrateAfterReconnect(sessionId: string) {
    if (!deps.onReconnectHydrate) return;
    wsReplaying.value = true;
    try {
      await deps.onReconnectHydrate(sessionId);
    } catch (e) {
      console.warn('[chat] reconnect v2 hydrate failed', e);
    } finally {
      wsReplaying.value = false;
    }
  }

  /**
   * F1（2026-08-21 全链路审查）：业务消息因发送队列满被 transport 丢弃时
   * 的通知接线。此前全链路无人传 onDrop，user_message 会静默丢失——用户
   * 看到消息已发出但后端从未收到。现在至少明示用户重发，并在下次连接后
   * 触发 REST 水合以对齐时间线。
   */
  function handleUpstreamDrop(sessionId: string) {
    return (upstream: WsUpstream) => {
      if (upstream.type === 'user_message') {
        $q.notify({
          type: 'warning',
          message: t('chat.wsQueueFullDropped', '连接拥塞，消息未能送达，请重新发送'),
        });
      }
      needsHydrateAfterReconnect.set(sessionId, true);
    };
  }

  function ensureChatStream(sessionId: string) {
    if (chatStream && chatStreamSessionId === sessionId) {
      // Only sync wsConnected→true when the stream reports connected;
      // never downgrade to false here — onDisconnected is the authoritative
      // source for disconnection and avoids stale ref reads after onError.
      if (chatStream.connected.value) {
        deps.runtimeStore.setWsConnected(sessionId, true);
      }
      if (!chatStream.connected.value) {
        chatStream.connect();
      }
      return chatStream;
    }
    // B-02: Disconnect previous stream before creating a new one.
    chatStream?.disconnect();
    deps.runtimeStore.setWsConnected(sessionId, false);
    needsHydrateAfterReconnect.delete(sessionId);

    chatStream = createChatStream(sessionId, {
      lastEventId: getChannelWsCursor(sessionId),
      onConnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, true);
        void deps.refreshRunStatus(sessionId);
        // B-06: after a disconnect, re-hydrate authoritative v2 snapshot.
        // Server also replays missed critical outbox frames via last_event_id;
        // REST hydrate remains the safety net for non-critical state.
        if (needsHydrateAfterReconnect.get(sessionId)) {
          needsHydrateAfterReconnect.set(sessionId, false);
          void hydrateAfterReconnect(sessionId);
        }
      },
      onDisconnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, false);
        needsHydrateAfterReconnect.set(sessionId, true);
      },
      onServerShutdown: () => {
        $q.notify({
          type: 'warning',
          message: t('chat.serverShutdown', '服务器已关闭，请重新登录'),
          timeout: 0,
          actions: [{ label: t('chat.relogin', '重新登录'), color: 'white', handler: () => {} }],
        });
        const auth = useAuthStore();
        auth.user = null;
        auth.sessionChecked = true;
        router.push({ name: 'login' });
      },
      onV2Event: deps.onV2Event,
      onDrop: handleUpstreamDrop(sessionId),
    });

    chatStream.connect();
    chatStreamSessionId = sessionId;
    return chatStream;
  }

  function ensureTeamStream(sessionId: string) {
    if (teamStream && teamStreamSessionId === sessionId) {
      // Only sync wsConnected→true; never downgrade — onDisconnected is authoritative.
      if (teamStream.connected.value) {
        deps.runtimeStore.setWsConnected(sessionId, true);
      }
      // B-01: Reconnect if the transport exists but is disconnected.
      if (!teamStream.connected.value) {
        teamStream.connect();
      }
      return teamStream;
    }
    // B-02: Disconnect previous stream before creating a new one.
    teamStream?.disconnect();
    deps.runtimeStore.setWsConnected(sessionId, false);
    needsHydrateAfterReconnect.delete(sessionId);

    teamStream = createTeamStream(sessionId, {
      onServerShutdown: () => {
        $q.notify({
          type: 'warning',
          message: t('chat.serverShutdown', '服务器已关闭，请重新登录'),
          timeout: 0,
          actions: [{ label: t('chat.relogin', '重新登录'), color: 'white', handler: () => {} }],
        });
        const auth = useAuthStore();
        auth.user = null;
        auth.sessionChecked = true;
        router.push({ name: 'login' });
      },
      onConnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, true);
        void deps.refreshRunStatus(sessionId);
        if (needsHydrateAfterReconnect.get(sessionId)) {
          needsHydrateAfterReconnect.set(sessionId, false);
          void hydrateAfterReconnect(sessionId);
        }
      },
      onDisconnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, false);
        needsHydrateAfterReconnect.set(sessionId, true);
      },
      onV2Event: deps.onV2Event,
      onDrop: handleUpstreamDrop(sessionId),
    });

    teamStream.connect();
    teamStreamSessionId = sessionId;
    return teamStream;
  }

  function sendChatViaWs(stream: WsSessionStream, upstream: WsUpstream): void {
    stream.connect();
    const transport = stream.transport.value;
    if (!transport) {
      throw new Error('WebSocket transport unavailable');
    }
    // Delegate to transport.send — it already handles non-OPEN state correctly
    // by enqueuing to businessQueue (never dropped) and flushing on ws.onopen.
    // This guarantees the backend WS subscription (setupEventSubscription runs
    // after handshake) is ready before the user_message is delivered, so the
    // subsequent v2_event frames flow back through the same WS.
    // Do NOT throw on !transport.connected — that would force HTTP fallback
    // and create a subscription race (HTTP delivers immediately but the WS
    // subscription isn't set up yet, so emitted events are missed → UI blank
    // until page refresh).
    transport.send(upstream);
  }

  function disconnectChatStream() {
    if (chatStreamSessionId) {
      deps.runtimeStore.setWsConnected(chatStreamSessionId, false);
    }
    chatStream?.disconnect();
    chatStream = null;
    chatStreamSessionId = null;
  }

  function disconnectTeamStream() {
    if (teamStreamSessionId) {
      deps.runtimeStore.setWsConnected(teamStreamSessionId, false);
    }
    teamStream?.disconnect();
    teamStream = null;
    teamStreamSessionId = null;
  }

  function disconnectAll() {
    disconnectChatStream();
    disconnectTeamStream();
  }

  function cancelActiveStream() {
    chatStream?.cancel();
    teamStream?.cancel();
  }

  return {
    wsReplaying,
    ensureChatStream,
    ensureTeamStream,
    sendChatViaWs,
    disconnectChatStream,
    disconnectTeamStream,
    disconnectAll,
    cancelActiveStream,
  };
}
