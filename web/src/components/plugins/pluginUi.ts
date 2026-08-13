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

type I18nT = (key: string) => string;

export function pluginEnabledOptions(t: I18nT) {
  return [
    { label: t('pluginsPage.enabledOptions.enabled'), value: true },
    { label: t('pluginsPage.enabledOptions.disabled'), value: false },
  ];
}

export const CALLBACK_CHIP_LIMIT = 2;

/** PluginsTable 列定义（修改列宽请改此处；schema 变更会自动使拖拽缓存失效） */
export function createPluginTableColumns(t: I18nT): QTableColumn<Plugin>[] {
  return [
    registryCol<Plugin>('name', t('pluginsPage.colPlugin'), 'name', 'left', REGISTRY_COL_W.nameWide),
    registryCol<Plugin>('category', t('pluginsPage.colCategoryRisk'), 'category', 'center', '15%'),
    registryCol<Plugin>('callbacks', t('pluginsPage.colCallbacks'), 'callback_points', 'center', '24%'),
    registryCol<Plugin>('stats', t('pluginsPage.colStats'), 'invoke_count', 'center', '20%'),
    registryCol<Plugin>('enabled', t('pluginsPage.colEnabled'), 'enabled', 'center', REGISTRY_COL_W.enabled, {
      sortable: false,
    }),
    registryCol<Plugin>('scope', t('pluginsPage.colScope'), 'scope', 'center', REGISTRY_COL_W.status),
    registryColActions<Plugin>(REGISTRY_COL_W.actionsWide),
  ];
}

export function scopeLabel(scope?: string) {
  const value = (scope || 'global').trim() || 'global';
  if (value === 'global') return 'global';
  if (value.length <= 14) return value;
  return `${value.slice(0, 12)}…`;
}

export function scopeTooltip(scope: string | undefined, t: I18nT) {
  const value = (scope || 'global').trim() || 'global';
  return value === 'global' ? t('pluginsPage.scopeGlobal') : value;
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

export function riskTagClass(risk: string) {
  if (risk === 'high') return 'plugin-tag--risk-high';
  if (risk === 'medium') return 'plugin-tag--risk-medium';
  return 'plugin-tag--risk-low';
}

export function lastStatusLabel(plugin: Plugin, t: I18nT) {
  if (!plugin.last_status) return t('pluginsPage.statusNotInvoked');
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
