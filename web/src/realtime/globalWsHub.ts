/**
 * Shared GlobalWsHub — the single source of truth for the global
 * WebSocket hub. Both chat and non-chat features import from here.
 *
 * Previously this module lived in features/chat/globalWsHub.ts; it has
 * been lifted to this shared location so that features don't need to
 * reach into the chat domain for hub-level infrastructure.
 */

import { GLOBAL_WS_SESSION_ID } from '../config/runtime';
import { createWsTransport, type WsTransport } from './ws-transport';
import type { ActivityEvent } from './activityEvent';
import type { MonitorEvent } from './monitorEvent';
import type { V2WsEnvelope } from '../features/chat/v2Types';

export type GlobalWsConsumer = {
  id: string;
  channels: Set<string>;
  logEnabled: boolean;
  /** Activity-First (AF): called when an activity_event message arrives. */
  onActivityEvent?: (ev: ActivityEvent) => void;
  /** Monitor channel: called when a monitor_event message arrives. */
  onMonitorEvent?: (event: MonitorEvent) => void;
  /** v2 chat events: called when a v2_event envelope arrives. */
  onV2Event?: (envelope: V2WsEnvelope) => void;
  onConnected?: () => void;
  onDisconnected?: () => void;
  onServerShutdown?: (reason: string) => void;
};

let transport: WsTransport | null = null;
const consumers = new Map<string, GlobalWsConsumer>();
let nextConsumerId = 0;

function syncHubSubscriptions(): void {
  const t = transport;
  if (!t?.connected) return;

  const extraChannels = new Set<string>();
  let logEnabled = false;
  for (const c of consumers.values()) {
    for (const ch of c.channels) {
      if (ch !== 'chat' && ch !== 'system') {
        extraChannels.add(ch);
      }
    }
    if (c.logEnabled) {
      logEnabled = true;
    }
  }
  for (const ch of extraChannels) {
    t.subscribe(ch);
  }
  t.enableLog(logEnabled);
}

function ensureHubTransport(): WsTransport {
  if (transport) {
    return transport;
  }

  transport = createWsTransport({
    sessionId: GLOBAL_WS_SESSION_ID,
    logEnabled: false,
    onActivityEvent: (ev) => {
      // Activity-First (AF): dispatch to all consumers that have opted in.
      for (const c of consumers.values()) {
        c.onActivityEvent?.(ev);
      }
    },
    onMonitorEvent: (event) => {
      // Monitor channel: dispatch to all consumers that have opted in.
      for (const c of consumers.values()) {
        c.onMonitorEvent?.(event);
      }
    },
    onV2Event: (envelope) => {
      // v2 chat events: dispatch to all consumers that have opted in.
      for (const c of consumers.values()) {
        c.onV2Event?.(envelope);
      }
    },
    onConnected: () => {
      syncHubSubscriptions();
      for (const c of consumers.values()) {
        c.onConnected?.();
      }
    },
    onDisconnected: () => {
      for (const c of consumers.values()) {
        c.onDisconnected?.();
      }
    },
    onServerShutdown: (reason) => {
      for (const c of consumers.values()) {
        c.onServerShutdown?.(reason);
      }
    },
  });

  return transport;
}

/** Share one `session_id=*` WebSocket across monitor/team consumers (excludes probe heartbeat). */
export function shouldUseGlobalWsHub(sessionId: string, lastEventId?: string): boolean {
  return sessionId === GLOBAL_WS_SESSION_ID && !String(lastEventId ?? '').trim();
}

export function acquireGlobalWsConsumer(
  opts: Omit<GlobalWsConsumer, 'id' | 'channels'> & { channels: Iterable<string> },
): string {
  const id = `gws-${++nextConsumerId}`;
  const channels = new Set(opts.channels);
  for (const ch of ['chat', 'system'] as const) {
    channels.add(ch);
  }
  consumers.set(id, {
    id,
    channels,
    logEnabled: opts.logEnabled,
    onActivityEvent: opts.onActivityEvent,
    onMonitorEvent: opts.onMonitorEvent,
    onV2Event: opts.onV2Event,
    onConnected: opts.onConnected,
    onDisconnected: opts.onDisconnected,
    onServerShutdown: opts.onServerShutdown,
  });

  const t = ensureHubTransport();
  if (!t.connected) {
    t.connect();
  } else {
    syncHubSubscriptions();
    opts.onConnected?.();
  }
  return id;
}

export function releaseGlobalWsConsumer(id: string): void {
  consumers.delete(id);
  if (consumers.size === 0) {
    transport?.disconnect();
    transport = null;
    return;
  }
  syncHubSubscriptions();
}

export function globalWsConsumerSubscribe(id: string, channel: string): void {
  const c = consumers.get(id);
  if (!c) return;
  c.channels.add(channel);
  syncHubSubscriptions();
}

export function globalWsConsumerUnsubscribe(id: string, channel: string): void {
  const c = consumers.get(id);
  if (!c || channel === 'chat' || channel === 'system') return;
  c.channels.delete(channel);
  syncHubSubscriptions();
}

export function globalWsConsumerEnableLog(id: string, enabled: boolean): void {
  const c = consumers.get(id);
  if (!c) return;
  c.logEnabled = enabled;
  syncHubSubscriptions();
}

export function globalWsHubConnected(): boolean {
  return transport?.connected ?? false;
}
