// 记忆中心的平台视角：哪些 Agent 名下有记忆、不属于任何 Agent 的记忆按作用域计。纯函数，单测见 __tests__/memoryFootprint.spec.ts。

export type MemoryAgentFootprint = { agent_id: string; facts: number; episodes: number };
export type MemoryScopeCount = { scope_type: string; facts: number };
export type MemoryFootprint = { agents: MemoryAgentFootprint[]; unowned: MemoryScopeCount[] };

export const EMPTY_FOOTPRINT: MemoryFootprint = { agents: [], unowned: [] };

/** 默认选中的 Agent：优先记忆最多、且在可选列表里的那个；都没有记忆时退回列表第一个 */
export function pickDefaultAgent(agentIds: readonly string[], footprint: MemoryFootprint): string | null {
  const available = new Set(agentIds);
  const withMemory = footprint.agents.find((a) => available.has(a.agent_id));
  return withMemory?.agent_id ?? agentIds[0] ?? null;
}

/** 选择器里给有记忆的 Agent 标上条数，并排到最前 */
export function annotateAgentOptions(
  options: ReadonlyArray<{ label: string; value: string }>,
  footprint: MemoryFootprint,
  unit: (n: number) => string,
): Array<{ label: string; value: string }> {
  const counts = new Map(footprint.agents.map((a) => [a.agent_id, a.facts + a.episodes]));
  const rank = new Map(footprint.agents.map((a, i) => [a.agent_id, i]));
  return [...options]
    .map((o) => (counts.has(o.value) ? { ...o, label: `${o.label} · ${unit(counts.get(o.value) ?? 0)}` } : o))
    .sort((a, b) => (rank.get(a.value) ?? Infinity) - (rank.get(b.value) ?? Infinity));
}
