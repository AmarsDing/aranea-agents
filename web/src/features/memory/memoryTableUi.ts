import type { QTableColumn } from 'quasar';
import type { CascadeProposal, L0AssemblySnapshot, MemoryFact } from './types';
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
    registryCol<MemoryFact>('scope', 'Scope', 'scope_type', 'center', '12%', { sortable: false }),
    registryCol<MemoryFact>('confidence', 'Confidence', 'confidence', 'left', '15%', { sortable: false }),
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
    registryCol<L0AssemblySnapshot>('segments', '段落', 'segments_json', 'left', REGISTRY_COL_W.category, {
      sortable: false,
    }),
    registryCol<L0AssemblySnapshot>('strategy', '裁剪策略', 'truncate_strategy', 'left', REGISTRY_COL_W.category, {
      sortable: false,
    }),
    registryColActions<L0AssemblySnapshot>(),
  ];
}
