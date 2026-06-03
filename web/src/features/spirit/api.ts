import { createSpiritService } from '../../services';
import type { SpiritTeam, SpiritMember } from './types';

const spiritService = createSpiritService();

export async function listSpiritTeams(spiritSessionId: string): Promise<SpiritTeam[]> {
  const { data } = await spiritService.listTeams(spiritSessionId);
  const items = Array.isArray(data?.items) ? (data.items as Record<string, unknown>[]) : [];
  return items.map(mapSpiritTeam);
}

export async function getSpiritTeamDetail(teamId: string): Promise<SpiritTeam> {
  const { data } = await spiritService.getTeamDetail(teamId);
  return mapSpiritTeam(data as Record<string, unknown>);
}

function mapSpiritTeam(raw: Record<string, unknown>): SpiritTeam {
  return {
    id: String(raw.id ?? ''),
    teamName: String(raw.teamName ?? ''),
    taskSummary: String(raw.taskSummary ?? ''),
    status: String(raw.status ?? ''),
    mode: String(raw.mode ?? ''),
    memberAvatars: Array.isArray(raw.memberAvatars) ? (raw.memberAvatars as string[]) : [],
    completedSteps: Number(raw.completedSteps ?? 0),
    totalSteps: Number(raw.totalSteps ?? 0),
    spiritSessionId: String(raw.spiritSessionId ?? ''),
    teamSessionId: String(raw.teamSessionId ?? ''),
    members: Array.isArray(raw.members) ? (raw.members as Record<string, unknown>[]).map(mapSpiritMember) : [],
    sharedAgentIds: Array.isArray(raw.sharedAgentIds) ? (raw.sharedAgentIds as string[]) : [],
  };
}

function mapSpiritMember(raw: Record<string, unknown>): SpiritMember {
  return {
    agentId: String(raw.agentId ?? ''),
    agentKey: String(raw.agentKey ?? ''),
    displayName: String(raw.displayName ?? ''),
    role: String(raw.role ?? ''),
    status: String(raw.status ?? ''),
    avatarUrl: String(raw.avatarUrl ?? ''),
  };
}
