import { REGISTRY_COL_W, registryCol, type RegistryTableColumn } from '../../features/ui/registryTableColumns';
import type { MemoryRecallHit } from '../../features/memory/types';

/** 最小翻译器签名：兼容 vue-i18n Composer 的 t()（仅简单 key 场景）。 */
export type RecallHitTranslator = (key: string) => string;

export function buildRecallHitColumns(t: RecallHitTranslator): RegistryTableColumn<MemoryRecallHit>[] {
  return [
    registryCol('id', t('memory.table.columns.id'), 'id', 'left', REGISTRY_COL_W.nameWide),
    registryCol('total', t('memory.table.columns.total'), (row) => row.scores.total, 'right', REGISTRY_COL_W.metric),
    registryCol(
      'cross_encoder',
      t('memory.table.columns.ce'),
      (row) => row.scores.cross_encoder,
      'right',
      REGISTRY_COL_W.metric,
    ),
  ];
}
