import { createSpiritService } from '../../services';
import type { SpiritTeam, SpiritMember, SpiritTeamStatus, SpiritTeamMode } from './types';

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

export async function cancelSpiritTeam(teamId: string): Promise<void> {
  await spiritService.cancelTeamRun(teamId);
}

function mapSpiritTeam(raw: Record<string, unknown>): SpiritTeam {
  const dependsOnRaw = raw.dependsOn ?? raw.depends_on;
  return {
    id: String(raw.id ?? ''),
    teamName: String(raw.teamName ?? raw.team_name ?? ''),
    taskSummary: String(raw.taskSummary ?? raw.task_summary ?? ''),
    status: String(raw.status ?? '') as SpiritTeamStatus,
    mode: String(raw.mode ?? '') as SpiritTeamMode,
    memberAvatars: Array.isArray(raw.memberAvatars) ? (raw.memberAvatars as string[]) : [],
    completedSteps: Number(raw.completedSteps ?? 0),
    totalSteps: Number(raw.totalSteps ?? 0),
    durationMs: Number(raw.durationMs ?? raw.duration_ms ?? 0),
    spiritSessionId: String(raw.spiritSessionId ?? raw.spirit_session_id ?? ''),
    teamSessionId: String(raw.teamSessionId ?? raw.team_session_id ?? ''),
    members: Array.isArray(raw.members) ? (raw.members as Record<string, unknown>[]).map(mapSpiritMember) : [],
    sharedAgentIds: Array.isArray(raw.sharedAgentIds) ? (raw.sharedAgentIds as string[]) : [],
    dagNodeId: String(raw.dagNodeId ?? raw.dag_node_id ?? ''),
    dependsOn: Array.isArray(dependsOnRaw) ? (dependsOnRaw as unknown[]).map(String) : [],
    topologyReason: String(raw.topologyReason ?? raw.topology_reason ?? ''),
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
