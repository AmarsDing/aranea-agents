import type { ChannelCatalogItem, ChannelConfig, ChannelMetadata, ChannelRow } from "../../features/channels/types";

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
  return row.enabled && row.status === "active" && !meta.last_error_message;
}

export function formatChannelDate(value: string): string {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}
