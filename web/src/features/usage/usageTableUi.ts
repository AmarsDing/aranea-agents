import type { ModelTokenUsageEvent } from './types';
import { REGISTRY_COL_W, registryCol } from '../ui/registryTableColumns';
import { i18n } from '../../i18n';

/** 来源分类展示名：i18n 映射命中则返回标签，否则回退原始枚举值（新枚举未翻译时仍可辨认）。 */
export function usageKindLabel(kind: string): string {
  const key = (kind ?? '').trim();
  if (!key) return '—';
  const t = i18n.global.te(`usageKinds.${key}`) ? i18n.global.t(`usageKinds.${key}`) : '';
  return t || key;
}

/** 将 UTC ISO 时间戳格式化为本地 "MM-DD HH:mm" 格式。 */
function formatLocalTime(iso: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return iso;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** UsageEventsPage 列定义 */
export const USAGE_EVENT_TABLE_COLUMNS = [
  registryCol<ModelTokenUsageEvent>('occurred_at', '时间', 'occurred_at', 'left', REGISTRY_COL_W.time, {
    format: (val: unknown) => formatLocalTime(String(val ?? '')),
  }),
  registryCol<ModelTokenUsageEvent>('usage_kind', '来源', 'usage_kind', 'left', REGISTRY_COL_W.category, {
    format: (val: unknown) => usageKindLabel(String(val ?? '')),
  }),
  // LBG-6：思考强度档位（off/low/medium/high/max），未记录的旧数据回退 '—'
  registryCol<ModelTokenUsageEvent>('effort', 'Effort', 'effort', 'left', REGISTRY_COL_W.status, {
    format: (val: unknown) => String(val ?? '').trim() || '—',
  }),
  registryCol<ModelTokenUsageEvent>('provider_code', 'Provider', 'provider_code', 'left', REGISTRY_COL_W.category),
  registryCol<ModelTokenUsageEvent>('model_api_id', '模型', 'model_api_id', 'left', REGISTRY_COL_W.name),
  registryCol<ModelTokenUsageEvent>('agent_id', 'Agent', 'agent_id', 'left', REGISTRY_COL_W.agent),
  registryCol<ModelTokenUsageEvent>('session_id', 'Session', 'session_id', 'left', REGISTRY_COL_W.session),
  registryCol<ModelTokenUsageEvent>('total_tokens', 'Tokens', 'total_tokens', 'right', REGISTRY_COL_W.status),
  // 缓存命中（读）与推理 tokens：undefined=旧行未记录，显示 —（P0-2 归一化后新行恒有值）
  registryCol<ModelTokenUsageEvent>('cached_input_tokens', '缓存命中', 'cached_input_tokens', 'right', REGISTRY_COL_W.status, {
    format: (val: unknown) => (val == null ? '—' : String(val)),
  }),
  registryCol<ModelTokenUsageEvent>('reasoning_tokens', '推理', 'reasoning_tokens', 'right', REGISTRY_COL_W.status, {
    format: (val: unknown) => (val == null ? '—' : String(val)),
  }),
  registryCol<ModelTokenUsageEvent>(
    'total_cost_micro_usd',
    '费用',
    'total_cost_micro_usd',
    'right',
    REGISTRY_COL_W.metric,
  ),
  registryCol<ModelTokenUsageEvent>('latency_ms', '延迟', 'latency_ms', 'right', REGISTRY_COL_W.metric),
  registryCol<ModelTokenUsageEvent>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
];
