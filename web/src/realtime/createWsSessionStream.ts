/**
 * Shared WS session stream factory.
 *
 * Connects via globalWsHub (session_id=*) or a dedicated transport.
 * Downstream business events are `v2_event`; monitor events are `monitor_event`.
 * There is no activity_event branch — domain features must use typed hooks:
 *   - createV2EventStream / useV2EventStream
 *   - createMonitorStream
 *   - createChatStream / createTeamStream (chat send path)
 */
import { ref, shallowRef, type Ref } from 'vue';
import { createWsTransport, type MonitorBackpressurePayload, type WsTransport } from './ws-transport';
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
import {
  buildClientToolResultFrame,
  createTauriClientToolExecutor,
  DESKTOP_COMPANION_CAPABILITY,
  executeClientToolInvoke,
  isDesktopCompanion,
} from '../services/clientTools';

export type WsSessionStreamOptions = {
  sessionId: string;
  channels?: string[];
  lastEventId?: string;
  autoConnect?: boolean;
  logEnabled?: boolean;
  onConnected?: (info: { sessionId: string; lastEventId?: string }) => void;
  onDisconnected?: () => void;
  onServerShutdown?: (reason: string) => void;
  onMonitorEvent?: (event: MonitorEvent) => void;
  onBackpressure?: (payload: MonitorBackpressurePayload) => void;
  onV2Event?: (envelope: V2WsEnvelope) => void;
};

export type WsSessionStream = {
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

/** Factory for session streams; safe to call outside component `setup()`. */
export function createWsSessionStream(opts: WsSessionStreamOptions): WsSessionStream {
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
      onMonitorEvent: opts.onMonitorEvent ? (event) => opts.onMonitorEvent!(event) : undefined,
      onBackpressure: opts.onBackpressure ? (payload) => opts.onBackpressure!(payload) : undefined,
      onV2Event: opts.onV2Event ? (env) => opts.onV2Event!(env) : undefined,
      onClientToolInvoke: (msg) => {
        const executor = createTauriClientToolExecutor();
        if (!executor) return;
        void executeClientToolInvoke(executor, msg).then((outcome) => {
          t.send(buildClientToolResultFrame(msg, outcome));
        });
      },
      onConnected: (info) => {
        connected.value = true;
        lastEventId.value = info.lastEventId;
        for (const ch of channels) {
          if (ch !== 'chat' && ch !== 'system') {
            t.subscribe(ch);
          }
        }
        if (isDesktopCompanion()) {
          t.registerCapabilities([DESKTOP_COMPANION_CAPABILITY]);
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
