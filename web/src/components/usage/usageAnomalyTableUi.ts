import { REGISTRY_COL_W, registryCol } from '../../features/ui/registryTableColumns';
import type { ModelTokenUsageEvent } from '../../features/usage/types';

export const usageAnomalyColumns = [
  registryCol('occurred_at', '时间', 'occurred_at', 'left', REGISTRY_COL_W.time),
  registryCol('model_api_id', '模型', 'model_api_id', 'left', REGISTRY_COL_W.name),
  registryCol('agent_id', 'Agent', 'agent_id', 'left', REGISTRY_COL_W.agent),
  registryCol('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
];
