import type { QTableColumn } from 'quasar';
import { parseHookConfig, type HookRow, type HookRuleConfig } from '../../features/hooks/types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../../features/ui/registryTableColumns';

export function hookRuleOf(row: HookRow): HookRuleConfig {
  return parseHookConfig(row.config_json);
}

type I18nT = (key: string) => string;

export function createHooksTableColumns(t: I18nT): QTableColumn<HookRow>[] {
  return [
    registryCol<HookRow>('name', t('hooksPage.colName'), 'name', 'left', REGISTRY_COL_W.nameWide),
    registryCol<HookRow>('rule', t('hooksPage.colRule'), 'config_json', 'left', REGISTRY_COL_W.desc),
    registryCol<HookRow>('sort', t('hooksPage.colSort'), 'sort_order', 'center', REGISTRY_COL_W.narrow),
    registryCol<HookRow>('enabled', t('hooksPage.colEnabled'), 'enabled', 'center', REGISTRY_COL_W.enabled, {
      sortable: false,
    }),
    registryColActions<HookRow>(REGISTRY_COL_W.actionsWide),
  ];
}

export function createHooksAgentTableColumns(t: I18nT): QTableColumn<HookRow>[] {
  return [
    registryCol<HookRow>('name', t('hooksPage.colName'), 'name', 'left', REGISTRY_COL_W.nameWide),
    registryCol<HookRow>('rule', t('hooksPage.colRule'), 'config_json', 'left', REGISTRY_COL_W.desc),
    registryColActions<HookRow>(),
  ];
}

export function createHookDeliveryTableColumns(t: I18nT) {
  return [
    registryCol('created_at', t('hooksPage.deliveries.colTime'), 'created_at', 'left', REGISTRY_COL_W.time),
    registryCol('hook_key', t('hooksPage.deliveries.colHook'), 'hook_key', 'left', REGISTRY_COL_W.desc),
    registryCol('status', t('hooksPage.deliveries.colStatus'), 'status', 'left', REGISTRY_COL_W.status),
    registryCol(
      'attempt_count',
      t('hooksPage.deliveries.colAttempts'),
      'attempt_count',
      'right',
      REGISTRY_COL_W.metric,
    ),
    registryCol(
      'max_attempts',
      t('hooksPage.deliveries.colMaxAttempts'),
      'max_attempts',
      'right',
      REGISTRY_COL_W.metric,
    ),
  ];
}

export function hookRunsTo(row: HookRow) {
  const rule = hookRuleOf(row);
  return {
    path: '/plugins/runs',
    query: {
      plugin_key: `hook:${row.key}`,
      callback_point: rule.callback_point,
    },
  };
}

export function hookConditionHint(row: HookRow, t: (key: string) => string) {
  const rule = hookRuleOf(row);
  const parts: string[] = [];
  if (rule.condition.agent_id?.trim())
    parts.push(`${t('hooksPage.conditionAgent')}: ${rule.condition.agent_id.trim()}`);
  if (rule.condition.tool_name?.trim())
    parts.push(`${t('hooksPage.conditionTool')}: ${rule.condition.tool_name.trim()}`);
  if (rule.condition.event_type?.trim())
    parts.push(`${t('hooksPage.conditionEvent')}: ${rule.condition.event_type.trim()}`);
  return parts.length ? parts.join('\n') : '';
}

/** Hook Delivery 状态颜色 */
export function hookDeliveryStatusColor(st: string) {
  if (st === 'failed') return 'negative';
  if (st === 'success') return 'positive';
  return 'grey';
}
