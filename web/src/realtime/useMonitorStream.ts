/**
 * Monitor stream factory — typed `monitor_event` subscription.
 *
 * The backend sends monitor events as `monitor_event` on `/v1/ws`
 * (log, flow_log, mcp, alert, computeruse.step). Features that consume
 * typed v2 chat/team/graph notices should use `createV2EventStream`.
 */
import type { Ref } from 'vue';
import { createWsSessionStream } from './createWsSessionStream';
import type { MonitorEvent } from './monitorEvent';
import type { MonitorBackpressurePayload } from './ws-transport';

export type UseMonitorStreamOptions = {
  sessionId: string;
  channels?: string[];
  autoConnect?: boolean;
  logEnabled?: boolean;
  onConnected?: () => void;
  onDisconnected?: () => void;
  onMonitorEvent?: (event: MonitorEvent) => void;
  onBackpressure?: (payload: MonitorBackpressurePayload) => void;
};

export type UseMonitorStreamReturn = {
  connected: Ref<boolean>;
  connect: () => void;
  disconnect: () => void;
  enableLog: (enabled: boolean) => void;
  subscribe: (channel: string) => void;
  unsubscribe: (channel: string) => void;
};

/** Factory for monitor-channel streams; safe to call outside component `setup()`. */
export function createMonitorStream(opts: UseMonitorStreamOptions): UseMonitorStreamReturn {
  const stream = createWsSessionStream({
    sessionId: opts.sessionId,
    channels: opts.channels ?? ['monitor', 'system'],
    autoConnect: opts.autoConnect,
    logEnabled: opts.logEnabled,
    onConnected: () => opts.onConnected?.(),
    onDisconnected: () => opts.onDisconnected?.(),
    onMonitorEvent: opts.onMonitorEvent,
    onBackpressure: opts.onBackpressure,
  });

  return {
    connected: stream.connected,
    connect: stream.connect,
    disconnect: stream.disconnect,
    enableLog: stream.enableLog,
    subscribe: stream.subscribe,
    unsubscribe: stream.unsubscribe,
  };
}
