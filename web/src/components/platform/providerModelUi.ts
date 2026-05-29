import type { QTableColumn } from "quasar";
import type { PlatformResource, ProviderConfig, ModelCategory, CapabilityChip } from "../../features/platform/types";
import {
  REGISTRY_COL_W,
  registryCol,
  registryColActions
} from "../../features/ui/registryTableColumns";
import { toNullableNumber, hasPricingConfigured } from "../../features/platform/providerUtils";

/** ProviderModelsTable 列定义 */
export const PROVIDER_MODEL_TABLE_COLUMNS: QTableColumn<PlatformResource>[] = [
  registryCol<PlatformResource>("model", "模型", "name", "left", "28%"),
  registryCol<PlatformResource>("size", "大小", "model", "center", REGISTRY_COL_W.narrow),
  registryCol<PlatformResource>("ctx", "上下文", "model", "center", REGISTRY_COL_W.metric),
  registryCol<PlatformResource>("tps", "TPS", "model", "center", REGISTRY_COL_W.metric),
  registryCol<PlatformResource>("usage", "热度 / 用量", "model", "left", "18%"),
  registryCol<PlatformResource>("secret", "密钥", "model", "left", "12%"),
  registryColActions<PlatformResource>(REGISTRY_COL_W.actionsWide)
];

/** 非 models 资源的 ResourceManager 通用表 */
export const PLATFORM_RESOURCE_TABLE_COLUMNS: QTableColumn<PlatformResource>[] = [
  registryCol<PlatformResource>("name", "Name", "name", "left", REGISTRY_COL_W.name, { sortable: true }),
  registryCol<PlatformResource>("key", "Key", "key", "left", REGISTRY_COL_W.name, { sortable: true }),
  registryCol<PlatformResource>("provider", "Provider", "provider", "left", REGISTRY_COL_W.category),
  registryCol<PlatformResource>("model", "Model", "model", "left", REGISTRY_COL_W.name),
  registryCol<PlatformResource>("status", "Status", "status", "left", REGISTRY_COL_W.status),
  registryColActions<PlatformResource>()
];

export function getProviderConfig(row: PlatformResource): ProviderConfig {
  if (!row.config_json) return {};
  try {
    const value = JSON.parse(row.config_json) as ProviderConfig;
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

export function providerDisplayName(row: PlatformResource, config: ProviderConfig): string {
  return config.provider_display_name || row.provider || row.key;
}

export function modelDisplayName(row: PlatformResource): string {
  return row.name || row.model || "未设置模型";
}

export function providerCategories(config: ProviderConfig): ModelCategory[] {
  const values = config.model_category;
  return Array.isArray(values) ? values.filter((category) => category?.value && category?.label && category?.tooltip) : [];
}

export function providerCapabilityChips(config: ProviderConfig): CapabilityChip[] {
  const values = config.capability_chips;
  return Array.isArray(values) ? values.filter((chip) => chip?.key && chip?.label) : [];
}

export function providerHasApiKey(config: ProviderConfig): boolean {
  return Boolean(config.api_key_set || config.api_key || config.secret_id || config.aws_region);
}

export function showVariantChip(config: ProviderConfig): boolean {
  const pt = (config.provider_type || "").toLowerCase();
  const variant = (config.variant || "").toLowerCase();
  return pt === "openai" && variant !== "" && variant !== "openai";
}

export function haChipLabel(config: ProviderConfig): string {
  const mode = (config.ha_mode || "").toLowerCase();
  if (mode === "failover") return "Failover";
  if (mode === "hedge") return "Hedge";
  return "";
}

export function haTagClass(config: ProviderConfig): string {
  return (config.ha_mode || "").toLowerCase() === "hedge" ? "provider-tag--ha-hedge" : "provider-tag--ha-failover";
}

export function hotnessScore(config: ProviderConfig): number {
  const score = toNullableNumber(config.model_hotness_score);
  return score === null ? 0 : Math.max(0, Math.min(100, Math.round(score)));
}

export function hotnessLabel(score: number): string {
  if (score >= 80) return "热门";
  if (score >= 50) return "活跃";
  if (score >= 20) return "低频";
  return "冷门";
}

export function hotnessTone(score: number): string {
  if (score >= 80) return "hot";
  if (score >= 50) return "warm";
  if (score >= 20) return "cool";
  return "idle";
}

export function formatTps(value: ProviderConfig["tokens_per_second"]) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  const rounded = numberValue >= 100 ? Math.round(numberValue) : Math.round(numberValue * 10) / 10;
  return `${rounded} tok/s`;
}

export function formatContextWindow(value: ProviderConfig["context_window_k"]) {
  const numberValue = toNullableNumber(value);
  return numberValue === null ? "—" : `${numberValue}K`;
}

export function formatCount(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(numberValue);
}

export function formatMicroUsd(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return `$${(numberValue / 1_000_000).toFixed(4)}`;
}

export function formatPercent(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return `${Math.round(numberValue * 100)}%`;
}

export function listSecretDisplay(visible: boolean, revealedApiKey: string) {
  return visible ? revealedApiKey || "••••••" : "••••••";
}

export function formatLatency(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return `${Math.round(numberValue)} ms`;
}

export function formatCompact(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 10_000) return `${(value / 10_000).toFixed(0)}k`;
  return String(value);
}

export function rowPricingNotConfigured(row: PlatformResource): boolean {
  const config = getProviderConfig(row);
  return !hasPricingConfigured(config);
}
