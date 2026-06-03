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
  registryCol<PlatformResource>('transport', '传输', 'config_json', 'left', '20%'),
  registryCol<PlatformResource>('health', '健康', 'metadata_json', 'left', '20%'),
  registryColEnabled<PlatformResource>('状态'),
  registryColActions<PlatformResource>('100px'),
];
