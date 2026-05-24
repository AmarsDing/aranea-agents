import type { ChannelCatalogItem, ChannelConfig, ChannelMetadata, ChannelRow } from "../../features/channels/types";
import { buildChannelWebhookURL, isLocalhostOrigin, resolvePublicWebhookOrigin } from "../../features/channels/publicWebhookOrigin";

export function parseJSON<T>(value: string | undefined, fallback: T): T {
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

export function channelConfig(row: ChannelRow): ChannelConfig {
  return parseJSON<ChannelConfig>(row.config_json, {});
}

export function channelMetadata(row: ChannelRow): ChannelMetadata {
  return parseJSON<ChannelMetadata>(row.metadata_json, {});
}

export function channelType(row: ChannelRow): string {
  return channelConfig(row).type || "unknown";
}

export function receiveMode(row: ChannelRow): string {
  return channelConfig(row).receive_mode || "-";
}

export function catalogLabelForType(catalog: ChannelCatalogItem[], type: string): string {
  return catalog.find((item) => item.type === type)?.label || type;
}

/** Quasar 语义色；对齐 UX §2 success / warning / danger */
export function statusQuasarColor(status: string): string {
  if (status === "active") return "positive";
  if (status === "error") return "negative";
  if (status === "pending_auth") return "warning";
  return "grey";
}

export function channelStatusBadgeText(row: ChannelRow): string {
  return row.enabled ? statusText(row) : "disabled";
}

function statusText(row: ChannelRow): string {
  if (isChannelConnected(row)) return "connected";
  return row.status || "unknown";
}

export function isChannelConnected(row: ChannelRow): boolean {
  const meta = channelMetadata(row);
  if (meta.runtime_connected === true) {
    return row.enabled;
  }
  return row.enabled && row.status === "active" && !meta.last_error_message;
}

export function formatChannelDate(value: string): string {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

export function channelWebhookPath(row: ChannelRow): string {
  const cfg = channelConfig(row);
  const configured = String(cfg.webhook?.path ?? "").trim();
  if (configured) {
    return configured.startsWith("/") ? configured : `/${configured}`;
  }
  const key = String(row.key || "").trim();
  return key ? `/webhooks/${key}` : "";
}

export function channelWebhookURL(row: ChannelRow): string {
  const path = channelWebhookPath(row);
  if (!path) return "";
  const meta = channelMetadata(row);
  return buildChannelWebhookURL(path, meta);
}

export function channelWebhookOrigin(row: ChannelRow): string {
  return resolvePublicWebhookOrigin(channelMetadata(row));
}

export function channelWebhookIsLocalhost(row: ChannelRow): boolean {
  return isLocalhostOrigin(channelWebhookOrigin(row));
}

export function channelSupportsWebhook(row: ChannelRow, catalog: ChannelCatalogItem[]): boolean {
  const type = channelType(row);
  const item = catalog.find((entry) => entry.type === type);
  if (item) return item.supports_webhook;
  return channelConfig(row).receive_mode === "webhook" || channelConfig(row).receive_mode === "event";
}

export function channelExternalID(row: ChannelRow): string {
  const meta = channelMetadata(row);
  if (meta.external_id?.trim()) return meta.external_id.trim();
  const cfg = channelConfig(row).config ?? {};
  for (const key of ["app_id", "page_id", "bot_id", "team_id"]) {
    const value = String((cfg as Record<string, unknown>)[key] ?? "").trim();
    if (value) return value.length > 24 ? `${value.slice(0, 24)}…` : value;
  }
  return "—";
}

export async function copyChannelWebhookURL(row: ChannelRow): Promise<string> {
  const url = channelWebhookURL(row);
  if (!url) throw new Error("Webhook URL 不可用");
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(url);
  }
  return url;
}
