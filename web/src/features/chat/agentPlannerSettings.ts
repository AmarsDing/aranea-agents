/** Ensure Chat has agent runtime settings (planner_kind) when list API omits them. */

import { getAgent } from '../agents/api';
import type { Agent } from '../agents/types';

export function agentNeedsSettingsHydration(agent: Agent | null | undefined): boolean {
  if (!agent?.id) return false;
  if (agent.settings == null) return true;
  return !(agent.settings.planner_kind ?? '').trim();
}

/** Fetch full agent when list row lacks settings; merges planner_kind for Chat presentation. */
export async function hydrateAgentSettings(agent: Agent): Promise<Agent> {
  if (!agent.id || !agentNeedsSettingsHydration(agent)) {
    return agent;
  }
  try {
    return await getAgent(agent.id);
  } catch (err) {
    if (import.meta.env.DEV) {
      console.warn('[hydrateAgentSettings] getAgent failed:', agent.id, err);
    }
    return agent;
  }
}
