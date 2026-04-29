
import { createTeamService } from "../../services";
import type {
  Team as WireTeam,
  TeamRun as WireTeamRun,
  TeamRunStep as WireTeamRunStep
} from "../../services/kratos/team/v1/index";

export type {
  Team,
  TeamDefinition,
  TeamRun,
  TeamRunEvent,
  TeamRunStep
} from "../../services/clientLegacy";

function wireTeam(t: WireTeam | null | undefined): import("../../services/clientLegacy").Team {
  return {
    id: t?.id ?? "",
    team_key: t?.teamKey ?? "",
    display_name: t?.displayName ?? "",
    status: t?.status ?? "",
    is_default: t?.isDefault ?? false,
    definition_json: t?.definitionJson ?? "",
    adk_app_name: t?.adkAppName ?? "",
    created_at: t?.createdAt ?? "",
    updated_at: t?.updatedAt ?? "",
    deleted_at: t?.deletedAt ?? ""
  };
}

function wireRun(r: WireTeamRun | null | undefined): import("../../services/clientLegacy").TeamRun {
  return {
    id: r?.id ?? "",
    team_id: r?.teamId ?? "",
    session_id: r?.sessionId ?? "",
    message_id: r?.messageId ?? "",
    mode: r?.mode ?? "",
    status: r?.status ?? "",
    input_preview: r?.inputPreview ?? "",
    output_preview: r?.outputPreview ?? "",
    token_in: r?.tokenIn ?? 0,
    token_out: r?.tokenOut ?? 0,
    cost_micro_usd: Number(r?.costMicroUsd ?? 0),
    duration_ms: r?.durationMs ?? 0,
    error_message: r?.errorMessage ?? "",
    topology_json: r?.topologyJson ?? "",
    started_at: r?.startedAt ?? "",
    finished_at: r?.finishedAt ?? "",
    created_at: r?.createdAt ?? "",
    updated_at: r?.updatedAt ?? ""
  };
}

function wireStep(s: WireTeamRunStep | null | undefined): import("../../services/clientLegacy").TeamRunStep {
  return {
    id: s?.id ?? "",
    run_id: s?.runId ?? "",
    team_id: s?.teamId ?? "",
    agent_id: s?.agentId ?? "",
    agent_key: s?.agentKey ?? "",
    agent_name: s?.agentName ?? "",
    role: s?.role ?? "",
    sort_order: s?.sortOrder ?? 0,
    status: s?.status ?? "",
    input_preview: s?.inputPreview ?? "",
    output_preview: s?.outputPreview ?? "",
    token_in: s?.tokenIn ?? 0,
    token_out: s?.tokenOut ?? 0,
    cost_micro_usd: Number(s?.costMicroUsd ?? 0),
    duration_ms: s?.durationMs ?? 0,
    error_message: s?.errorMessage ?? "",
    started_at: s?.startedAt ?? "",
    finished_at: s?.finishedAt ?? "",
    created_at: s?.createdAt ?? ""
  };
}

function patchToWire(
  payload: Partial<import("../../services/clientLegacy").Team>
): WireTeam {
  const t = {} as WireTeam;
  if (payload.team_key !== undefined) t.teamKey = payload.team_key;
  if (payload.display_name !== undefined) t.displayName = payload.display_name;
  if (payload.status !== undefined) t.status = payload.status;
  if (payload.is_default !== undefined) t.isDefault = payload.is_default;
  if (payload.definition_json !== undefined) t.definitionJson = payload.definition_json;
  if (payload.adk_app_name !== undefined) t.adkAppName = payload.adk_app_name;
  if (payload.created_at !== undefined) t.createdAt = payload.created_at;
  if (payload.updated_at !== undefined) t.updatedAt = payload.updated_at;
  if (payload.deleted_at !== undefined) t.deletedAt = payload.deleted_at;
  return t;
}

export async function listTeams(): Promise<import("../../services/clientLegacy").Team[]> {
  const svc = createTeamService();
  const res = await svc.ListTeams({});
  const items = res.items ?? [];
  return items.map(wireTeam);
}

export async function createTeam(
  payload: Partial<import("../../services/clientLegacy").Team>
): Promise<import("../../services/clientLegacy").Team> {
  const svc = createTeamService();
  const data = await svc.CreateTeam({
    teamKey: payload.team_key,
    displayName: payload.display_name,
    status: payload.status,
    definitionJson: payload.definition_json,
    adkAppName: payload.adk_app_name
  });
  return wireTeam(data);
}

export async function updateTeam(
  id: string,
  payload: Partial<import("../../services/clientLegacy").Team>
): Promise<import("../../services/clientLegacy").Team> {
  const svc = createTeamService();
  const data = await svc.UpdateTeam({ id, team: patchToWire(payload) });
  return wireTeam(data);
}

export async function duplicateTeam(id: string): Promise<import("../../services/clientLegacy").Team> {
  const svc = createTeamService();
  const data = await svc.DuplicateTeam({ id });
  return wireTeam(data);
}

export async function deleteTeam(id: string): Promise<void> {
  const svc = createTeamService();
  await svc.DeleteTeam({ id });
}

export async function listTeamRuns(
  teamID?: string,
  limit = 50
): Promise<import("../../services/clientLegacy").TeamRun[]> {
  const svc = createTeamService();
  const res = await svc.ListTeamRuns({ teamId: teamID, limit });
  const items = res.items ?? [];
  return items.map(wireRun);
}

export async function listTeamRunSteps(
  runID: string
): Promise<import("../../services/clientLegacy").TeamRunStep[]> {
  const svc = createTeamService();
  const res = await svc.ListTeamRunSteps({ runId: runID });
  const items = res.items ?? [];
  return items.map(wireStep);
}

export { subscribeTeamRunEvents } from "../../services/clientLegacy";
