import { createCronService } from "../../services";
import { listAgents } from "../agents/api";
import { listTeams } from "../teams/api";
import type { Agent } from "../agents/api";
import type { Team } from "../teams/api";
import type { PlatformResource, PlatformResourceInput } from "../platform/api";
import type { CronTaskRun, CronTaskRunQuery } from "./types";
import type { CronTask as WireCronTask, CronTaskRun as WireCronTaskRun } from "../../services/kratos/cron/v1/index";

const cron = createCronService();

function wireCronTask(t: WireCronTask | null | undefined): PlatformResource {
  return {
    id: t?.id ?? "",
    resource: "cron-tasks",
    key: t?.taskKey ?? "",
    name: t?.name ?? "",
    description: t?.description ?? "",
    status: t?.status ?? "",
    enabled: t?.enabled ?? false,
    sort_order: t?.sortOrder ?? 0,
    parent_id: "",
    level: "",
    agent_id: t?.agentId ?? "",
    provider: "",
    model: "",
    config_json: t?.configJson ?? "",
    metadata_json: t?.metadataJson ?? "",
    created_at: t?.createdAt ?? "",
    updated_at: t?.updatedAt ?? "",
    deleted_at: t?.deletedAt ?? ""
  };
}

function wireCronTaskRun(r: WireCronTaskRun | null | undefined): CronTaskRun {
  return {
    id: r?.id ?? "",
    task_id: r?.taskId ?? "",
    task_name: r?.taskName ?? "",
    status: r?.status ?? "",
    started_at: r?.startedAt ?? "",
    finished_at: r?.finishedAt ?? "",
    trigger: r?.trigger ?? "",
    run_id: r?.runId ?? "",
    output_json: r?.outputJson ?? "",
    error_message: r?.errorMessage ?? "",
    created_at: r?.createdAt ?? ""
  };
}

export async function listCronTasks(): Promise<PlatformResource[]> {
  const res = await cron.ListCronTasks({});
  return (res.items ?? []).map(wireCronTask);
}

export async function createCronTask(payload: PlatformResourceInput): Promise<PlatformResource> {
  const row = await cron.CreateCronTask({
    taskKey: payload.key,
    name: payload.name,
    description: payload.description,
    status: payload.status,
    enabled: payload.enabled,
    sortOrder: payload.sort_order,
    agentId: payload.agent_id,
    configJson: payload.config_json,
    metadataJson: payload.metadata_json
  });
  return wireCronTask(row);
}

export async function updateCronTask(id: string, payload: Partial<PlatformResourceInput>): Promise<PlatformResource> {
  const row = await cron.UpdateCronTask({
    id,
    task: {
      taskKey: payload.key,
      name: payload.name,
      description: payload.description,
      status: payload.status,
      enabled: payload.enabled,
      sortOrder: payload.sort_order,
      agentId: payload.agent_id,
      configJson: payload.config_json,
      metadataJson: payload.metadata_json
    }
  });
  return wireCronTask(row);
}

export async function deleteCronTask(id: string): Promise<void> {
  await cron.DeleteCronTask({ id });
}

export async function listCronTaskRuns(query: CronTaskRunQuery = {}): Promise<CronTaskRun[]> {
  const res = await cron.ListCronTaskRuns({
    cronTaskId: query.cron_task_id,
    status: query.status,
    limit: query.limit
  });
  return (res.items ?? []).map(wireCronTaskRun);
}

export async function listCronAgents(): Promise<Agent[]> {
  return listAgents({ limit: 200 });
}

export async function listCronTeams(): Promise<Team[]> {
  return listTeams();
}
