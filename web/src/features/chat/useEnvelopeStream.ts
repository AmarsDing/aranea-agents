/**
 * Chat-specific WS stream helpers.
 *
 * - createChatStream / createTeamStream: session WS (typed v2_event)
 * Re-exports graph stream for historical import paths.
 *
 * Production features outside chat must not import realtime/useEnvelopeStream.
 */

export type {
  GraphNodeState,
  GraphExecutionState,
  GraphStreamInterrupt,
  GraphStreamExecutionSummary,
} from '../../realtime/graphState';

import { createV2EventStream } from '../../realtime/useV2EventStream';
import type { WsSessionStream } from '../../realtime/createWsSessionStream';
import type { WsUpstream } from '../../realtime/ws-transport';
import type { V2WsEnvelope } from './v2Types';

export type ChatStreamFactoryOpts = {
  lastEventId?: string;
  onConnected?: () => void;
  onDisconnected?: () => void;
  onServerShutdown?: (reason: string) => void;
  /** v2 chat events: called when a v2_event WS envelope arrives. */
  onV2Event?: (envelope: V2WsEnvelope) => void;
  /** 业务消息发送队列满被丢弃时触发（F1：此前未接线，user_message 静默丢失）。 */
  onDrop?: (upstream: WsUpstream) => void;
};

/** Chat session WS stream; use imperatively via {@link createChatStream}. */
export function createChatStream(sessionId: string, streamOpts?: ChatStreamFactoryOpts): WsSessionStream {
  return createV2EventStream({
    sessionId,
    channels: ['chat', 'system'],
    autoConnect: false,
    lastEventId: streamOpts?.lastEventId,
    onConnected: () => streamOpts?.onConnected?.(),
    onDisconnected: () => streamOpts?.onDisconnected?.(),
    onServerShutdown: streamOpts?.onServerShutdown,
    onV2Event: streamOpts?.onV2Event ?? (() => undefined),
    onDrop: streamOpts?.onDrop,
  });
}

export function createTeamStream(
  sessionId: string,
  streamOpts?: {
    onConnected?: () => void;
    onDisconnected?: () => void;
    onServerShutdown?: (reason: string) => void;
    onV2Event?: (envelope: V2WsEnvelope) => void;
    onDrop?: (upstream: WsUpstream) => void;
  },
): WsSessionStream {
  return createV2EventStream({
    sessionId,
    channels: ['chat', 'team', 'system'],
    autoConnect: false,
    onConnected: () => streamOpts?.onConnected?.(),
    onDisconnected: () => streamOpts?.onDisconnected?.(),
    onServerShutdown: streamOpts?.onServerShutdown,
    onV2Event: streamOpts?.onV2Event ?? (() => undefined),
    onDrop: streamOpts?.onDrop,
  });
}

export { useGraphStream } from '../graph/runtime/useGraphStream';
