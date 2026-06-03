import type { QTableColumn } from 'quasar';
import type { ArtifactMeta } from './types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../ui/registryTableColumns';

/** ArtifactsPage 列定义 */
export const ARTIFACT_TABLE_COLUMNS: QTableColumn<ArtifactMeta>[] = [
  registryCol<ArtifactMeta>('name', '名称', 'name', 'left', REGISTRY_COL_W.name, { sortable: true }),
  registryCol<ArtifactMeta>('session_id', 'Session', 'session_id', 'left', REGISTRY_COL_W.agent),
  registryCol<ArtifactMeta>('mime_type', 'MIME', 'mime_type', 'left', REGISTRY_COL_W.agent),
  registryCol<ArtifactMeta>('size', '大小', 'size', 'left', REGISTRY_COL_W.metric),
  registryCol<ArtifactMeta>('version', '版本', 'version', 'left', REGISTRY_COL_W.narrow),
  registryCol<ArtifactMeta>('created_at', '创建时间', 'created_at', 'left', REGISTRY_COL_W.time),
  registryColActions<ArtifactMeta>(REGISTRY_COL_W.actions, ''),
];
