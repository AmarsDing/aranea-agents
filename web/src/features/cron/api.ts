import { createCronService } from '../../services';
import { listAgents } from '../agents/api';
import { listTeams } from '../teams/api';
import type { Agent } from '../agents/types';
import type { Team } from '../teams/types';
import type { PlatformResource, PlatformResourceInput } from '../platform/types';
import type { CronTaskRun, CronTaskRunPage, CronTaskRunQuery } from './types';

export type { PlatformResource, PlatformResourceInput } from '../platform/types';
import type { CronTask as WireCronTask, CronTaskRun as WireCronTaskRun } from '../../services/kratos/cron/v1/index';
import type { CronTaskConfig, CronTaskMetadata } from './types';

const cron = createCronService();

function wireCronTask(t: WireCronTask | null | undefined): PlatformResource {
  return {
    id: t?.id ?? '',
    resource: 'cron-tasks',
    key: t?.taskKey ?? '',
    name: t?.name ?? '',
    description: t?.description ?? '',
    status: t?.status ?? '',
    enabled: t?.enabled ?? false,
    sort_order: t?.sortOrder ?? 0,
    parent_id: '',
    level: '',
    agent_id: t?.agentId ?? '',
    provider: '',
    model: '',
    is_system: false,
    config_json: t?.configJson ?? '',
    metadata_json: t?.metadataJson ?? '',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: t?.createdAt ?? '',
    updated_at: t?.updatedAt ?? '',
    deleted_at: t?.deletedAt ?? '',
  };
}

function wireCronTaskRun(r: WireCronTaskRun | null | undefined): CronTaskRun {
  return {
    id: r?.id ?? '',
    task_id: r?.taskId ?? '',
    task_name: r?.taskName ?? '',
    status: r?.status ?? '',
    started_at: r?.startedAt ?? '',
    finished_at: r?.finishedAt ?? '',
    trigger: r?.trigger ?? '',
    run_id: r?.runId ?? '',
    output_json: r?.outputJson ?? '',
    error_message: r?.errorMessage ?? '',
    created_at: r?.createdAt ?? '',
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
    metadataJson: payload.metadata_json,
  });
  return wireCronTask(row);
}

export async function updateCronTask(id: string, payload: Partial<PlatformResourceInput>): Promise<PlatformResource> {
  const task: Record<string, unknown> = { id };
  if (payload.key !== undefined) task.taskKey = payload.key;
  if (payload.name !== undefined) task.name = payload.name;
  if (payload.description !== undefined) task.description = payload.description;
  if (payload.status !== undefined) task.status = payload.status;
  if (payload.enabled !== undefined) task.enabled = payload.enabled;
  if (payload.sort_order !== undefined) task.sortOrder = payload.sort_order;
  if (payload.agent_id !== undefined) task.agentId = payload.agent_id;
  if (payload.config_json !== undefined) task.configJson = payload.config_json;
  if (payload.metadata_json !== undefined) task.metadataJson = payload.metadata_json;
  const row = await cron.UpdateCronTask({ id, task: task as WireCronTask });
  return wireCronTask(row);
}

export async function deleteCronTask(id: string): Promise<void> {
  await cron.DeleteCronTask({ id });
}

export async function triggerCronTask(id: string): Promise<CronTaskRun> {
  const row = await cron.TriggerCronTask({ id });
  return wireCronTaskRun(row);
}

export async function resetCronTaskFailures(id: string): Promise<PlatformResource> {
  const row = await cron.ResetCronTaskFailures({ id });
  return wireCronTask(row);
}

export async function listCronTaskRuns(query: CronTaskRunQuery = {}): Promise<CronTaskRunPage> {
  const res = await cron.ListCronTaskRuns({
    cronTaskId: query.cron_task_id,
    status: query.status,
    limit: query.limit,
    page: query.page,
    pageSize: query.page_size,
  });
  return { items: (res.items ?? []).map(wireCronTaskRun), total: res.total ?? 0 };
}

export async function listCronAgents(): Promise<Agent[]> {
  return listAgents({ limit: 200 });
}

export async function listCronTeams(): Promise<Team[]> {
  return listTeams();
}

export function parseCronConfig(row: PlatformResource): CronTaskConfig {
  return parseJSON<CronTaskConfig>(row.config_json, {});
}

export function parseCronMetadata(row: PlatformResource): CronTaskMetadata {
  return parseJSON<CronTaskMetadata>(row.metadata_json, {});
}

function parseJSON<T>(value: string | undefined, fallback: T): T {
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}
