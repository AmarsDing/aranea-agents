import type { QTableColumn } from 'quasar';
import type {
  ChannelTypeItem,
  ChannelConfig,
  ChannelDeliveryRow,
  ChannelMetadata,
  ChannelRow,
} from '../../features/channels/types';
import { parseJSON, buildChannelWebhookURL, isLocalhostOrigin, resolvePublicWebhookOrigin } from '../../domain/channel';
import { deliveryStatusFromChannelStatus } from '../../domain/conversation';
import { presentDeliveryStatus, toneToQuasarColor } from '../../domain/conversationPresentation';
import {
  REGISTRY_COL_W,
  registryCol,
  registryColActions,
  registryColEnabled,
} from '../../features/ui/registryTableColumns';

export { parseJSON };

export function channelConfig(row: ChannelRow): ChannelConfig {
  return parseJSON<ChannelConfig>(row.config_json, {});
}

export function channelMetadata(row: ChannelRow): ChannelMetadata {
  return parseJSON<ChannelMetadata>(row.metadata_json, {});
}

export function channelType(row: ChannelRow): string {
  return channelConfig(row).type || 'unknown';
}

export function receiveMode(row: ChannelRow): string {
  return channelConfig(row).receive_mode || '-';
}

export function catalogLabelForType(catalog: ChannelTypeItem[], type: string): string {
  return catalog.find((item) => item.type === type)?.label || type;
}

/** Quasar 语义色；对齐 UX §2 success / warning / danger */
export function statusQuasarColor(status: string): string {
  if (status === 'active') return 'positive';
  if (status === 'error') return 'negative';
  if (status === 'pending_auth') return 'warning';
  return 'grey';
}

export function channelStatusBadgeText(row: ChannelRow): string {
  return row.enabled ? statusText(row) : 'disabled';
}

/** 徽标颜色与文字同源：文字显示 connected 时颜色必须为 positive，
 *  避免 DB status=error（测试失败残留）+ 运行时 connected 出现「红字 connected」 */
export function channelStatusBadgeColor(row: ChannelRow): string {
  if (!row.enabled) return 'grey';
  if (isChannelConnected(row)) return 'positive';
  return statusQuasarColor(row.status);
}

function statusText(row: ChannelRow): string {
  if (isChannelConnected(row)) return 'connected';
  return row.status || 'unknown';
}

export function isChannelConnected(row: ChannelRow): boolean {
  const meta = channelMetadata(row);
  if (meta.runtime_connected === true) {
    return row.enabled;
  }
  return row.enabled && row.status === 'active' && !meta.last_error_message;
}

export function formatChannelDate(value: string): string {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

export function channelWebhookPath(row: ChannelRow): string {
  const cfg = channelConfig(row);
  const configured = String(cfg.webhook?.path ?? '').trim();
  if (configured) {
    return configured.startsWith('/') ? configured : `/${configured}`;
  }
  const key = String(row.key || '').trim();
  return key ? `/webhooks/${key}` : '';
}

export function channelWebhookURL(row: ChannelRow): string {
  const path = channelWebhookPath(row);
  if (!path) return '';
  const meta = channelMetadata(row);
  return buildChannelWebhookURL(path, meta);
}

export function channelWebhookOrigin(row: ChannelRow): string {
  return resolvePublicWebhookOrigin(channelMetadata(row));
}

export function channelWebhookIsLocalhost(row: ChannelRow): boolean {
  return isLocalhostOrigin(channelWebhookOrigin(row));
}

export function channelSupportsWebhook(row: ChannelRow, catalog: ChannelTypeItem[]): boolean {
  const type = channelType(row);
  const item = catalog.find((entry) => entry.type === type);
  if (item) return item.supports_webhook;
  return channelConfig(row).receive_mode === 'webhook' || channelConfig(row).receive_mode === 'event';
}

export function channelExternalID(row: ChannelRow): string {
  const meta = channelMetadata(row);
  if (meta.external_id?.trim()) return meta.external_id.trim();
  const cfg = channelConfig(row).config ?? {};
  for (const key of ['app_id', 'page_id', 'bot_id', 'team_id']) {
    const value = String((cfg as Record<string, unknown>)[key] ?? '').trim();
    if (value) return value.length > 24 ? `${value.slice(0, 24)}…` : value;
  }
  return '—';
}

export async function copyChannelWebhookURL(row: ChannelRow): Promise<string> {
  const url = channelWebhookURL(row);
  if (!url) throw new Error('Webhook URL not available');
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(url);
  }
  return url;
}

export function channelTableColumns(t: (key: string) => string): QTableColumn<ChannelRow>[] {
  return [
    registryCol<ChannelRow>('name', t('channelsPage.colName'), 'name', 'left', REGISTRY_COL_W.name),
    registryCol<ChannelRow>('type', t('channelsPage.colType'), 'config_json', 'left', '15%'),
    registryCol<ChannelRow>('external_id', t('channelsPage.colExternalId'), 'metadata_json', 'left', '16%'),
    registryCol<ChannelRow>('status', t('channelsPage.colStatus'), 'status', 'left', '12%'),
    registryColEnabled<ChannelRow>(),
    registryCol<ChannelRow>('updated', t('channelsPage.colUpdated'), 'updated_at', 'left', REGISTRY_COL_W.time),
    registryColActions<ChannelRow>('80px'),
  ];
}

export function channelTurnJobsColumns(t: (key: string) => string) {
  return [
    registryCol('status', t('channelsPage.colStatus'), 'status', 'left', REGISTRY_COL_W.status),
    registryCol('peer_id', t('channelsPage.colPeerId'), 'peer_id', 'left', REGISTRY_COL_W.name),
    registryCol('session_id', t('channelsPage.colSessionId'), 'session_id', 'left', REGISTRY_COL_W.name),
    registryCol('updated_at', t('channelsPage.colUpdated'), 'updated_at', 'left', REGISTRY_COL_W.time),
  ];
}

export function channelDeliveriesColumns(t: (key: string) => string): QTableColumn<ChannelDeliveryRow>[] {
  return [
    registryCol<ChannelDeliveryRow>('status', t('channelsPage.colStatus'), 'status', 'left', REGISTRY_COL_W.status),
    registryCol<ChannelDeliveryRow>('agent_id', t('channelsPage.colAgentId'), 'agent_id', 'left', REGISTRY_COL_W.name),
    registryCol<ChannelDeliveryRow>(
      'payload',
      t('channelsPage.colPayload'),
      'payload_json',
      'left',
      REGISTRY_COL_W.name,
    ),
    registryCol<ChannelDeliveryRow>(
      'updated_at',
      t('channelsPage.colUpdated'),
      'updated_at',
      'left',
      REGISTRY_COL_W.time,
    ),
  ];
}

/** Channel Delivery 状态颜色 */
export function deliveryStatusColor(status: string) {
  return toneToQuasarColor(presentDeliveryStatus(deliveryStatusFromChannelStatus(status)).tone);
}

/** Channel Delivery 状态标签 */
export function deliveryStatusLabel(status: string) {
  return deliveryStatusFromChannelStatus(status)
    ? presentDeliveryStatus(deliveryStatusFromChannelStatus(status)).label
    : status || '—';
}
