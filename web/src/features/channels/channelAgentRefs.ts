import type { ChannelRow } from './types';

function routingFromChannel(ch: ChannelRow): Record<string, unknown> | null {
  const raw = (ch.config_json ?? '').trim();
  if (!raw) return null;
  try {
    const cfg = JSON.parse(raw) as { routing?: Record<string, unknown> };
    return cfg.routing && typeof cfg.routing === 'object' ? cfg.routing : null;
  } catch {
    return null;
  }
}

/** Client-side filter: channels whose routing points at this agent (id or agent_key). */
export function channelsReferencingAgent(channels: ChannelRow[], agentId: string, agentKey: string): ChannelRow[] {
  const id = agentId.trim();
  const key = agentKey.trim();
  if (!id && !key) return [];
  return channels.filter((ch) => {
    const routing = routingFromChannel(ch);
    if (!routing) return false;
    const r = routing as Record<string, unknown>;
    const def = String(r.default_agent_id ?? '').trim();
    if (def && (def === id || (key && def === key))) return true;
    const rules = Array.isArray(r.rules) ? r.rules : [];
    for (const rule of rules) {
      if (!rule || typeof rule !== 'object') continue;
      const row = rule as Record<string, unknown>;
      if (String(row.team_id ?? '').trim()) continue;
      const ra = String(row.agent_id ?? '').trim();
      if (ra && (ra === id || (key && ra === key))) return true;
    }
    return false;
  });
}
