import type { Envelope, EnvelopeType } from "./envelope";

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
    for (const sub of this.subs.values()) {
      if (!this.matchFilter(sub.filter, env)) continue;
      try {
        sub.handler(env);
      } catch {
        // handler errors are swallowed to avoid breaking other subscribers
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
  }
}

export function matchFilterKey(subscriberKey: string, eventKey: string): boolean {
  if (!subscriberKey || !eventKey) return true;
  const sk = subscriberKey + "/";
  const ek = eventKey + "/";
  return sk.startsWith(ek) || ek.startsWith(sk);
}
