import { createPluginService } from '../../services';
import type { PaginatedResponse, Plugin, PluginListQuery, PluginRun, PluginRunListQuery } from './types';

/** 模块级 service 单例，避免每次 API 调用重复创建实例 */
const pluginService = createPluginService();

function mapPluginRow(row: unknown): Plugin {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? '');
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  const b = (snake: string, camel: string) => Boolean(r[snake] ?? r[camel]);
  const rawPerms = r.permissions as Record<string, unknown> | undefined;
  const p = rawPerms ?? {};
  const pb = (snake: string, camel: string) => Boolean(p[snake] ?? p[camel] ?? false);
  const cbs = r.callback_points ?? r.callbackPoints;
  const callbackPoints = Array.isArray(cbs) ? cbs.map((x) => String(x)) : [];
  const lastInvoked = s('last_invoked_at', 'lastInvokedAt');
  const lastStat = s('last_status', 'lastStatus');
  const risk = s('risk_level', 'riskLevel');
  return {
    id: s('id', 'id'),
    key: s('key', 'key'),
    name: s('name', 'name'),
    description: s('description', 'description'),
    category: s('category', 'category'),
    risk_level: risk || 'low',
    enabled: b('enabled', 'enabled'),
    scope: s('scope', 'scope'),
    callback_points: callbackPoints,
    sort_order: n('sort_order', 'sortOrder'),
    config_schema_json: s('config_schema_json', 'configSchemaJson'),
    config_json: s('config_json', 'configJson'),
    default_config_json: s('default_config_json', 'defaultConfigJson'),
    invoke_count: n('invoke_count', 'invokeCount'),
    block_count: n('block_count', 'blockCount'),
    error_count: n('error_count', 'errorCount'),
    last_invoked_at: lastInvoked || undefined,
    last_status: lastStat || undefined,
    created_at: s('created_at', 'createdAt'),
    updated_at: s('updated_at', 'updatedAt'),
    permissions: {
      can_view: pb('can_view', 'canView'),
      can_toggle: pb('can_toggle', 'canToggle'),
      can_edit_config: pb('can_edit_config', 'canEditConfig'),
      can_view_logs: pb('can_view_logs', 'canViewLogs'),
    },
  };
}

export async function listPlugins(query: PluginListQuery = {}): Promise<PaginatedResponse<Plugin>> {
  const svc = pluginService;
  let enabled: string | undefined;
  if (query.enabled === true) enabled = 'true';
  else if (query.enabled === false) enabled = 'false';
  const page = query.page ?? 1;
  const pageSize = query.page_size ?? 20;
  const res = await svc.ListPlugins({
    search: query.search?.trim() || undefined,
    category: query.category?.trim() || undefined,
    enabled,
    callbackPoint: query.callback_point?.trim() || undefined,
    page,
    pageSize,
  });
  const items = (res.items ?? []).map(mapPluginRow);
  return {
    items,
    total: Number(res.total ?? 0),
    page: Number(res.page ?? page),
    page_size: Number(res.pageSize ?? pageSize),
  };
}

export async function togglePluginEnabled(id: string, enabled: boolean): Promise<Plugin> {
  const svc = pluginService;
  const row = await svc.TogglePluginEnabled({ id, enabled });
  return mapPluginRow(row);
}

export async function updatePluginConfig(id: string, configJSON: string): Promise<Plugin> {
  const svc = pluginService;
  const row = await svc.UpdatePluginConfig({ id, configJson: configJSON });
  return mapPluginRow(row);
}

export async function updatePluginSortOrder(id: string, sortOrder: number): Promise<Plugin> {
  const svc = pluginService;
  const row = await svc.UpdatePluginSortOrder({ id, sortOrder });
  return mapPluginRow(row);
}

export async function updatePluginScope(id: string, scope: string): Promise<Plugin> {
  const svc = pluginService;
  const row = await svc.UpdatePluginScope({ id, scope });
  return mapPluginRow(row);
}

function mapPluginRunRow(row: unknown): PluginRun {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? '');
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  return {
    id: s('id', 'id'),
    plugin_key: s('plugin_key', 'pluginKey'),
    plugin_id: s('plugin_id', 'pluginId'),
    session_id: s('session_id', 'sessionId'),
    agent_id: s('agent_id', 'agentId'),
    callback_point: s('callback_point', 'callbackPoint'),
    status: s('status', 'status'),
    duration_ms: n('duration_ms', 'durationMs'),
    detail_json: s('detail_json', 'detailJson'),
    created_at: s('created_at', 'createdAt'),
  };
}

export async function listPluginRuns(query: PluginRunListQuery = {}): Promise<PaginatedResponse<PluginRun>> {
  const svc = pluginService;
  const page = query.page ?? 1;
  const pageSize = query.page_size ?? 20;
  const res = await svc.ListPluginRuns({
    pluginKey: query.plugin_key?.trim() || undefined,
    pluginId: query.plugin_id?.trim() || undefined,
    sessionId: query.session_id?.trim() || undefined,
    agentId: query.agent_id?.trim() || undefined,
    callbackPoint: query.callback_point?.trim() || undefined,
    status: query.status?.trim() || undefined,
    from: query.from?.trim() || undefined,
    to: query.to?.trim() || undefined,
    page,
    pageSize,
  });
  return {
    items: (res.items ?? []).map(mapPluginRunRow),
    total: Number(res.total ?? 0),
    page: Number(res.page ?? page),
    page_size: Number(res.pageSize ?? pageSize),
  };
}

export async function deleteAllPluginRuns(): Promise<number> {
  const svc = pluginService;
  const res = await svc.DeleteAllPluginRuns({});
  return Number(res.deletedCount ?? 0);
}
