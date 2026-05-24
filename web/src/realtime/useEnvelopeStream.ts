/**
 * Shared envelope stream composable — lifted from features/chat/ so that
 * any feature (chat, teams, monitor, graph, orchestration) can use it
 * without creating a cross-feature dependency on the Chat module.
 *
 * Chat-specific helpers (useChatStream, useTeamStream, useMonitorStream,
 * useGraphStream) remain in their respective feature directories.
 */
import { onUnmounted, ref, shallowRef } from "vue";
import { createWsTransport, type WsTransport } from "./ws-transport";
import { EnvelopeDispatcher } from "./dispatcher";
import type { Envelope, EnvelopeType } from "./envelope";
import {
  acquireGlobalWsConsumer,
  globalWsConsumerEnableLog,
  globalWsConsumerSubscribe,
  globalWsConsumerUnsubscribe,
  releaseGlobalWsConsumer,
  shouldUseGlobalWsHub,
} from "./globalWsHub";
export type {
  GraphNodeState,
  GraphExecutionState,
  GraphStreamInterrupt,
  GraphStreamExecutionSummary,
} from "./graphState";
import type {
  GraphNodeState,
  GraphExecutionState,
  GraphStreamInterrupt,
  GraphStreamExecutionSummary,
} from "./graphState";

export type UseEnvelopeStreamOptions = {
  sessionId: string;
  channels?: string[];
  lastEventId?: string;
  autoConnect?: boolean;
  logEnabled?: boolean;
  onConnected?: (info: { sessionId: string; lastEventId?: string }) => void;
  onDisconnected?: () => void;
  onServerShutdown?: (reason: string) => void;
  onReplayState?: (replaying: boolean, count?: number) => void;
  onReconnectFailed?: () => void;
};

export type UseEnvelopeStreamReturn = {
  connected: ReturnType<typeof ref<boolean>>;
  wsReplaying: ReturnType<typeof ref<boolean>>;
  lastEventId: ReturnType<typeof ref<string | undefined>>;
  transport: ReturnType<typeof shallowRef<WsTransport | null>>;
  dispatcher: EnvelopeDispatcher;
  connect: () => void;
  disconnect: () => void;
  onType: (type: EnvelopeType | EnvelopeType[], handler: (env: Envelope) => void) => () => void;
  onChannel: (channel: string | string[], handler: (env: Envelope) => void) => () => void;
  subscribe: (channel: string) => void;
  unsubscribe: (channel: string) => void;
  enableLog: (enabled: boolean) => void;
  cancel: () => void;
};

/** Factory for session streams; safe to call outside component `setup()` (e.g. on session select). */
export function createEnvelopeStream(opts: UseEnvelopeStreamOptions): UseEnvelopeStreamReturn {
  const connected = ref(false);
  const wsReplaying = ref(false);
  const lastEventId = ref<string | undefined>(opts.lastEventId);
  const transport = shallowRef<WsTransport | null>(null);
  const dispatcher = new EnvelopeDispatcher();

  const channels = opts.channels ?? ["chat", "system"];
  let globalHubId: string | null = null;

  function connect(): void {
    if (globalHubId) return;

    if (shouldUseGlobalWsHub(opts.sessionId, lastEventId.value)) {
      globalHubId = acquireGlobalWsConsumer({
        channels,
        logEnabled: opts.logEnabled ?? false,
        onEnvelope: (env) => dispatcher.dispatch(env),
        onConnected: () => {
          connected.value = true;
          opts.onConnected?.({ sessionId: opts.sessionId, lastEventId: lastEventId.value });
        },
        onDisconnected: () => {
          connected.value = false;
          opts.onDisconnected?.();
        },
        onServerShutdown: (reason) => {
          opts.onServerShutdown?.(reason);
        },
      });
      return;
    }

    if (transport.value?.connected) return;
    if (transport.value) {
      transport.value.connect();
      return;
    }

    const t = createWsTransport({
      sessionId: opts.sessionId,
      lastEventId: lastEventId.value,
      logEnabled: opts.logEnabled,
      onEnvelope: (env) => {
        dispatcher.dispatch(env);
      },
      onConnected: (info) => {
        connected.value = true;
        lastEventId.value = info.lastEventId;
        for (const ch of channels) {
          if (ch !== "chat" && ch !== "system") {
            t.subscribe(ch);
          }
        }
        opts.onConnected?.(info);
      },
      onDisconnected: () => {
        connected.value = false;
        opts.onDisconnected?.();
      },
      onError: () => {
        connected.value = false;
      },
      onServerShutdown: (reason) => {
        opts.onServerShutdown?.(reason);
      },
      onReplayState: (replaying, count) => {
        wsReplaying.value = replaying;
        opts.onReplayState?.(replaying, count);
      },
      onReconnectFailed: () => {
        opts.onReconnectFailed?.();
      },
    });

    transport.value = t;
    t.connect();
  }

  function disconnect(): void {
    if (globalHubId) {
      releaseGlobalWsConsumer(globalHubId);
      globalHubId = null;
      connected.value = false;
      dispatcher.clear();
      return;
    }
    transport.value?.disconnect();
    transport.value = null;
    connected.value = false;
    dispatcher.clear();
  }

  function onType(type: EnvelopeType | EnvelopeType[], handler: (env: Envelope) => void): () => void {
    return dispatcher.onType(type, handler);
  }

  function onChannel(channel: string | string[], handler: (env: Envelope) => void): () => void {
    return dispatcher.onChannel(channel, handler);
  }

  function subscribe(channel: string): void {
    if (globalHubId) {
      globalWsConsumerSubscribe(globalHubId, channel);
      return;
    }
    transport.value?.subscribe(channel);
  }

  function unsubscribe(channel: string): void {
    if (globalHubId) {
      globalWsConsumerUnsubscribe(globalHubId, channel);
      return;
    }
    transport.value?.unsubscribe(channel);
  }

  function cancel(): void {
    transport.value?.cancel();
  }

  function enableLog(enabled: boolean): void {
    if (globalHubId) {
      globalWsConsumerEnableLog(globalHubId, enabled);
      return;
    }
    transport.value?.enableLog(enabled);
  }

  if (opts.autoConnect !== false) {
    connect();
  }

  return {
    connected,
    wsReplaying,
    lastEventId,
    transport,
    dispatcher,
    connect,
    disconnect,
    onType,
    onChannel,
    subscribe,
    unsubscribe,
    enableLog,
    cancel,
  };
}

export function useEnvelopeStream(opts: UseEnvelopeStreamOptions): UseEnvelopeStreamReturn {
  const stream = createEnvelopeStream({ ...opts, autoConnect: false });
  if (opts.autoConnect !== false) {
    stream.connect();
  }
  onUnmounted(() => {
    stream.disconnect();
  });
  return stream;
}
