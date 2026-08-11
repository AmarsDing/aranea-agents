import type { AuthType, OpenAIVariant, ProviderType } from './providerRuntimeOverlay.types';
import overlayData from './provider_runtime_overlay.json';

export type { AuthType, OpenAIVariant, ProviderType } from './providerRuntimeOverlay.types';

export type RuntimeProfile = {
  providerType: ProviderType;
  variant?: OpenAIVariant;
  authType: AuthType;
  apiBaseUrl?: string;
};

type OverlayJSON = Record<
  string,
  {
    provider_type: ProviderType;
    variant?: OpenAIVariant;
    auth_type: AuthType;
    api_base_url?: string;
  }
>;

function mapOverlay(raw: OverlayJSON): Record<string, RuntimeProfile> {
  const out: Record<string, RuntimeProfile> = {};
  for (const [id, row] of Object.entries(raw)) {
    out[id] = {
      providerType: row.provider_type,
      variant: row.variant,
      authType: row.auth_type,
      apiBaseUrl: row.api_base_url,
    };
  }
  return out;
}

/** models.dev provider id → trpc 运行时（与 internal/modelcatalog/runtime_overlay.json 同步） */
export const PROVIDER_RUNTIME_OVERLAY: Record<string, RuntimeProfile> = mapOverlay(overlayData as OverlayJSON);

export const PROVIDER_TYPE_OPTIONS: { label: string; value: ProviderType }[] = [
  { label: 'OpenAI Compatible', value: 'openai' },
  { label: 'Anthropic', value: 'anthropic' },
  { label: 'Gemini', value: 'gemini' },
  { label: 'Ollama', value: 'ollama' },
  { label: 'Hunyuan', value: 'hunyuan' },
  { label: 'HuggingFace', value: 'huggingface' },
  { label: 'Bedrock', value: 'bedrock' },
];

export const VARIANT_OPTIONS: { label: string; value: OpenAIVariant }[] = [
  { label: 'OpenAI', value: 'openai' },
  { label: 'DeepSeek', value: 'deepseek' },
  { label: 'Qwen', value: 'qwen' },
  { label: 'Hunyuan', value: 'hunyuan' },
];

export function runtimeProfileFor(providerId: string): RuntimeProfile {
  return PROVIDER_RUNTIME_OVERLAY[providerId] ?? { providerType: 'openai', variant: 'openai', authType: 'api_key' };
}

export function usdPer1MToMicroPer1K(usdPer1M: number): number {
  if (!Number.isFinite(usdPer1M) || usdPer1M <= 0) return 0;
  return Math.round(usdPer1M * 1000);
}

export function microPer1KToUsdPer1M(microPer1K: number): number {
  if (!Number.isFinite(microPer1K) || microPer1K <= 0) return 0;
  return microPer1K / 1000;
}

/**
 * 去除上游 models.dev 价格的 float32 加宽噪声（如 0.14000000059604645）。
 * 价格粒度为 micro-USD/1K，即 USD/1M 下 6 位小数以外恒为噪声，
 * 加载进编辑表单前统一舍入到 6 位小数，避免 number input 原样显示长小数。
 */
export function normalizeUsdPer1M(usdPer1M: number): number {
  if (!Number.isFinite(usdPer1M) || usdPer1M <= 0) return 0;
  return Math.round(usdPer1M * 1e6) / 1e6;
}
