import type { QTableColumn } from 'quasar';
import type {
  CascadeProposal,
  CompositeSearchHit,
  L0AssemblySegmentStats,
  L0AssemblySnapshot,
  MemoryFact,
} from './types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../ui/registryTableColumns';

/** 最小翻译器签名：兼容 vue-i18n Composer 的 t()（仅简单 key 场景）。 */
export type MemoryTranslator = (key: string) => string;

/** MemorySnapshotDrawer — 段落统计行（map entry 展开为行） */
export type MemorySnapshotSegmentRow = L0AssemblySegmentStats & { section: string };

/** MemorySnapshotDrawer — Prompt 段落统计列 */
export function buildMemorySnapshotSegmentColumns(t: MemoryTranslator): QTableColumn<MemorySnapshotSegmentRow>[] {
  return [
    registryCol<MemorySnapshotSegmentRow>('section', t('memory.table.columns.section'), 'section', 'left', '20%', {
      sortable: true,
    }),
    registryCol<MemorySnapshotSegmentRow>(
      'token_estimate',
      t('memory.table.columns.tokenEstimate'),
      'token_estimate',
      'right',
      '20%',
      { sortable: true },
    ),
    registryCol<MemorySnapshotSegmentRow>(
      'message_count',
      t('memory.table.columns.messageCount'),
      'message_count',
      'right',
      '20%',
      { sortable: true },
    ),
    registryCol<MemorySnapshotSegmentRow>('detail', t('memory.table.columns.detail'), () => '', 'left', '40%', {
      sortable: false,
    }),
  ];
}

export function buildCascadeSagaTableColumns(t: MemoryTranslator): QTableColumn<CascadeProposal>[] {
  return [
    registryCol<CascadeProposal>('change', t('memory.table.columns.change'), 'old_value', 'left', REGISTRY_COL_W.name),
    registryCol<CascadeProposal>('status', t('memory.table.columns.status'), 'status', 'left', REGISTRY_COL_W.status),
    registryCol<CascadeProposal>('risk', t('memory.table.columns.risk'), 'risk_level', 'left', REGISTRY_COL_W.status),
    registryCol<CascadeProposal>(
      'affected',
      t('memory.table.columns.affected'),
      'affected_entities',
      'center',
      REGISTRY_COL_W.metric,
    ),
    registryCol<CascadeProposal>(
      'created',
      t('memory.table.columns.created'),
      'created_at',
      'left',
      REGISTRY_COL_W.time,
    ),
    registryColActions<CascadeProposal>(REGISTRY_COL_W.actions, t('memory.table.columns.actions')),
  ];
}

/** MemoryCenter — Facts 表 */
export function buildMemoryFactTableColumns(formatDate: (value: string) => string, t: MemoryTranslator) {
  return [
    // 陈述是知识列表的核心内容（memory.md §9.2 首列），必须可直接扫读，
    // 不能只靠 hover 作用域 chip 查看。
    registryCol<MemoryFact>(
      'statement',
      t('memory.table.columns.statement'),
      'statement',
      'left',
      REGISTRY_COL_W.contentWide,
      {
        sortable: false,
      },
    ),
    registryCol<MemoryFact>('scope', t('memory.table.columns.scope'), 'scope_type', 'center', REGISTRY_COL_W.metric, {
      sortable: false,
    }),
    registryCol<MemoryFact>(
      'confidence',
      t('memory.table.columns.confidence'),
      'confidence',
      'left',
      REGISTRY_COL_W.category,
      { sortable: false },
    ),
    registryCol<MemoryFact>(
      'source',
      t('memory.table.columns.source'),
      'source_kind',
      'left',
      REGISTRY_COL_W.category,
      {
        sortable: false,
      },
    ),
    // FR-12.6: compact three-stage counters "recalled / injected / cited".
    registryCol<MemoryFact>(
      'usage',
      t('memory.table.columns.usage'),
      (row) => `${row.recalled_count} / ${row.injected_count} / ${row.cited_count}`,
      'center',
      REGISTRY_COL_W.metric,
      { sortable: false },
    ),
    registryCol<MemoryFact>('updated', t('memory.table.columns.updated'), 'updated_at', 'left', REGISTRY_COL_W.time, {
      sortable: false,
      format: formatDate,
    }),
    registryColActions<MemoryFact>('10%', t('memory.table.columns.actions')),
  ];
}

/** MemoryCenter — L0 Assembly 表 */
export function buildMemoryAssemblyTableColumns(formatDate: (value: string) => string, t: MemoryTranslator) {
  return [
    registryCol<L0AssemblySnapshot>(
      'created',
      t('memory.table.columns.time'),
      'created_at',
      'left',
      REGISTRY_COL_W.time,
      {
        sortable: false,
        format: formatDate,
      },
    ),
    registryCol<L0AssemblySnapshot>(
      'model',
      t('memory.table.columns.model'),
      (row) => `${row.provider || '-'} / ${row.model || '-'}`,
      'left',
      REGISTRY_COL_W.name,
      { sortable: false },
    ),
    registryCol<L0AssemblySnapshot>(
      'ratio',
      t('memory.table.columns.used'),
      'used_ratio',
      'left',
      REGISTRY_COL_W.status,
      {
        sortable: false,
      },
    ),
    registryCol<L0AssemblySnapshot>(
      'segments',
      t('memory.table.columns.segments'),
      'segments_json',
      'left',
      REGISTRY_COL_W.category,
      { sortable: false },
    ),
    registryCol<L0AssemblySnapshot>(
      'strategy',
      t('memory.table.columns.strategy'),
      'truncate_strategy',
      'left',
      REGISTRY_COL_W.category,
      { sortable: false },
    ),
    registryColActions<L0AssemblySnapshot>(REGISTRY_COL_W.actions, t('memory.table.columns.actions')),
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

/** MemoryRecallTester — Composite Search 结果列 */
export function buildCompositeColumns(t: MemoryTranslator): QTableColumn<CompositeSearchHit>[] {
  return [
    registryCol<CompositeSearchHit>('layer', t('memory.table.columns.layer'), 'layer', 'left', REGISTRY_COL_W.nameWide),
    registryCol<CompositeSearchHit>('score', t('memory.table.columns.score'), 'score', 'right', REGISTRY_COL_W.metric),
  ];
}
