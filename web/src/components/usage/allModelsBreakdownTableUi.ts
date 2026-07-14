import type { QTableColumn } from 'quasar';
import type { ModelUsageBreakdownRow } from '../../features/usage/types';
import { REGISTRY_COL_W, registryCol } from '../../features/ui/registryTableColumns';

/**
 * AllModelsBreakdownTable 列定义。
 *
 * 与 PROVIDER_MODEL_TABLE_COLUMNS 一致采用 registryCol + REGISTRY_COL_W
 * 构建列宽，避免裸 q-table 散落 width 字符串。
 *
 * sortable: true 的列将触发 @request，由后端排序（与 useAllModelsBreakdown.onRequest 对接）。
 */
export const ALL_MODELS_BREAKDOWN_TABLE_COLUMNS: QTableColumn<ModelUsageBreakdownRow>[] = [
  registryCol<ModelUsageBreakdownRow>('model', '模型', 'model_api_id', 'left', '20%', { sortable: false }),
  registryCol<ModelUsageBreakdownRow>('call_count', '调用次数', 'call_count', 'right', REGISTRY_COL_W.stats, {
    sortable: true,
  }),
  registryCol<ModelUsageBreakdownRow>('total_tokens', 'Token 总量', 'total_tokens', 'right', REGISTRY_COL_W.stats, {
    sortable: true,
  }),
  registryCol<ModelUsageBreakdownRow>(
    'total_cost_micro_usd',
    '费用',
    'total_cost_micro_usd',
    'right',
    REGISTRY_COL_W.stats,
    { sortable: true },
  ),
  registryCol<ModelUsageBreakdownRow>('success_rate', '成功率', 'success_rate', 'right', REGISTRY_COL_W.narrow, {
    sortable: true,
  }),
  registryCol<ModelUsageBreakdownRow>('avg_latency_ms', '平均延迟', 'avg_latency_ms', 'right', REGISTRY_COL_W.narrow, {
    sortable: true,
  }),
  registryCol<ModelUsageBreakdownRow>(
    'avg_tokens_per_second',
    '吞吐 (tok/s)',
    'avg_tokens_per_second',
    'right',
    REGISTRY_COL_W.narrow,
    { sortable: false },
  ),
];
