import { api } from "../../api/http";
import type { PaginatedResponse, Plugin, PluginListQuery } from "./types";

function compactParams(params: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(params).filter(([, value]) => value !== undefined && value !== null && value !== ""));
}

export async function listPlugins(query: PluginListQuery = {}): Promise<PaginatedResponse<Plugin>> {
  const { data } = await api.get("/plugins", {
    params: compactParams({
      search: query.search,
      category: query.category,
      enabled: query.enabled,
      callback_point: query.callback_point,
      page: query.page ?? 1,
      page_size: query.page_size ?? 20
    })
  });
  return data;
}

export async function togglePluginEnabled(id: string, enabled: boolean): Promise<Plugin> {
  const { data } = await api.patch(`/plugins/${id}/enabled`, { enabled });
  return data;
}

export async function updatePluginConfig(id: string, configJSON: string): Promise<Plugin> {
  const { data } = await api.put(`/plugins/${id}/config`, { config_json: configJSON });
  return data;
}
