import type { QTableColumn } from 'quasar';
import type { CascadeProposal, CompositeSearchHit, L0AssemblySnapshot, MemoryFact, MemoryRelation } from './types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../ui/registryTableColumns';

export const CASCADE_SAGA_TABLE_COLUMNS: QTableColumn<CascadeProposal>[] = [
  registryCol<CascadeProposal>('change', '更名', 'old_value', 'left', REGISTRY_COL_W.name),
  registryCol<CascadeProposal>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol<CascadeProposal>('risk', '风险', 'risk_level', 'left', REGISTRY_COL_W.status),
  registryCol<CascadeProposal>('affected', '影响实体', 'affected_entities', 'center', REGISTRY_COL_W.metric),
  registryCol<CascadeProposal>('created', '创建时间', 'created_at', 'left', REGISTRY_COL_W.time),
  registryColActions<CascadeProposal>(),
];

/** MemoryCenter — Facts 表 */
export function buildMemoryFactTableColumns(formatDate: (value: string) => string) {
  return [
    registryCol<MemoryFact>('scope', 'Scope', 'scope_type', 'center', REGISTRY_COL_W.metric, { sortable: false }),
    registryCol<MemoryFact>('confidence', 'Confidence', 'confidence', 'left', REGISTRY_COL_W.category, {
      sortable: false,
    }),
    registryCol<MemoryFact>('source', 'Source', 'source_kind', 'left', REGISTRY_COL_W.category, { sortable: false }),
    registryCol<MemoryFact>('updated', 'Updated', 'updated_at', 'left', REGISTRY_COL_W.time, {
      sortable: false,
      format: formatDate,
    }),
    registryColActions<MemoryFact>('10%', '操作'),
  ];
}

/** MemoryCenter — L0 Assembly 表 */
export function buildMemoryAssemblyTableColumns(formatDate: (value: string) => string) {
  return [
    registryCol<L0AssemblySnapshot>('created', '时间', 'created_at', 'left', REGISTRY_COL_W.time, {
      sortable: false,
      format: formatDate,
    }),
    registryCol<L0AssemblySnapshot>(
      'model',
      '模型',
      (row) => `${row.provider || '-'} / ${row.model || '-'}`,
      'left',
      REGISTRY_COL_W.name,
      { sortable: false },
    ),
    registryCol<L0AssemblySnapshot>('ratio', 'Used', 'used_ratio', 'left', REGISTRY_COL_W.status, { sortable: false }),
    registryCol<L0AssemblySnapshot>('segments', '段落数', 'segments_json', 'left', REGISTRY_COL_W.category, {
      sortable: false,
    }),
    registryCol<L0AssemblySnapshot>('strategy', '裁剪策略', 'truncate_strategy', 'left', REGISTRY_COL_W.category, {
      sortable: false,
    }),
    registryColActions<L0AssemblySnapshot>(),
  ];
}

/** Memory Session 状态颜色 */
export function memorySessionStatusColor(status?: string) {
  if (status === 'active' || status === 'completed') return 'positive';
  if (status === 'paused' || status === 'pending') return 'warning';
  if (status === 'failed' || status === 'cancelled' || status === 'timeout') return 'negative';
  return 'blue-grey';
}

/** Memory Cascade 状态颜色 */
export function memoryCascadeStatusColor(status: string) {
  switch (status) {
    case 'pending':
      return 'grey-7';
    case 'applied':
      return 'positive';
    case 'partial':
      return 'warning';
    case 'failed':
      return 'negative';
    case 'rejected':
      return 'deep-orange';
    default:
      return 'grey-7';
  }
}

/** MemoryGraphExplorer — Neighborhood BFS 关系列 */
export const RELATION_COLUMNS: QTableColumn<MemoryRelation>[] = [
  registryCol<MemoryRelation>('source_id', 'Source', 'source_id', 'left', REGISTRY_COL_W.name),
  registryCol<MemoryRelation>('relation_type', 'Relation', 'relation_type', 'left', '11%'),
  registryCol<MemoryRelation>('target_id', 'Target', 'target_id', 'left', REGISTRY_COL_W.name),
  registryCol<MemoryRelation>('weight', 'Weight', 'weight', 'right', REGISTRY_COL_W.metric),
];

/** MemoryRecallTester — Composite Search 结果列 */
export const COMPOSITE_COLUMNS: QTableColumn<CompositeSearchHit>[] = [
  registryCol<CompositeSearchHit>('layer', 'Layer', 'layer', 'left', REGISTRY_COL_W.nameWide),
  registryCol<CompositeSearchHit>('score', 'Score', 'score', 'right', REGISTRY_COL_W.metric),
];
