/**
 * Chat-specific envelope stream helpers + backward-compatible re-exports.
 *
 * - createChatStream / useChatStream: Chat session WS stream helpers
 * - createTeamStream / useTeamStream: Team session WS stream helpers
 * - useMonitorStream: Monitor/log stream helper
 * - Re-exports from realtime/ for backward compatibility
 *
 * New code that only needs the generic stream should import from
 * "realtime/useEnvelopeStream" directly.
 *
 * @deprecated The Activity-First architecture replaces envelope-based chat
 * streams with ActivityEvent consumption via useActivityTimeline. Chat code
 * should not extend this module. See ADR-02 §遗留项.
 */

export { createEnvelopeStream, useEnvelopeStream } from '../../realtime/useEnvelopeStream';

export type { UseEnvelopeStreamOptions, UseEnvelopeStreamReturn } from '../../realtime/useEnvelopeStream';

export type {
  GraphNodeState,
  GraphExecutionState,
  GraphStreamInterrupt,
  GraphStreamExecutionSummary,
} from '../../realtime/graphState';

import { createEnvelopeStream } from '../../realtime/useEnvelopeStream';
import type { UseEnvelopeStreamReturn } from '../../realtime/useEnvelopeStream';
import type { ActivityEvent } from '../../realtime/activityEvent';
import type { V2WsEnvelope } from './v2Types';

export type ChatStreamFactoryOpts = {
  lastEventId?: string;
  onConnected?: () => void;
  onDisconnected?: () => void;
  onServerShutdown?: (reason: string) => void;
  /**
   * Activity-First (AF): called when an activity_event WS message arrives.
   * Replaces the legacy activity_start/delta/done/child_start envelopes.
   */
  onActivityEvent?: (ev: ActivityEvent) => void;
  /** v2 chat events: called when a v2_event WS envelope arrives. */
  onV2Event?: (envelope: V2WsEnvelope) => void;
};

/** Chat session WS stream; use in `setup()` via {@link useChatStream} or imperatively via this factory. */
export function createChatStream(sessionId: string, streamOpts?: ChatStreamFactoryOpts): UseEnvelopeStreamReturn {
  return createEnvelopeStream({
    sessionId,
    channels: ['chat', 'system'],
    autoConnect: false,
    lastEventId: streamOpts?.lastEventId,
    onConnected: () => streamOpts?.onConnected?.(),
    onDisconnected: () => streamOpts?.onDisconnected?.(),
    onServerShutdown: streamOpts?.onServerShutdown,
    onActivityEvent: streamOpts?.onActivityEvent,
    onV2Event: streamOpts?.onV2Event,
  });
}

export function createTeamStream(
  sessionId: string,
  streamOpts?: {
    onConnected?: () => void;
    onDisconnected?: () => void;
    onServerShutdown?: (reason: string) => void;
    onActivityEvent?: (ev: ActivityEvent) => void;
    onV2Event?: (envelope: V2WsEnvelope) => void;
  },
): UseEnvelopeStreamReturn {
  return createEnvelopeStream({
    sessionId,
    channels: ['chat', 'team', 'system'],
    autoConnect: false,
    onConnected: () => streamOpts?.onConnected?.(),
    onDisconnected: () => streamOpts?.onDisconnected?.(),
    onServerShutdown: streamOpts?.onServerShutdown,
    onActivityEvent: streamOpts?.onActivityEvent,
    onV2Event: streamOpts?.onV2Event,
  });
}

export { useGraphStream } from '../graph/runtime/useGraphStream';
