/**
 * Shared envelope stream composable — lifted from features/chat/ so that
 * any feature (chat, teams, monitor, graph, orchestration) can use it
 * without creating a cross-feature dependency on the Chat module.
 *
 * Chat-specific helpers (useChatStream, useTeamStream, useMonitorStream,
 * useGraphStream) remain in their respective feature directories.
 *
 * @deprecated For chat sessions, prefer the ActivityEvent-based stream
 * (realtime/activityEvent.ts + useActivityTimeline). This envelope stream
 * is retained for non-chat features until they migrate. See ADR-02 §遗留项.
 */
import { onUnmounted, ref, shallowRef, type Ref } from 'vue';
import { createWsTransport, type MonitorBackpressurePayload, type WsTransport } from './ws-transport';
import type { ActivityEvent } from './activityEvent';
import type { MonitorEvent } from './monitorEvent';
import type { V2WsEnvelope } from '../features/chat/v2Types';
import {
  acquireGlobalWsConsumer,
  globalWsConsumerEnableLog,
  globalWsConsumerSubscribe,
  globalWsConsumerUnsubscribe,
  releaseGlobalWsConsumer,
  shouldUseGlobalWsHub,
} from './globalWsHub';
export type {
  GraphNodeState,
  GraphExecutionState,
  GraphStreamInterrupt,
  GraphStreamExecutionSummary,
} from './graphState';

export type UseEnvelopeStreamOptions = {
  sessionId: string;
  channels?: string[];
  lastEventId?: string;
  autoConnect?: boolean;
  logEnabled?: boolean;
  onConnected?: (info: { sessionId: string; lastEventId?: string }) => void;
  onDisconnected?: () => void;
  onServerShutdown?: (reason: string) => void;
  /**
   * Activity-First (AF): called when a downstream message carries an
   * activity_event payload. If not provided, activity_event messages are
   * silently ignored by the transport.
   */
  onActivityEvent?: (ev: ActivityEvent) => void;
  /**
   * Monitor channel: called when a downstream message carries a
   * monitor_event payload (log, flow_log, mcp, alert). If not provided,
   * monitor_event messages are silently ignored by the transport.
   */
  onMonitorEvent?: (event: MonitorEvent) => void;
  /** MON-OPT-04 backpressure notification from the server send queues. */
  onBackpressure?: (payload: MonitorBackpressurePayload) => void;
  /**
   * v2 chat events: called when a downstream message carries a v2_event
   * envelope. If not provided, v2_event messages are silently ignored.
   */
  onV2Event?: (envelope: V2WsEnvelope) => void;
};

export type UseEnvelopeStreamReturn = {
  connected: Ref<boolean>;
  wsReplaying: Ref<boolean>;
  lastEventId: Ref<string | undefined>;
  transport: Ref<WsTransport | null>;
  connect: () => void;
  disconnect: () => void;
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

  const channels = opts.channels ?? ['chat', 'system'];
  let globalHubId: string | null = null;

  function connect(): void {
    if (globalHubId) {
      return;
    }

    if (shouldUseGlobalWsHub(opts.sessionId, lastEventId.value)) {
      globalHubId = acquireGlobalWsConsumer({
        channels,
        logEnabled: opts.logEnabled ?? false,
        onActivityEvent: opts.onActivityEvent,
        onMonitorEvent: opts.onMonitorEvent,
        onBackpressure: opts.onBackpressure,
        onV2Event: opts.onV2Event,
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

    if (transport.value?.connected) {
      return;
    }
    if (transport.value) {
      transport.value.connect();
      return;
    }

    const t = createWsTransport({
      sessionId: opts.sessionId,
      lastEventId: lastEventId.value,
      logEnabled: opts.logEnabled,
      onActivityEvent: opts.onActivityEvent ? (ev) => opts.onActivityEvent!(ev) : undefined,
      onMonitorEvent: opts.onMonitorEvent ? (event) => opts.onMonitorEvent!(event) : undefined,
      onBackpressure: opts.onBackpressure ? (payload) => opts.onBackpressure!(payload) : undefined,
      onV2Event: opts.onV2Event ? (env) => opts.onV2Event!(env) : undefined,
      onConnected: (info) => {
        connected.value = true;
        lastEventId.value = info.lastEventId;
        for (const ch of channels) {
          if (ch !== 'chat' && ch !== 'system') {
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
    });

    transport.value = t;
    t.connect();
  }

  function disconnect(): void {
    if (globalHubId) {
      releaseGlobalWsConsumer(globalHubId);
      globalHubId = null;
      connected.value = false;
      return;
    }
    transport.value?.disconnect();
    transport.value = null;
    connected.value = false;
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
    connect,
    disconnect,
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
