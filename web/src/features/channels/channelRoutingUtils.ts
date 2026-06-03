import type { Agent } from '../agents/types';
import type { Team } from '../teams/types';

export type ChannelRoutingTargetType = 'agent' | 'team';

export const channelRoutingTargetToggleOptions = [
  { label: 'Agent', value: 'agent' as const },
  { label: 'Team', value: 'team' as const },
];

export function channelAgentSelectOptions(agents: Agent[]): Array<{ label: string; value: string }> {
  return agents.map((agent) => ({
    label: agent.display_name || agent.agent_key || agent.id,
    value: agent.id,
  }));
}

export function channelTeamSelectOptions(teams: Team[]): Array<{ label: string; value: string }> {
  return teams.map((team) => ({
    label: team.display_name || team.team_key || team.id,
    value: team.id,
  }));
}

/** 将 routing 里存的 agent id / agent_key 解析为下拉选项 value（agent.id）。 */
export function resolveChannelAgentSelectValue(raw: string, agents: Agent[]): string {
  const value = String(raw || '').trim();
  if (!value) return pickDefaultAgentId(agents);
  if (agents.some((agent) => agent.id === value)) return value;
  const byKey = agents.find((agent) => agent.agent_key === value);
  return byKey?.id || value;
}

export function pickDefaultAgentId(agents: Agent[]): string {
  return agents.find((agent) => agent.is_default)?.id || agents[0]?.id || '';
}

export function inferRoutingTargetType(routing?: Record<string, unknown>): ChannelRoutingTargetType {
  const teamId = String(routing?.default_team_id ?? '').trim();
  return teamId ? 'team' : 'agent';
}

export function isChannelRoutingValid(targetType: ChannelRoutingTargetType, agentId: string, teamId: string): boolean {
  return targetType === 'team' ? Boolean(teamId.trim()) : Boolean(agentId.trim());
}
