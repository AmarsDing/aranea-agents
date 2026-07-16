/**
 * Monitor stream composable — a thin wrapper around createEnvelopeStream
 * that exposes a clean API for monitor-channel events (log, flow_log, mcp,
 * alert) via the `onMonitorEvent` callback.
 *
 * The backend now sends monitor events as `monitor_event?` on the WS
 * protocol's "monitor" channel (replacing the legacy envelope-based
 * dispatch for log/flow_log/mcp/alert). This factory is the preferred
 * entry point for monitor features; it delegates transport concerns to
 * `createEnvelopeStream` until the legacy envelope stream is removed.
 *
 * Features that need to consume team/chat runtime events (which still
 * flow as envelopes) should use `createEnvelopeStream` directly.
 */
import type { Ref } from 'vue';
import { createEnvelopeStream } from './useEnvelopeStream';
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
  const stream = createEnvelopeStream({
    sessionId: opts.sessionId,
    channels: opts.channels,
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
