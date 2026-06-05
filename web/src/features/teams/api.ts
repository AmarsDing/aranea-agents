import { createTeamService } from '../../services';
import type { Team, TeamRun, TeamRunEvent, TeamRunStep, TeamRunSummary, TaskDeadLetterRow } from './types';
import type {
  Team as WireTeam,
  TeamRun as WireTeamRun,
  TeamRunStep as WireTeamRunStep,
  TeamRunSummary as WireTeamRunSummary,
  TeamRunMemberSummary as WireTeamRunMemberSummary,
} from '../../services/kratos/team/v1/index';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import { createEnvelopeStream } from '../../realtime/useEnvelopeStream';
import { TEAM_RUNTIME_ENVELOPE_TYPES, teamRunEventFromEnvelope } from './teamRunEventFromEnvelope';
import type { Envelope } from '../../realtime/envelope';

export type {
  Team,
  TeamDefinition,
  TeamDefinitionGraphEdge,
  TeamDefinitionGraphNode,
  TeamDefinitionMember,
  TeamRun,
  TeamRunEvent,
  TeamRunStep,
  TeamRunSummary,
  TaskDeadLetterRow,
} from './types';

function wireTeam(t: WireTeam | null | undefined): Team {
  return {
    id: t?.id ?? '',
    team_key: t?.teamKey ?? '',
    display_name: t?.displayName ?? '',
    status: t?.status ?? '',
    is_default: t?.isDefault ?? false,
    definition_json: t?.definitionJson ?? '',
    app_name: t?.adkAppName ?? '',
    linked_graph_id: t?.linkedGraphId ?? '',
    has_active_run: t?.hasActiveRun ?? false,
    taxonomy_industry_id: t?.categoryIndustryId ?? '',
    readonly: t?.readonly ?? false,
    source: t?.source ?? '',
    kind: t?.kind ?? '',
    created_at: t?.createdAt ?? '',
    updated_at: t?.updatedAt ?? '',
    deleted_at: t?.deletedAt ?? '',
  };
}

function wireRun(r: WireTeamRun | null | undefined): TeamRun {
  return {
    id: r?.id ?? '',
    team_id: r?.teamId ?? '',
    session_id: r?.sessionId ?? '',
    message_id: r?.messageId ?? '',
    mode: r?.mode ?? '',
    status: r?.status ?? '',
    input_preview: r?.inputPreview ?? '',
    output_preview: r?.outputPreview ?? '',
    token_in: r?.tokenIn ?? 0,
    token_out: r?.tokenOut ?? 0,
    cost_micro_usd: Number(r?.costMicroUsd ?? 0),
    duration_ms: r?.durationMs ?? 0,
    error_message: r?.errorMessage ?? '',
    topology_json: r?.topologyJson ?? '',
    graph_execution_id: r?.graphExecutionId ?? '',
    started_at: r?.startedAt ?? '',
    finished_at: r?.finishedAt ?? '',
    created_at: r?.createdAt ?? '',
    updated_at: r?.updatedAt ?? '',
  };
}

function wireStep(s: WireTeamRunStep | null | undefined): TeamRunStep {
  return {
    id: s?.id ?? '',
    run_id: s?.runId ?? '',
    team_id: s?.teamId ?? '',
    agent_id: s?.agentId ?? '',
    agent_key: s?.agentKey ?? '',
    agent_name: s?.agentName ?? '',
    role: s?.role ?? '',
    sort_order: s?.sortOrder ?? 0,
    status: s?.status ?? '',
    input_preview: s?.inputPreview ?? '',
    output_preview: s?.outputPreview ?? '',
    token_in: s?.tokenIn ?? 0,
    token_out: s?.tokenOut ?? 0,
    cost_micro_usd: Number(s?.costMicroUsd ?? 0),
    duration_ms: s?.durationMs ?? 0,
    error_message: s?.errorMessage ?? '',
    started_at: s?.startedAt ?? '',
    finished_at: s?.finishedAt ?? '',
    created_at: s?.createdAt ?? '',
    tool_call_count: s?.toolCallCount ?? 0,
  };
}

function wireMemberSummary(m: WireTeamRunMemberSummary | null | undefined) {
  return {
    agent_id: m?.agentId ?? '',
    agent_key: m?.agentKey ?? '',
    agent_name: m?.agentName ?? '',
    role: m?.role ?? '',
    sort_order: m?.sortOrder ?? 0,
    status: m?.status ?? '',
    token_in: m?.tokenIn ?? 0,
    token_out: m?.tokenOut ?? 0,
    duration_ms: m?.durationMs ?? 0,
    cost_micro_usd: Number(m?.costMicroUsd ?? 0),
    output_preview: m?.outputPreview ?? '',
    tool_call_count: m?.toolCallCount ?? 0,
  };
}

function wireRunSummary(s: WireTeamRunSummary | null | undefined): TeamRunSummary {
  return {
    run_id: s?.runId ?? '',
    team_id: s?.teamId ?? '',
    session_id: s?.sessionId ?? '',
    mode: s?.mode ?? '',
    status: s?.status ?? '',
    duration_ms: s?.durationMs ?? 0,
    token_in: s?.tokenIn ?? 0,
    token_out: s?.tokenOut ?? 0,
    cost_micro_usd: Number(s?.costMicroUsd ?? 0),
    member_count: s?.memberCount ?? 0,
    tool_call_count: s?.toolCallCount ?? 0,
    output_preview: s?.outputPreview ?? '',
    error_message: s?.errorMessage ?? '',
    members: (s?.members ?? []).map(wireMemberSummary),
  };
}

function patchToWire(payload: Partial<Team>): WireTeam {
  const t = {} as WireTeam;
  if (payload.team_key !== undefined) t.teamKey = payload.team_key;
  if (payload.display_name !== undefined) t.displayName = payload.display_name;
  if (payload.status !== undefined) t.status = payload.status;
  if (payload.is_default !== undefined) t.isDefault = payload.is_default;
  if (payload.definition_json !== undefined) t.definitionJson = payload.definition_json;
  if (payload.app_name !== undefined) t.adkAppName = payload.app_name;
  if (payload.linked_graph_id !== undefined) t.linkedGraphId = payload.linked_graph_id;
  if (payload.taxonomy_industry_id !== undefined) t.categoryIndustryId = payload.taxonomy_industry_id;
  if (payload.created_at !== undefined) t.createdAt = payload.created_at;
  if (payload.updated_at !== undefined) t.updatedAt = payload.updated_at;
  if (payload.deleted_at !== undefined) t.deletedAt = payload.deleted_at;
  return t;
}

export async function listTeams(): Promise<Team[]> {
  const svc = createTeamService();
  const res = await svc.ListTeams({});
  const items = res.items ?? [];
  return items.map(wireTeam);
}

export async function createTeam(payload: Partial<Team>): Promise<Team> {
  const svc = createTeamService();
  const data = await svc.CreateTeam({
    teamKey: payload.team_key,
    displayName: payload.display_name,
    status: payload.status,
    definitionJson: payload.definition_json,
    adkAppName: payload.app_name,
    categoryIndustryId: payload.taxonomy_industry_id,
  });
  return wireTeam(data);
}

export async function updateTeam(id: string, payload: Partial<Team>): Promise<Team> {
  const svc = createTeamService();
  const data = await svc.UpdateTeam({ id, team: patchToWire(payload) });
  return wireTeam(data);
}

export async function duplicateTeam(id: string): Promise<Team> {
  const svc = createTeamService();
  const data = await svc.DuplicateTeam({ id });
  return wireTeam(data);
}

export async function deleteTeam(id: string): Promise<void> {
  const svc = createTeamService();
  await svc.DeleteTeam({ id });
}

export async function listTeamRuns(teamID?: string, limit = 50): Promise<TeamRun[]> {
  const svc = createTeamService();
  const res = await svc.ListTeamRuns({ teamId: teamID, limit });
  const items = res.items ?? [];
  return items.map(wireRun);
}

const ACTIVE_RUN_STATUSES = new Set(['running', 'pending']);

export async function findActiveTeamRun(teamID: string): Promise<TeamRun | null> {
  const runs = await listTeamRuns(teamID, 50);
  return runs.find((run) => ACTIVE_RUN_STATUSES.has(run.status)) ?? null;
}

export async function listTeamRunSteps(runID: string): Promise<TeamRunStep[]> {
  const svc = createTeamService();
  const res = await svc.ListTeamRunSteps({ runId: runID });
  const items = res.items ?? [];
  return items.map(wireStep);
}

export async function runTeamTest(teamID: string, content?: string): Promise<{ run: TeamRun; reply: string }> {
  const svc = createTeamService();
  const res = await svc.RunTeamTest({ id: teamID, content: content?.trim() || undefined });
  return {
    run: wireRun(res.run),
    reply: res.reply ?? '',
  };
}

export async function getTeamRunSummary(runID: string): Promise<TeamRunSummary> {
  const svc = createTeamService();
  const res = await svc.GetTeamRunSummary({ id: runID });
  return wireRunSummary(res.summary);
}

export async function resumeTeamRunExecution(
  runID: string,
  resumeValue?: Record<string, unknown>,
): Promise<{ runId: string; graphExecutionId: string; status: string }> {
  const svc = createTeamService();
  const res = await svc.ResumeTeamRunExecution({ runId: runID, resumeValue });
  return {
    runId: res.runId ?? runID,
    graphExecutionId: res.graphExecutionId ?? '',
    status: res.status ?? '',
  };
}

/**
 * Team run events over `WS /v1/ws`.
 * Pass a real chat `sessionId` for session-scoped runs, or `GLOBAL_WS_SESSION_ID` (`*`) for admin-wide monitoring.
 */
export function subscribeTeamRunEventsWs(
  sessionId: string,
  teamID: string,
  onEvent: (event: TeamRunEvent) => void,
  onError?: (error: string) => void,
  onReplayState?: (replaying: boolean) => void,
) {
  const effectiveSession = sessionId.trim() === '' || sessionId === 'team-monitor' ? GLOBAL_WS_SESSION_ID : sessionId;
  const stream = createEnvelopeStream({
    sessionId: effectiveSession,
    channels: ['team', 'monitor', 'system'],
    autoConnect: false,
    onReplayState: (replaying) => onReplayState?.(replaying),
  });

  stream.onType(TEAM_RUNTIME_ENVELOPE_TYPES, (env: Envelope) => {
    const mapped = teamRunEventFromEnvelope(env, teamID);
    if (mapped) {
      onEvent(mapped);
    }
  });

  stream.onType('log', (env: Envelope) => {
    const mapped = teamRunEventFromEnvelope(env, teamID);
    if (mapped) {
      onEvent(mapped);
    }
  });

  stream.onType('error', (env: Envelope) => {
    onError?.(env.error?.message ?? 'team ws error');
  });

  stream.connect();

  return {
    close: () => stream.disconnect(),
    connected: stream.connected,
  };
}

function wireTaskDeadLetter(
  row:
    | {
        id?: string;
        sourceType?: string;
        sourceId?: string;
        teamId?: string;
        teamRunId?: string;
        sessionId?: string;
        graphExecutionId?: string;
        errorMessage?: string;
        payloadJson?: string;
        status?: string;
        createdAt?: string;
        resolvedAt?: string;
      }
    | null
    | undefined,
): TaskDeadLetterRow {
  return {
    id: row?.id ?? '',
    source_type: row?.sourceType ?? '',
    source_id: row?.sourceId ?? '',
    team_id: row?.teamId ?? '',
    team_run_id: row?.teamRunId ?? '',
    session_id: row?.sessionId ?? '',
    graph_execution_id: row?.graphExecutionId ?? '',
    error_message: row?.errorMessage ?? '',
    payload_json: row?.payloadJson ?? '',
    status: row?.status ?? '',
    created_at: row?.createdAt ?? '',
    resolved_at: row?.resolvedAt ?? '',
  };
}

export async function listTaskDeadLetters(opts: {
  sessionId?: string;
  teamId?: string;
  status?: string;
  limit?: number;
}): Promise<TaskDeadLetterRow[]> {
  const svc = createTeamService();
  const res = await svc.ListTaskDeadLetters({
    sessionId: opts.sessionId?.trim() || undefined,
    teamId: opts.teamId?.trim() || undefined,
    status: opts.status?.trim() || undefined,
    limit: opts.limit ?? 50,
  });
  return (res.items ?? []).map(wireTaskDeadLetter);
}

export async function resolveTaskDeadLetter(id: string): Promise<TaskDeadLetterRow> {
  const svc = createTeamService();
  const res = await svc.ResolveTaskDeadLetter({ id: id.trim() });
  return wireTaskDeadLetter(res.item);
}
