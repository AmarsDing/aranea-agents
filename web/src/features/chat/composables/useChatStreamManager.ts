import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../../../stores/auth';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { createChatStream, createTeamStream, type UseEnvelopeStreamReturn } from '../useEnvelopeStream';
import type { WsUpstream } from '../../../realtime/ws-transport';
import type { ActivityEvent } from '../../../realtime/activityEvent';
import type { V2WsEnvelope } from '../v2Types';
import { getChannelWsCursor } from '../channelWsCursor';

export type StreamManagerDeps = {
  runtimeStore: ReturnType<typeof useChatRuntimeStore>;
  /**
   * Activity-First (AF): called when an activity_event WS message arrives.
   * Replaces the legacy onActivityEnvelope for chat events. The handler
   * receives a business-semantic ActivityEvent with a full Activity snapshot.
   */
  onActivityEvent?: (ev: ActivityEvent) => void;
  /** v2 chat events: dispatched when a v2_event WS envelope arrives. */
  onV2Event?: (envelope: V2WsEnvelope) => void;
  refreshRunStatus: (sessionId?: string) => Promise<void>;
};

export function useChatStreamManager(deps: StreamManagerDeps) {
  const { t } = useI18n();
  const $q = useQuasar();
  const router = useRouter();

  let chatStream: UseEnvelopeStreamReturn | null = null;
  let chatStreamSessionId: string | null = null;
  let teamStream: UseEnvelopeStreamReturn | null = null;
  let teamStreamSessionId: string | null = null;

  const wsReplaying = ref(false);

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

    chatStream = createChatStream(sessionId, {
      lastEventId: getChannelWsCursor(sessionId),
      onConnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, true);
        void deps.refreshRunStatus(sessionId);
      },
      onDisconnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, false);
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
      onActivityEvent: deps.onActivityEvent,
      onV2Event: deps.onV2Event,
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
      },
      onActivityEvent: deps.onActivityEvent,
      onV2Event: deps.onV2Event,
    });

    teamStream.connect();
    teamStreamSessionId = sessionId;
    return teamStream;
  }

  function sendChatViaWs(stream: UseEnvelopeStreamReturn, upstream: WsUpstream): void {
    stream.connect();
    const transport = stream.transport.value;
    if (!transport) {
      throw new Error('WebSocket transport unavailable');
    }
    // Delegate to transport.send — it already handles non-OPEN state correctly
    // by enqueuing to businessQueue (never dropped) and flushing on ws.onopen.
    // This guarantees the backend WS subscription (setupEventSubscription runs
    // after handshake) is ready before the user_message is delivered, so the
    // subsequent ActivityEvents flow back through the same WS.
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
