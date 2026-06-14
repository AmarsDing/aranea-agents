/**
 * Shared EnvelopeDispatcher — the single source of truth for the
 * envelope dispatch infrastructure. Both chat and non-chat features
 * (monitor, teams, orchestration, graph) import from here.
 *
 * Previously this class lived in features/chat/dispatcher.ts; it has
 * been lifted to this shared location so that features don't need to
 * reach into the chat domain for protocol-level infrastructure.
 */

import type { Envelope, EnvelopeType } from './envelope';

export type EnvelopeHandler = (env: Envelope) => void;

export type DispatcherFilter = {
  channels?: string[];
  types?: EnvelopeType[];
  sessionId?: string;
  teamId?: string;
  filterKey?: string;
};

type subscription = {
  id: number;
  filter: DispatcherFilter;
  handler: EnvelopeHandler;
};

let nextSubId = 1;

export class EnvelopeDispatcher {
  private subs: Map<number, subscription> = new Map();
  /**
   * Structural dedup: prevents the same envelope from being dispatched
   * twice within a single stream lifecycle. This is a safety net that
   * makes duplicate dispatch *impossible* rather than relying on upstream
   * correctness (WAL replay, reconnect overlap, network retransmission).
   *
   * Lifecycle: cleared on clear() / disconnect, so replay after reconnect
   * is not affected. Empty envelope IDs are skipped (defensive).
   */
  private dispatchedIds: Set<string> = new Set();
  private static readonly DEDUP_MAX = 256;

  on(filter: DispatcherFilter, handler: EnvelopeHandler): () => void {
    const id = nextSubId++;
    this.subs.set(id, { id, filter, handler });
    return () => {
      this.subs.delete(id);
    };
  }

  onType(type: EnvelopeType | EnvelopeType[], handler: EnvelopeHandler): () => void {
    const types = Array.isArray(type) ? type : [type];
    return this.on({ types }, handler);
  }

  onChannel(channel: string | string[], handler: EnvelopeHandler): () => void {
    const channels = Array.isArray(channel) ? channel : [channel];
    return this.on({ channels }, handler);
  }

  dispatch(env: Envelope): void {
    // Structural dedup: skip envelopes already dispatched in this stream.
    // This prevents WAL replay, reconnect overlap, or any upstream
    // duplication from causing duplicate handler invocations.
    if (env.id) {
      if (this.dispatchedIds.has(env.id)) return;
      this.dispatchedIds.add(env.id);
      if (this.dispatchedIds.size > EnvelopeDispatcher.DEDUP_MAX) {
        // Evict oldest entries to prevent unbounded growth
        const iter = this.dispatchedIds.values();
        for (let i = 0; i < 64; i++) iter.next();
        for (let i = 0; i < 64; i++) {
          const r = iter.next();
          if (r.done) break;
          this.dispatchedIds.delete(r.value);
        }
      }
    }
    for (const sub of this.subs.values()) {
      if (!this.matchFilter(sub.filter, env)) continue;
      try {
        sub.handler(env);
      } catch (err) {
        if (import.meta.env.DEV) {
          console.warn('[EnvelopeDispatcher] handler error:', err, env);
        }
      }
    }
  }

  private matchFilter(filter: DispatcherFilter, env: Envelope): boolean {
    if (filter.channels && filter.channels.length > 0) {
      if (!env.channel || !filter.channels.includes(env.channel)) return false;
    }
    if (filter.types && filter.types.length > 0) {
      if (!filter.types.includes(env.type)) return false;
    }
    if (filter.sessionId && env.session_id !== filter.sessionId) return false;
    if (filter.teamId && env.team_id !== filter.teamId) return false;
    if (filter.filterKey && env.filter_key && !matchFilterKey(filter.filterKey, env.filter_key)) return false;
    return true;
  }

  clear(): void {
    this.subs.clear();
    this.dispatchedIds.clear();
  }
}

export function matchFilterKey(subscriberKey: string, eventKey: string): boolean {
  if (!subscriberKey || !eventKey) return true;
  const sk = subscriberKey + '/';
  const ek = eventKey + '/';
  return sk.startsWith(ek) || ek.startsWith(sk);
}
