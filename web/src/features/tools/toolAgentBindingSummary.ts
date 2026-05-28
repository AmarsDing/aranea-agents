import { listAgents } from "../agents/api";
import { getAgentEffectiveTools } from "./api";

export type ToolAgentBindingRow = {
  agent_id: string;
  agent_name: string;
  effective_state: string;
  reason: string;
  tools_enabled: boolean;
};

export type ToolAgentBindingSummary = {
  total_agents: number;
  allowed: number;
  denied: number;
  tools_disabled_agents: number;
  override_count: number;
  rows: ToolAgentBindingRow[];
};

const EMPTY_SUMMARY: ToolAgentBindingSummary = {
  total_agents: 0,
  allowed: 0,
  denied: 0,
  tools_disabled_agents: 0,
  override_count: 0,
  rows: []
};

/** Scan agents and resolve effective tool state for one tool key (client-side, ≤200 agents). */
export async function fetchToolAgentBindingSummary(
  toolKey: string,
  overrideCount = 0
): Promise<ToolAgentBindingSummary> {
  const key = toolKey.trim();
  if (!key) return { ...EMPTY_SUMMARY, override_count: overrideCount };

  const agents = await listAgents({ limit: 200 });
  const rows = await Promise.all(
    agents.map(async (agent) => {
      const agentId = agent.id || agent.agent_key;
      const agentName = agent.display_name || agent.agent_key || agentId;
      try {
        const view = await getAgentEffectiveTools(agentId);
        const item = view.items.find((t) => t.tool_key === key);
        const effective_state = item?.effective_state ?? "denied";
        return {
          agent_id: agentId,
          agent_name: agentName,
          effective_state,
          reason: item?.reason ?? "未在有效工具列表中",
          tools_enabled: view.tools_enabled
        };
      } catch {
        return {
          agent_id: agentId,
          agent_name: agentName,
          effective_state: "unknown",
          reason: "无法加载 Agent 工具策略",
          tools_enabled: false
        };
      }
    })
  );

  let allowed = 0;
  let denied = 0;
  let tools_disabled_agents = 0;
  for (const row of rows) {
    if (!row.tools_enabled) {
      tools_disabled_agents += 1;
      continue;
    }
    if (row.effective_state === "allowed") allowed += 1;
    else denied += 1;
  }

  return {
    total_agents: rows.length,
    allowed,
    denied,
    tools_disabled_agents,
    override_count: overrideCount,
    rows: rows.sort((a, b) => a.agent_name.localeCompare(b.agent_name))
  };
}

export function bindingSummaryLine(summary: ToolAgentBindingSummary): string {
  const parts = [`${summary.allowed} 个 Agent 可用`, `${summary.denied} 个不可用`];
  if (summary.tools_disabled_agents) {
    parts.push(`${summary.tools_disabled_agents} 个未启用工具调用`);
  }
  if (summary.override_count) {
    parts.push(`${summary.override_count} 条显式覆盖`);
  }
  return parts.join(" · ");
}
