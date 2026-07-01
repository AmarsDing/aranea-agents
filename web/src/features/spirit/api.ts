import { createSpiritService } from '../../services';
import type { SpiritTeam, SpiritMember, SpiritTeamStatus, SpiritTeamMode } from './types';
import { isValidTeamStatus, isValidTeamMode } from './types';

const spiritService = createSpiritService();

export async function listSpiritTeams(spiritSessionId: string): Promise<SpiritTeam[]> {
  const { data } = await spiritService.listTeams(spiritSessionId);
  // Backend returns `teams` field (proto: repeated SpiritTeamView teams = 1)
  const items = Array.isArray(data?.teams) ? (data.teams as Record<string, unknown>[]) : [];
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

// pauseSpiritTeam transitions a running team run to paused state (B.5.3).
// Resolves the active run_id from the team's run list before calling pause.
// MVP: cancels the in-flight member step + marks the run as paused.
export async function pauseSpiritTeam(teamId: string): Promise<void> {
  const runId = await resolveActiveRunId(teamId);
  if (runId) {
    await spiritService.pauseTeamRun(runId);
  }
}

// unpauseSpiritTeam transitions a paused team run back to running (B.5.3).
// Distinct from resumeSpiritTeam (HITL graph recovery via /resume).
// MVP: flips the status marker; user must inject a message to resume execution.
export async function unpauseSpiritTeam(teamId: string): Promise<void> {
  const runId = await resolveActiveRunId(teamId);
  if (runId) {
    await spiritService.unpauseTeamRun(runId);
  }
}

// injectSpiritTeam injects a user message into the team's active/paused run
// pending queue (B.5.3). The message is processed at the next step boundary.
// Takes teamId directly (no run_id resolution needed).
export async function injectSpiritTeam(teamId: string, message: string): Promise<void> {
  await spiritService.injectTeamMessage(teamId, message);
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

// cancelAgentSession cancels the active run for a sub-agent session.
// Reuses the existing StopGeneration RPC (POST /v1/chat/stop) with the
// childSessionId as the target (B.5.2 childSessionId scheme).
export async function cancelAgentSession(sessionId: string): Promise<void> {
  await spiritService.stopGeneration(sessionId);
}

// retryAgentSession re-triggers the last failed/interrupted turn for a sub-agent
// session by re-enqueuing the latest user message (RetrySession RPC).
export async function retryAgentSession(sessionId: string): Promise<void> {
  await spiritService.retrySession(sessionId);
}

// pauseAgentSession transitions a running sub-agent session to paused state
// (B.5.3). Used by AgentCard pause button. MVP cancels the in-flight turn.
export async function pauseAgentSession(sessionId: string): Promise<void> {
  await spiritService.pauseSession(sessionId);
}

// resumeAgentSession transitions a paused sub-agent session back to running
// (B.5.3). Used by AgentCard resume button. MVP flips the status marker.
export async function resumeAgentSession(sessionId: string): Promise<void> {
  await spiritService.resumeSession(sessionId);
}

function mapSpiritTeam(raw: Record<string, unknown>): SpiritTeam {
  return {
    id: String(raw.id ?? ''),
    teamName: String(raw.team_name ?? raw.teamName ?? ''),
    taskSummary: String(raw.task_summary ?? raw.taskSummary ?? ''),
    status: isValidTeamStatus(String(raw.status ?? '')) ? (String(raw.status) as SpiritTeamStatus) : 'pending',
    mode: isValidTeamMode(String(raw.mode ?? '')) ? (String(raw.mode) as SpiritTeamMode) : 'coordinator',
    memberAvatars: Array.isArray(raw.member_avatars ?? raw.memberAvatars)
      ? ((raw.member_avatars ?? raw.memberAvatars) as string[])
      : [],
    completedSteps: Number(raw.completed_steps ?? raw.completedSteps ?? 0),
    totalSteps: Number(raw.total_steps ?? raw.totalSteps ?? 0),
    progressPct: Number(raw.progress_pct ?? raw.progressPct ?? 0),
    durationMs: Number(raw.duration_ms ?? raw.durationMs ?? 0),
    spiritSessionId: String(raw.spirit_session_id ?? raw.spiritSessionId ?? ''),
    teamSessionId: String(raw.team_session_id ?? raw.teamSessionId ?? ''),
    members: Array.isArray(raw.members) ? (raw.members as Record<string, unknown>[]).map(mapSpiritMember) : [],
    sharedAgentIds: Array.isArray(raw.shared_agent_ids ?? raw.sharedAgentIds)
      ? ((raw.shared_agent_ids ?? raw.sharedAgentIds) as unknown[]).map(String)
      : [],
    createdAt: Number(raw.created_at ?? raw.createdAt ?? 0) || Date.now(),
    dagNodeId: String(raw.dag_node_id ?? raw.dagNodeId ?? ''),
    graphExecutionId: String(raw.graph_execution_id ?? raw.graphExecutionId ?? '') || undefined,
    dependsOn: Array.isArray(raw.depends_on ?? raw.dependsOn)
      ? ((raw.depends_on ?? raw.dependsOn) as unknown[]).map(String)
      : [],
    topologyReason: String(raw.topology_reason ?? raw.topologyReason ?? ''),
    interruptReason: String(raw.interrupt_reason ?? raw.interruptReason ?? '') || undefined,
    tokenIn: Number(raw.token_in ?? raw.tokenIn ?? 0) || undefined,
    tokenOut: Number(raw.token_out ?? raw.tokenOut ?? 0) || undefined,
  };
}

function mapSpiritMember(raw: Record<string, unknown>): SpiritMember {
  return {
    agentId: String(raw.agent_id ?? raw.agentId ?? ''),
    agentKey: String(raw.agent_key ?? raw.agentKey ?? ''),
    displayName: String(raw.display_name ?? raw.displayName ?? ''),
    role: String(raw.role ?? ''),
    status: String(raw.status ?? ''),
    avatarUrl: String(raw.avatar_url ?? raw.avatarUrl ?? ''),
  };
}
