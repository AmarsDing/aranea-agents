import type { QTableColumn } from 'quasar';
import type { A2AAgentCard, A2AAuditEntry, A2AGatewayEntry, A2ARemoteAgent } from './types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../ui/registryTableColumns';

export const A2A_AUTH_TYPE_LABELS: Record<string, string> = {
  none: '无',
  api_key: 'API Key',
  bearer: 'Bearer',
  mtls: 'mTLS',
};

export function a2aAuthTypeLabel(type: string) {
  return A2A_AUTH_TYPE_LABELS[type] ?? type ?? '—';
}

/** A2A Discover 卡片列表 */
export const A2A_CARD_TABLE_COLUMNS: QTableColumn<A2AAgentCard>[] = [
  registryCol<A2AAgentCard>('agent_id', 'Agent ID', 'agent_id', 'left', REGISTRY_COL_W.agent),
  registryCol<A2AAgentCard>('display_name', '名称', 'display_name', 'left', REGISTRY_COL_W.name),
  registryCol<A2AAgentCard>('workspace', 'Workspace', 'workspace', 'left', REGISTRY_COL_W.metric),
  registryCol<A2AAgentCard>('enabled', '状态', 'enabled', 'left', REGISTRY_COL_W.status),
  registryCol<A2AAgentCard>('capabilities', '能力', 'capabilities', 'left', REGISTRY_COL_W.category),
];

/** A2A Remote 列表 */
export const A2A_REMOTE_TABLE_COLUMNS: QTableColumn<A2ARemoteAgent>[] = [
  registryCol<A2ARemoteAgent>('display_name', '名称', 'display_name', 'left', REGISTRY_COL_W.nameWide),
  registryCol<A2ARemoteAgent>('workspace', 'Workspace', 'workspace', 'left', REGISTRY_COL_W.metric),
  registryCol<A2ARemoteAgent>('auth_type', '鉴权', 'auth_type', 'left', REGISTRY_COL_W.metric),
  registryCol<A2ARemoteAgent>('enabled', '状态', 'enabled', 'left', REGISTRY_COL_W.status),
  registryCol<A2ARemoteAgent>('healthy', '健康', 'healthy', 'left', REGISTRY_COL_W.status),
  registryColActions<A2ARemoteAgent>(REGISTRY_COL_W.actions, '', 'actions'),
];

/** A2A 审计列表 */
export const A2A_AUDIT_TABLE_COLUMNS: QTableColumn<A2AAuditEntry>[] = [
  registryCol<A2AAuditEntry>('created_at', '时间', 'created_at', 'left', REGISTRY_COL_W.time),
  registryCol<A2AAuditEntry>('caller_agent_id', '调用方', 'caller_agent_id', 'left', REGISTRY_COL_W.agent),
  registryCol<A2AAuditEntry>('callee_agent_id', '被调方', 'callee_agent_id', 'left', REGISTRY_COL_W.agent),
  registryCol<A2AAuditEntry>('capability', '能力', 'capability', 'left', REGISTRY_COL_W.category),
  registryCol<A2AAuditEntry>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol<A2AAuditEntry>('duration_ms', '耗时(ms)', 'duration_ms', 'right', REGISTRY_COL_W.metric),
];

/** A2A Gateway 联邦列表 */
export const A2A_GATEWAY_TABLE_COLUMNS: QTableColumn<A2AGatewayEntry>[] = [
  registryCol<A2AGatewayEntry>('source', '来源', 'source', 'left', REGISTRY_COL_W.metric),
  registryCol<A2AGatewayEntry>('display_name', '名称', 'card.display_name', 'left', REGISTRY_COL_W.nameWide),
  registryCol<A2AGatewayEntry>('workspace', 'Workspace', 'card.workspace', 'left', REGISTRY_COL_W.metric),
  registryCol<A2AGatewayEntry>('capabilities', '能力', 'card.capabilities', 'left', REGISTRY_COL_W.category),
  registryCol<A2AGatewayEntry>('healthy', '健康', 'healthy', 'left', REGISTRY_COL_W.status),
  registryCol<A2AGatewayEntry>('endpoint_url', '端点 URL', 'endpoint_url', 'left', REGISTRY_COL_W.nameWide),
];
