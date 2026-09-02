import type { ModelTokenUsageEvent } from './types';
import { REGISTRY_COL_W, registryCol } from '../ui/registryTableColumns';

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
  registryCol<ModelTokenUsageEvent>('usage_kind', '来源', 'usage_kind', 'left', REGISTRY_COL_W.category),
  // LBG-6：思考强度档位（off/low/medium/high/max），未记录的旧数据回退 '—'
  registryCol<ModelTokenUsageEvent>('effort', 'Effort', 'effort', 'left', REGISTRY_COL_W.status, {
    format: (val: unknown) => String(val ?? '').trim() || '—',
  }),
  registryCol<ModelTokenUsageEvent>('provider_code', 'Provider', 'provider_code', 'left', REGISTRY_COL_W.category),
  registryCol<ModelTokenUsageEvent>('model_api_id', '模型', 'model_api_id', 'left', REGISTRY_COL_W.name),
  registryCol<ModelTokenUsageEvent>('agent_id', 'Agent', 'agent_id', 'left', REGISTRY_COL_W.agent),
  registryCol<ModelTokenUsageEvent>('session_id', 'Session', 'session_id', 'left', REGISTRY_COL_W.session),
  registryCol<ModelTokenUsageEvent>('total_tokens', 'Tokens', 'total_tokens', 'right', REGISTRY_COL_W.status),
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
