/**
 * Typed v2_event subscription hook.
 *
 * Graph / Teams / Knowledge / Orchestration (and Chat session streams) subscribe
 * here. Do not import useEnvelopeStream from production features.
 */
import { onUnmounted } from 'vue';
import type { V2WsEnvelope } from '../features/chat/v2Types';
import { createWsSessionStream, type WsSessionStream, type WsSessionStreamOptions } from './createWsSessionStream';

export type UseV2EventStreamOptions = Omit<WsSessionStreamOptions, 'onMonitorEvent' | 'onBackpressure'> & {
  onV2Event: (envelope: V2WsEnvelope) => void;
};

export type UseV2EventStreamReturn = WsSessionStream;

/** Factory for v2_event streams; safe to call outside component `setup()`. */
export function createV2EventStream(opts: UseV2EventStreamOptions): UseV2EventStreamReturn {
  return createWsSessionStream(opts);
}

export function useV2EventStream(opts: UseV2EventStreamOptions): UseV2EventStreamReturn {
  const stream = createV2EventStream({ ...opts, autoConnect: false });
  if (opts.autoConnect !== false) {
    stream.connect();
  }
  onUnmounted(() => {
    stream.disconnect();
  });
  return stream;
}
