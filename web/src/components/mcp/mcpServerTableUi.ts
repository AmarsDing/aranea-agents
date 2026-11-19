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
  // 卡片：配置卡——名称、健康状态，传输 / 前缀 / 超时作元信息
  registryCol<PlatformResource>('name', '服务器', 'name', 'left', '20%', { cardRole: 'title' }),
  registryCol<PlatformResource>('transport', '传输', 'config_json', 'left', '12%', { cardRole: 'meta' }),
  registryCol<PlatformResource>('toolPrefix', '工具前缀', 'config_json', 'left', '14%', { cardRole: 'meta' }),
  registryCol<PlatformResource>('timeout', '超时', 'config_json', 'left', '8%', { cardRole: 'meta' }),
  registryCol<PlatformResource>('health', '健康', 'metadata_json', 'left', '12%', { cardRole: 'status' }),
  registryColEnabled<PlatformResource>(),
  registryColActions<PlatformResource>(REGISTRY_COL_W.actionsWide),
];
