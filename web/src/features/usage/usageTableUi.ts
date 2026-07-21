import type { ModelTokenUsageEvent } from './types';
import { REGISTRY_COL_W, registryCol } from '../ui/registryTableColumns';

/** UsageEventsPage 列定义 */
export const USAGE_EVENT_TABLE_COLUMNS = [
  registryCol<ModelTokenUsageEvent>('occurred_at', '时间', 'occurred_at', 'left', REGISTRY_COL_W.time),
  registryCol<ModelTokenUsageEvent>('usage_kind', '来源', 'usage_kind', 'left', REGISTRY_COL_W.category),
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
