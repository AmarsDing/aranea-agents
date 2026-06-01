import type { TeamRow, SessionView } from "../../../components/chat/types";
import type { Agent } from "../../agents/types";
import type { Session } from "../../session/api";

export const LS_AG_ORDER = "chat:order:agents";
export const LS_TM_ORDER = "chat:order:teams";
export const LS_AG_GROUP_ORDER_PREFIX = "chat:order:agents:";

export function formatSessionTime(iso: string) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

export function sessionToView(session: Session, t: (key: string) => string): SessionView {
  return {
    id: session.id,
    title: session.title || t("chat.untitledSession"),
    context_used_ratio: session.context_used_ratio,
    context_status: session.context_status,
    context_used_tokens: session.context_used_tokens,
    last_context_window_tokens: session.last_context_window_tokens,
    input_tokens: session.input_tokens,
    output_tokens: session.output_tokens,
    total_tokens: session.total_tokens,
    total_cost_micro_usd: session.total_cost_micro_usd,
    at: formatSessionTime(session.last_message_at || session.updated_at || session.created_at),
    timeline_at: session.last_message_at || session.updated_at || session.created_at,
    agent_id: session.agent_id,
    status: session.status,
    pinned_at: session.pinned_at,
    metadata_json: session.metadata_json,
  };
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

export function loadGroupOrder<T extends { id: string }>(items: T[], groupKey: string, pinnedId?: string | null): T[] {
  if (items.length === 0) return [];
  const lsKey = `${LS_AG_GROUP_ORDER_PREFIX}${groupKey}`;
  const ordered = applyStoredOrder(items, lsKey);
  if (pinnedId) {
    const pinned = ordered.find((item) => item.id === pinnedId);
    if (pinned) {
      return [pinned, ...ordered.filter((item) => item.id !== pinnedId)];
    }
  }
  return ordered;
}

export function saveGroupOrder(groupKey: string, ids: string[]) {
  const lsKey = `${LS_AG_GROUP_ORDER_PREFIX}${groupKey}`;
  try {
    localStorage.setItem(lsKey, JSON.stringify(ids));
  } catch {
    /* ignore */
  }
}
