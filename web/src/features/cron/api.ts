import { api } from "../../api/http";
import type { Agent } from "../../features/agents/api";
import type { Team } from "../../api/client";
import type { PlatformResource, PlatformResourceInput } from "../platform/api";
import type { CronTaskRun, CronTaskRunQuery } from "./types";

type ListResponse<T> = {
  items: T[];
};

export async function listCronTasks(): Promise<PlatformResource[]> {
  const { data } = await api.get<ListResponse<PlatformResource>>("/cron-tasks");
  return data.items ?? [];
}

export async function createCronTask(payload: PlatformResourceInput): Promise<PlatformResource> {
  const { data } = await api.post<PlatformResource>("/cron-tasks", payload);
  return data;
}

export async function updateCronTask(id: string, payload: Partial<PlatformResourceInput>): Promise<PlatformResource> {
  const { data } = await api.patch<PlatformResource>(`/cron-tasks/${id}`, payload);
  return data;
}

export async function deleteCronTask(id: string): Promise<void> {
  await api.delete(`/cron-tasks/${id}`);
}

export async function listCronTaskRuns(query: CronTaskRunQuery = {}): Promise<CronTaskRun[]> {
  const { data } = await api.get<ListResponse<CronTaskRun>>("/cron-task-runs", { params: cleanQuery(query) });
  return data.items ?? [];
}

export async function listCronAgents(): Promise<Agent[]> {
  const { data } = await api.get<ListResponse<Agent>>("/agents", { params: { limit: 200 } });
  return data.items ?? [];
}

export async function listCronTeams(): Promise<Team[]> {
  const { data } = await api.get<ListResponse<Team>>("/teams");
  return data.items ?? [];
}

function cleanQuery(query: CronTaskRunQuery) {
  return Object.fromEntries(Object.entries(query).filter(([, value]) => value !== "" && value !== undefined && value !== null));
}
