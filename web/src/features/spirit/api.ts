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

// cancelSpiritTeam cancels the active run for a team.
// NOTE: The backend CancelTeamRun RPC expects a team_run_id, but SpiritTeam
// only exposes team_id. The route is correct per proto; the caller must resolve
// the active run ID when available. For now we pass team_id as a best-effort
// until SpiritTeamView includes an active_run_id field.
export async function cancelSpiritTeam(teamId: string): Promise<void> {
  await spiritService.cancelTeamRun(teamId);
}

// resumeSpiritTeam resumes a paused team run.
// NOTE: Same caveat as cancelSpiritTeam — backend expects run_id, we pass team_id.
export async function resumeSpiritTeam(teamId: string): Promise<void> {
  await spiritService.resumeTeamRun(teamId);
}

export async function archiveSpiritTeam(teamId: string): Promise<void> {
  await spiritService.archiveTeam(teamId);
}

export async function retrySpiritTeam(teamId: string): Promise<void> {
  await spiritService.retryTeam(teamId);
}

function mapSpiritTeam(raw: Record<string, unknown>): SpiritTeam {
  return {
    id: String(raw.id ?? ''),
    teamName: String(raw.teamName ?? ''),
    taskSummary: String(raw.taskSummary ?? ''),
    status: String(raw.status ?? '') as SpiritTeamStatus,
    mode: String(raw.mode ?? '') as SpiritTeamMode,
    memberAvatars: Array.isArray(raw.memberAvatars) ? (raw.memberAvatars as string[]) : [],
    completedSteps: Number(raw.completedSteps ?? 0),
    totalSteps: Number(raw.totalSteps ?? 0),
    durationMs: Number(raw.durationMs ?? 0),
    spiritSessionId: String(raw.spiritSessionId ?? ''),
    teamSessionId: String(raw.teamSessionId ?? ''),
    members: Array.isArray(raw.members) ? (raw.members as Record<string, unknown>[]).map(mapSpiritMember) : [],
    sharedAgentIds: Array.isArray(raw.sharedAgentIds) ? (raw.sharedAgentIds as string[]) : [],
    dagNodeId: String(raw.dagNodeId ?? ''),
    graphExecutionId: String(raw.graphExecutionId ?? '') || undefined,
    dependsOn: Array.isArray(raw.dependsOn) ? (raw.dependsOn as unknown[]).map(String) : [],
    topologyReason: String(raw.topologyReason ?? ''),
    interruptReason: String(raw.interruptReason ?? '') || undefined,
    tokenIn: Number(raw.tokenIn ?? 0) || undefined,
    tokenOut: Number(raw.tokenOut ?? 0) || undefined,
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
