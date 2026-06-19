/**
 * WS Data Channel — B2 channel separation.
 *
 * Responsibility: the SINGLE source of truth for all message/state/streaming
 * data. Envelopes arriving via WebSocket are dispatched to subscribers
 * through this channel.
 *
 * Channel separation contract:
 *   - HTTP command channel (`command_channel.ts`): submit messages, get ACK
 *   - WS data channel (this module): receive all message/state/streaming data
 *
 * This module is a thin facade over the existing WS transport + dispatcher
 * infrastructure (`ws-transport.ts`, `dispatcher.ts`, `useEnvelopeStream.ts`).
 * It documents the "WS = data channel" responsibility and provides a
 * unified subscription interface for data consumers.
 *
 * Usage: consumers should subscribe to data via `useEnvelopeStream` or the
 * `EnvelopeDispatcher` directly. This module provides helper types and
 * documentation for the data channel contract.
 */
import type { Envelope, EnvelopeType } from './envelope';
import type { EnvelopeDispatcher } from './dispatcher';

/** Data channel subscription filter — mirrors DispatcherFilter but documents data-channel semantics. */
export interface DataChannelFilter {
  /** Channel names to listen on (e.g., 'chat', 'system'). */
  channels?: string[];
  /** Envelope types to listen for (e.g., 'text_delta', 'run_status'). */
  types?: EnvelopeType[];
  /** Session ID to filter on. */
  sessionId?: string;
  /** Team ID to filter on. */
  teamId?: string;
}

/** Data channel handler — receives envelopes from the WS data stream. */
export type DataChannelHandler = (env: Envelope) => void;

/** Unsubscribe function returned by `subscribeToDataChannel`. */
export type Unsubscribe = () => void;

/**
 * Subscribe to the WS data channel via an existing `EnvelopeDispatcher`.
 *
 * This is the canonical way to consume data from the WS data channel.
 * All message content, state deltas, tool calls, run status, and streaming
 * text arrive through this channel — HTTP responses must NOT be used as a
 * data source (see `command_channel.ts`).
 *
 * @param dispatcher The envelope dispatcher from `useEnvelopeStream` or `createEnvelopeStream`.
 * @param filter Optional filter (channels/types/sessionId/teamId).
 * @param handler Handler invoked for each matching envelope.
 * @returns Unsubscribe function.
 */
export function subscribeToDataChannel(
  dispatcher: EnvelopeDispatcher,
  filter: DataChannelFilter,
  handler: DataChannelHandler,
): Unsubscribe {
  return dispatcher.on(filter, handler);
}

/**
 * Subscribe to specific envelope types on the WS data channel.
 *
 * Convenience wrapper for `subscribeToDataChannel` with a type-only filter.
 */
export function subscribeToDataTypes(
  dispatcher: EnvelopeDispatcher,
  types: EnvelopeType | EnvelopeType[],
  handler: DataChannelHandler,
): Unsubscribe {
  return dispatcher.onType(types, handler);
}
