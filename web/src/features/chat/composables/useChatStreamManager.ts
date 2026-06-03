import { ref, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useChatStreamingSnapshots } from '../../../stores/chatStreamingSnapshots';
import { useAuthStore } from '../../../stores/auth';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import { useChatMessageStore } from '../../../stores/chat/messageStore';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { createChatStream, createTeamStream, type UseEnvelopeStreamReturn } from '../useEnvelopeStream';
import type { Envelope, EnvelopeType, WsUpstream } from '../envelope';
import { bindStreamHandlers, patchStreamingEnvelope } from '../streamHandlers';
import { getChannelWsCursor } from '../channelWsCursor';
import { reloadSessionAfterCompletion } from '../sessionCompletionReload';
import type { TeamRow } from '../../../components/chat/types';
import type { TeamDefinition } from '../../teams/types';

export type StreamManagerDeps = {
  sessionStore: ReturnType<typeof useChatSessionStore>;
  messageStore: ReturnType<typeof useChatMessageStore>;
  runtimeStore: ReturnType<typeof useChatRuntimeStore>;
  displayTeams: Ref<TeamRow[]>;
  resolveAgentId: () => string | undefined;
  markSendingDone: () => void;
  clearSendingTimeout: () => void;
  onRunAccepted: () => void;
  onRunStatus: (env: Envelope) => void;
  touchRunActivity: () => void;
  onFirstByteArrived: () => void;
  refreshRunStatus: (sessionId?: string) => Promise<void>;
  onCompressNotice?: (sessionId: string, prevRatio: number, newRatio: number) => void;
};

export function useChatStreamManager(deps: StreamManagerDeps) {
  const { t } = useI18n();
  const $q = useQuasar();
  const router = useRouter();
  const streamingSnapshots = useChatStreamingSnapshots();

  let chatStream: UseEnvelopeStreamReturn | null = null;
  let chatStreamSessionId: string | null = null;
  let teamStream: UseEnvelopeStreamReturn | null = null;
  let teamStreamSessionId: string | null = null;

  const wsReplaying = ref(false);
  let lastErrorNotifyMessage = '';
  let lastErrorNotifyAt = 0;

  function notifyError(message: string) {
    const now = Date.now();
    if (message === lastErrorNotifyMessage && now - lastErrorNotifyAt < 5000) {
      return;
    }
    lastErrorNotifyMessage = message;
    lastErrorNotifyAt = now;
    $q.notify({ type: 'negative', message, group: 'chat-stream-error' });
  }

  function notifyOrchestration(message: string) {
    $q.notify({ type: 'info', message, timeout: 4000, group: false });
  }

  async function reloadSessionMessagesAfterCompletion(sessionId: string) {
    try {
      await reloadSessionAfterCompletion({
        sessionStore: deps.sessionStore,
        messageStore: deps.messageStore,
        streamingSnapshots,
        sessionId,
        resolveAgentId: deps.resolveAgentId,
      });
    } catch (err) {
      notifyError(err instanceof Error ? err.message : t('chat.loadMessagesFailed', '加载消息失败'));
    }
  }

  function resolveTeamMemberMeta(agentKey: string) {
    const team = deps.displayTeams.value.find((row) => row.id === deps.sessionStore.selectedTeamId);
    let def: TeamDefinition | null = null;
    try {
      def = team?.definition_json ? (JSON.parse(team.definition_json) as TeamDefinition) : null;
    } catch {
      def = null;
    }
    const member = def?.members?.find((m) => m.agent_id === agentKey || m.name === agentKey);
    return {
      agent_key: agentKey,
      name: member?.name || agentKey,
      role: member?.role || '',
    };
  }

  function sessionContextHandlers(sessionId: string) {
    return {
      onSessionContextPatch: (sid: string, patch: Parameters<typeof deps.sessionStore.patchSessionMetricsLocal>[1]) => {
        deps.sessionStore.patchSessionMetricsLocal(sid, patch);
      },
      onCompressNotice: (sid: string, prevRatio: number, newRatio: number) => {
        deps.onCompressNotice?.(sid, prevRatio, newRatio);
      },
      getSessionMetrics: (sid: string) => {
        const row = deps.sessionStore.findSessionById(sid);
        if (!row) return undefined;
        return {
          total_tokens: row.total_tokens,
          max_context_used_ratio: row.max_context_used_ratio,
          input_tokens: row.input_tokens,
          output_tokens: row.output_tokens,
        };
      },
    };
  }

  function ensureChatStream(sessionId: string) {
    if (chatStream && chatStreamSessionId === sessionId) {
      deps.runtimeStore.setWsConnected(sessionId, chatStream.connected.value ?? false);
      if (!chatStream.connected.value) {
        chatStream.connect();
      }
      return chatStream;
    }
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
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
      },
      onReconnectFailed: () => {
        $q.notify({
          type: 'negative',
          message: t('chat.reconnectFailed', '连接已断开，请刷新页面重试'),
          timeout: 0,
          actions: [{ label: t('chat.refresh', '刷新页面'), color: 'white', handler: () => window.location.reload() }],
        });
      },
    });

    bindStreamHandlers(
      chatStream,
      {
        sessionId,
        resolveActiveSessionId: () => deps.sessionStore.selectedSession?.id ?? null,
        getMessages: (sid) => deps.messageStore.getMessages(sid),
        setMessages: (sid, rows) => deps.messageStore.setMessages(sid, rows),
        markSendingDone: deps.markSendingDone,
        clearSendingTimeout: deps.clearSendingTimeout,
        onRunAccepted: deps.onRunAccepted,
        onRunStatus: deps.onRunStatus,
        onErrorNotify: notifyError,
        onOrchestrationNotice: notifyOrchestration,
        onReloadAfterCompletion: reloadSessionMessagesAfterCompletion,
        ...sessionContextHandlers(sessionId),
        setLastIntentPass: (value) => {
          deps.messageStore.lastIntentPass = value;
        },
        onStreamingPatch: (sid, patch) => {
          if (patch.done) {
            streamingSnapshots.put(sid, {
              reasoning: patch.reasoning,
              partialText: patch.partialText,
              replace: true,
            });
            return;
          }
          streamingSnapshots.put(sid, {
            reasoning: patch.reasoning,
            partialText: patch.partialText,
          });
        },
        onRunActivity: deps.touchRunActivity,
        onFirstByteArrived: deps.onFirstByteArrived,
      },
      { batched: true },
    );

    chatStream.connect();
    chatStreamSessionId = sessionId;
    return chatStream;
  }

  function ensureTeamStream(sessionId: string) {
    if (teamStream && teamStream.transport.value && teamStreamSessionId === sessionId) {
      deps.runtimeStore.setWsConnected(sessionId, teamStream.connected.value ?? false);
      return teamStream;
    }
    teamStream?.disconnect();
    deps.runtimeStore.setWsConnected(sessionId, false);

    teamStream = createTeamStream(sessionId, {
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
      },
      onReconnectFailed: () => {
        $q.notify({
          type: 'negative',
          message: t('chat.reconnectFailed', '连接已断开，请刷新页面重试'),
          timeout: 0,
          actions: [{ label: t('chat.refresh', '刷新页面'), color: 'white', handler: () => window.location.reload() }],
        });
      },
      onConnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, true);
        void deps.refreshRunStatus(sessionId);
      },
    });

    bindStreamHandlers(teamStream, {
      sessionId,
      streamIdPrefix: 'ws-team-stream',
      resolveActiveSessionId: () => deps.sessionStore.teamSelectedSessionId,
      getMessages: (sid) => deps.messageStore.getMessages(sid),
      setMessages: (sid, rows) => deps.messageStore.setMessages(sid, rows),
      markSendingDone: deps.markSendingDone,
      clearSendingTimeout: deps.clearSendingTimeout,
      onRunAccepted: deps.onRunAccepted,
      onRunStatus: deps.onRunStatus,
      onErrorNotify: notifyError,
      onOrchestrationNotice: notifyOrchestration,
      onReloadAfterCompletion: reloadSessionMessagesAfterCompletion,
      ...sessionContextHandlers(sessionId),
      setLastIntentPass: (value) => {
        deps.messageStore.lastIntentPass = value;
      },
      resolveMemberMeta: resolveTeamMemberMeta,
      onRunActivity: deps.touchRunActivity,
      onFirstByteArrived: deps.onFirstByteArrived,
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

  function patchAgentMessages(sessionId: string, streamId: string, env: Envelope, isDone: boolean) {
    deps.messageStore.setMessages(
      sessionId,
      patchStreamingEnvelope(deps.messageStore.getMessages(sessionId), sessionId, streamId, env, isDone),
    );
  }

  function subscribeSessionStream(
    sessionId: string,
    ownerKind: 'agent' | 'team',
    types: EnvelopeType[],
    handler: (env: Envelope) => void,
  ): () => void {
    const stream = ownerKind === 'team' ? ensureTeamStream(sessionId) : ensureChatStream(sessionId);
    return stream.onType(types, handler);
  }

  return {
    wsReplaying,
    ensureChatStream,
    ensureTeamStream,
    subscribeSessionStream,
    sendChatViaWs,
    disconnectChatStream,
    disconnectTeamStream,
    disconnectAll,
    cancelActiveStream,
    patchAgentMessages,
  };
}
