import type { QTableColumn } from 'quasar';
import type { PluginRun } from './types';
import { REGISTRY_COL_W, registryCol } from '../ui/registryTableColumns';
import { formatDate } from '../../shared/format';

type I18nT = (key: string) => string;

/** PluginRunsPage 列定义 */
export function createPluginRunsTableColumns(t: I18nT): QTableColumn<PluginRun>[] {
  return [
    // created_at 为 UTC ISO 串，统一格式化为本地时间（与详情弹框「最近调用」一致）
    registryCol<PluginRun>('created_at', t('pluginsPage.runs.colTime'), 'created_at', 'left', REGISTRY_COL_W.time, {
      format: (val: unknown) => formatDate(typeof val === 'string' ? val : undefined),
    }),
    registryCol<PluginRun>('plugin_key', t('pluginsPage.runs.colPlugin'), 'plugin_key', 'left', REGISTRY_COL_W.name),
    registryCol<PluginRun>('agent_id', t('pluginsPage.runs.colAgent'), 'agent_id', 'left', REGISTRY_COL_W.agent),
    registryCol<PluginRun>(
      'callback_point',
      t('pluginsPage.runs.colCallbackPoint'),
      'callback_point',
      'left',
      REGISTRY_COL_W.status,
    ),
    registryCol<PluginRun>('status', t('pluginsPage.runs.colStatus'), 'status', 'left', REGISTRY_COL_W.status),
    registryCol<PluginRun>(
      'duration_ms',
      t('pluginsPage.runs.colDuration'),
      'duration_ms',
      'right',
      REGISTRY_COL_W.metric,
    ),
  ];
}
