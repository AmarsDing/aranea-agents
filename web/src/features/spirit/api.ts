import { createSpiritService } from '../../services';
import type { SpiritTeam, SpiritMember, SpiritTeamStatus, SpiritTeamMode } from './types';
import { isValidTeamStatus, isValidTeamMode } from './types';

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
// Resolves the active run_id from the team's run list before calling the
// cancel RPC, since the backend expects a team_run_id, not a team_id.
export async function cancelSpiritTeam(teamId: string): Promise<void> {
  const runId = await resolveActiveRunId(teamId);
  if (runId) {
    await spiritService.cancelTeamRun(runId);
  }
}

// resumeSpiritTeam resumes a paused team run.
// Same resolution strategy as cancelSpiritTeam.
export async function resumeSpiritTeam(teamId: string): Promise<void> {
  const runId = await resolveActiveRunId(teamId);
  if (runId) {
    await spiritService.resumeTeamRun(runId);
  }
}

// resolveActiveRunId fetches the latest run for a team and returns its ID.
// Returns null if no runs exist, allowing the caller to skip the RPC gracefully.
async function resolveActiveRunId(teamId: string): Promise<string | null> {
  try {
    const { data } = await spiritService.listTeamRuns(teamId);
    const runs = Array.isArray(data?.items) ? (data.items as Array<{ id: string }>) : [];
    return runs.length > 0 ? runs[0].id : null;
  } catch {
    return null;
  }
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
    status: isValidTeamStatus(String(raw.status ?? '')) ? String(raw.status) as SpiritTeamStatus : 'pending',
    mode: isValidTeamMode(String(raw.mode ?? '')) ? String(raw.mode) as SpiritTeamMode : 'coordinator',
    memberAvatars: Array.isArray(raw.memberAvatars) ? (raw.memberAvatars as string[]) : [],
    completedSteps: Number(raw.completedSteps ?? 0),
    totalSteps: Number(raw.totalSteps ?? 0),
    progressPct: Number(raw.progressPct ?? 0),
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
