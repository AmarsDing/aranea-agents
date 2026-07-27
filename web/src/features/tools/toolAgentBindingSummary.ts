import { getToolAgentBindings } from './api';

export type ToolAgentBindingRow = {
  agent_id: string;
  agent_key: string;
  agent_name: string;
  agent_status: string;
  effective_state: string;
  reason: string;
  tools_enabled: boolean;
  profile: string;
  override_mode: string;
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
  rows: [],
};

/** 单次聚合请求获取某工具在所有可见 Agent 上的生效状态（后端 GetToolAgentBindings）。 */
export async function fetchToolAgentBindingSummary(toolId: string): Promise<ToolAgentBindingSummary> {
  const id = toolId.trim();
  if (!id) return { ...EMPTY_SUMMARY };

  const bindings = await getToolAgentBindings(id);
  const rows: ToolAgentBindingRow[] = bindings.map((b) => ({
    agent_id: b.agent_id,
    agent_key: b.agent_key,
    agent_name: b.agent_name || b.agent_key || b.agent_id,
    agent_status: b.agent_status,
    effective_state: b.effective_state,
    reason: b.reason,
    tools_enabled: b.tools_enabled,
    profile: b.profile,
    override_mode: b.override_mode,
  }));

  let allowed = 0;
  let denied = 0;
  let toolsDisabled = 0;
  let overrideCount = 0;
  for (const row of rows) {
    if (row.override_mode) overrideCount += 1;
    if (!row.tools_enabled) {
      toolsDisabled += 1;
      continue;
    }
    if (row.effective_state === 'allowed') allowed += 1;
    else denied += 1;
  }

  return {
    total_agents: rows.length,
    allowed,
    denied,
    tools_disabled_agents: toolsDisabled,
    override_count: overrideCount,
    rows: rows.sort((a, b) => a.agent_name.localeCompare(b.agent_name)),
  };
}

type TranslateFn = (key: string, named?: Record<string, unknown>) => string;

export function bindingSummaryLine(summary: ToolAgentBindingSummary, t: TranslateFn): string {
  const parts = [
    t('toolsPage.agentBinding.summaryAllowed', { count: summary.allowed }),
    t('toolsPage.agentBinding.summaryDenied', { count: summary.denied }),
  ];
  if (summary.tools_disabled_agents) {
    parts.push(t('toolsPage.agentBinding.summaryToolsDisabled', { count: summary.tools_disabled_agents }));
  }
  if (summary.override_count) {
    parts.push(t('toolsPage.agentBinding.summaryOverrides', { count: summary.override_count }));
  }
  return parts.join(' · ');
}
