import type { ModelCategory, PlatformResource, ProviderConfig } from './types';

export function errorMessage(error: unknown): string {
  if (typeof error === 'object' && error && 'response' in error) {
    const response = (error as { response?: { data?: { error?: string } } }).response;
    if (response?.data?.error) return response.data.error;
  }
  return error instanceof Error ? error.message : '模型检查失败';
}

export function toNullableNumber(value: unknown): number | null {
  if (value === '' || value === null || value === undefined) return null;
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue : null;
}

export function toNumber(value: unknown, fallback: number = 0): number {
  const numberValue = toNullableNumber(value);
  return numberValue === null ? fallback : numberValue;
}

export function getConfig(row: PlatformResource): ProviderConfig {
  if (!row.config_json) return {};
  try {
    const value = JSON.parse(row.config_json) as ProviderConfig;
    return value && typeof value === 'object' ? value : {};
  } catch {
    return {};
  }
}

export function getCategories(row: PlatformResource): ModelCategory[] {
  const categories = getConfig(row).model_category;
  if (!Array.isArray(categories)) return [];
  return categories.filter((category) => category?.value && category?.label && category?.tooltip);
}

export function hasPricingConfigured(config: ProviderConfig): boolean {
  if (toNullableNumber(config.input_price_micro_usd_per_1k)) return true;
  if (toNullableNumber(config.output_price_micro_usd_per_1k)) return true;
  if (toNullableNumber(config.cached_input_price_micro_usd_per_1k)) return true;
  if (toNullableNumber(config.reasoning_price_micro_usd_per_1k)) return true;
  if (toNullableNumber(config.embedding_price_micro_usd_per_1k)) return true;
  const cost = config.cost;
  if (cost) {
    if (
      (cost.input_usd_per_1m ?? 0) > 0 ||
      (cost.output_usd_per_1m ?? 0) > 0 ||
      (cost.cache_read_usd_per_1m ?? 0) > 0 ||
      (cost.cache_write_usd_per_1m ?? 0) > 0 ||
      (cost.reasoning_usd_per_1m ?? 0) > 0 ||
      (cost.embedding_usd_per_1m ?? 0) > 0
    ) {
      return true;
    }
  }
  return false;
}
