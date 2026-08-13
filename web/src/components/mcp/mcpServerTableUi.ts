import type { QTableColumn } from 'quasar';
import type { PlatformResource } from '../../features/platform/types';
import {
  REGISTRY_COL_W,
  registryCol,
  registryColActions,
  registryColEnabled,
} from '../../features/ui/registryTableColumns';

/** McpServersTable 列定义 */
export const MCP_SERVER_TABLE_COLUMNS: QTableColumn<PlatformResource>[] = [
  registryCol<PlatformResource>('name', '服务器', 'name', 'left', '20%'),
  registryCol<PlatformResource>('transport', '传输', 'config_json', 'left', '12%'),
  registryCol<PlatformResource>('toolPrefix', '工具前缀', 'config_json', 'left', '14%'),
  registryCol<PlatformResource>('timeout', '超时', 'config_json', 'left', '8%'),
  registryCol<PlatformResource>('health', '健康', 'metadata_json', 'left', '12%'),
  registryColEnabled<PlatformResource>(),
  registryColActions<PlatformResource>(REGISTRY_COL_W.actionsWide),
];
