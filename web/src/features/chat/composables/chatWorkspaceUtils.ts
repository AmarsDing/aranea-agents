import type { TeamRow } from "../../../components/chat/types";
import type { Agent } from "../../agents/types";

export const LS_AG_ORDER = "chat:order:agents";
export const LS_TM_ORDER = "chat:order:teams";

export function formatSessionTime(iso: string) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

export function getProviderModelValue(row: { key?: string; provider?: string; model?: string }) {
  return row.key || `${row.provider}:${row.model}`;
}

export function applyStoredOrder<T extends { id: string }>(items: T[], key: string): T[] {
  const byId = new Map(items.map((item) => [item.id, item] as const));
  const ordered: T[] = [];
  try {
    const ids = JSON.parse(localStorage.getItem(key) || "[]") as string[];
    for (const id of ids) {
      const item = byId.get(id);
      if (item) ordered.push(item);
    }
  } catch {
    /* ignore */
  }
  for (const item of items) {
    if (!ordered.some((candidate) => candidate.id === item.id)) ordered.push(item);
  }
  return ordered;
}

export function loadAgentOrder(agents: Agent[], defaultId: string | null): Agent[] {
  if (agents.length === 0) return [];
  const defaultResolved =
    defaultId && agents.some((agent) => agent.id === defaultId) ? defaultId : agents[0]!.id;
  const ordered = applyStoredOrder(agents, LS_AG_ORDER);
  const fixed = ordered.find((agent) => agent.id === defaultResolved) ?? ordered[0]!;
  return [fixed, ...ordered.filter((agent) => agent.id !== fixed.id)];
}

export function loadTeamOrder(teams: TeamRow[], defaultTeamId: string): TeamRow[] {
  const ordered = applyStoredOrder(teams, LS_TM_ORDER);
  const fixed = ordered.find((team) => team.id === defaultTeamId) ?? ordered[0];
  return fixed ? [fixed, ...ordered.filter((team) => team.id !== fixed.id)] : ordered;
}

export function isAgentWorking(agent: { status?: string }) {
  return /work|run|busy|ing/i.test(agent.status || "");
}
