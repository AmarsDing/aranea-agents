import type { QTableColumn } from 'quasar';
import type { ArtifactMeta } from './types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../ui/registryTableColumns';

/**
 * ArtifactsPage 列定义（工厂函数，i18n 标签）。
 * 不含 session 列：「全部产物」Tab 组头已展示会话，「会话产物」Tab 由筛选条件确定会话，列内重复展示 36 位 UUID 无信息量。
 */
export function createArtifactColumns(t: (key: string) => string): QTableColumn<ArtifactMeta>[] {
  return [
    registryCol<ArtifactMeta>('name', t('artifact.page.colName'), 'name', 'left', REGISTRY_COL_W.name, { sortable: true }),
    registryCol<ArtifactMeta>('mime_type', 'MIME', 'mime_type', 'left', REGISTRY_COL_W.agent),
    registryCol<ArtifactMeta>('size', t('artifact.page.colSize'), 'size', 'left', REGISTRY_COL_W.metric),
    registryCol<ArtifactMeta>('version', t('artifact.page.colVersion'), 'version', 'left', REGISTRY_COL_W.narrow),
    registryCol<ArtifactMeta>('created_at', t('artifact.page.colCreatedAt'), 'created_at', 'left', REGISTRY_COL_W.time),
    registryColActions<ArtifactMeta>(REGISTRY_COL_W.actions, ''),
  ];
}
