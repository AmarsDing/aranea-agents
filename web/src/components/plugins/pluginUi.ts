import type { QTableColumn } from 'quasar';
import type { Plugin } from '../../features/plugins/types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../../features/ui/registryTableColumns';

// 与后端内置插件注册表（internal/plugin/trpc/registry.go）的 category 枚举保持一致
export const PLUGIN_CATEGORY_OPTIONS = [
  'observability',
  'tracking',
  'reliability',
  'security',
  'governance',
  'routing',
].map((value) => ({ label: value, value }));

export const PLUGIN_ENABLED_OPTIONS = [
  { label: '已启用', value: true },
  { label: '已停用', value: false },
] as const;

export const CALLBACK_CHIP_LIMIT = 2;

/** PluginsTable 列定义（修改列宽请改此处；schema 变更会自动使拖拽缓存失效） */
export const PLUGIN_TABLE_COLUMNS: QTableColumn<Plugin>[] = [
  registryCol<Plugin>('name', 'Plugin', 'name', 'left', REGISTRY_COL_W.nameWide),
  registryCol<Plugin>('category', '类型 / 风险', 'category', 'center', '15%'),
  registryCol<Plugin>('callbacks', 'Callback', 'callback_points', 'center', '24%'),
  registryCol<Plugin>('stats', '运行', 'invoke_count', 'center', '20%'),
  registryCol<Plugin>('enabled', '启用', 'enabled', 'center', REGISTRY_COL_W.enabled, { sortable: false }),
  registryCol<Plugin>('scope', '作用域', 'scope', 'center', REGISTRY_COL_W.status),
  registryColActions<Plugin>(REGISTRY_COL_W.actionsWide),
];

export function scopeLabel(scope?: string) {
  const value = (scope || 'global').trim() || 'global';
  if (value === 'global') return 'global';
  if (value.length <= 14) return value;
  return `${value.slice(0, 12)}…`;
}

export function scopeTooltip(scope?: string) {
  const value = (scope || 'global').trim() || 'global';
  return value === 'global' ? '全局生效' : value;
}

export function formatCallbacksSummary(points?: string[]) {
  const list = points ?? [];
  if (!list.length) return '—';
  return list.join(', ');
}

export function visibleCallbackPoints(points?: string[]) {
  return (points ?? []).slice(0, CALLBACK_CHIP_LIMIT);
}

export function hiddenCallbackCount(points?: string[]) {
  return Math.max(0, (points ?? []).length - CALLBACK_CHIP_LIMIT);
}

export function formatPluginDate(value?: string) {
  if (!value) return '—';
  return new Date(value).toLocaleString();
}

export function riskColor(risk: string) {
  if (risk === 'high') return 'negative';
  if (risk === 'medium') return 'warning';
  return 'positive';
}

export function riskTagClass(risk: string) {
  if (risk === 'high') return 'plugin-tag--risk-high';
  if (risk === 'medium') return 'plugin-tag--risk-medium';
  return 'plugin-tag--risk-low';
}

export function lastStatusLabel(plugin: Plugin) {
  if (!plugin.last_status) return '未调用';
  return plugin.last_status;
}

export function lastStatusTone(plugin: Plugin) {
  const status = (plugin.last_status || '').toLowerCase();
  if (!status) return 'idle';
  if (status.includes('error') || status.includes('fail') || status.includes('block')) return 'bad';
  if (status.includes('warn')) return 'warn';
  return 'ok';
}

export function lastStatusTagClass(plugin: Plugin) {
  const tone = lastStatusTone(plugin);
  if (tone === 'bad') return 'plugin-tag--risk-high';
  if (tone === 'warn') return 'plugin-tag--risk-medium';
  if (tone === 'ok') return 'plugin-tag--risk-low';
  return '';
}

export function prettyJSON(value: string, emptyLabel = '{}') {
  try {
    const parsed = JSON.parse(value || '{}');
    if (parsed && typeof parsed === 'object' && Object.keys(parsed).length === 0) {
      return emptyLabel;
    }
    return JSON.stringify(parsed, null, 2);
  } catch {
    return value || emptyLabel;
  }
}

export function pluginRunsTo(plugin: Plugin) {
  return { path: '/plugins/runs', query: { plugin_key: plugin.key } };
}
