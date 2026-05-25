export type AuthType = "api_key" | "secret_id_key" | "aws_config" | "none";

export type ProviderType =
  | "openai"
  | "anthropic"
  | "gemini"
  | "ollama"
  | "hunyuan"
  | "huggingface"
  | "bedrock";

export type OpenAIVariant = "openai" | "deepseek" | "qwen" | "hunyuan";

export type ProviderModelPreset = {
  id: string;
  label: string;
  sizeLabel?: string;
  contextWindowK?: number;
  maxOutputTokens?: number;
  /** @deprecated prefer inputUsdPer1M — kept for legacy inspect fallback */
  inputPriceMicroUsdPer1K?: number;
  outputPriceMicroUsdPer1K?: number;
};

export type ProviderPreset = {
  key: string;
  label: string;
  providerCode: string;
  providerType: ProviderType;
  variant?: OpenAIVariant;
  authType: AuthType;
  apiBaseUrl: string;
  metadataApi: "full" | "partial" | "limited" | "none";
  metadataNote: string;
  models: ProviderModelPreset[];
};

export {
  PROVIDER_TYPE_OPTIONS,
  VARIANT_OPTIONS,
  runtimeProfileFor,
  type RuntimeProfile,
} from "./providerRuntimeOverlay";

import { runtimeProfileFor, type AuthType as OverlayAuthType } from "./providerRuntimeOverlay";

/** 自定义模式下的轻量预设（无模型列表；目录模式请用 models.dev API） */
export const PROVIDER_PRESETS: ProviderPreset[] = [
  presetShell("openai", "OpenAI", "openai"),
  presetShell("anthropic", "Anthropic", "anthropic"),
  presetShell("google", "Google Gemini", "google"),
  presetShell("deepseek", "DeepSeek", "deepseek"),
  presetShell("alibaba-cn", "阿里云百炼", "alibaba-cn"),
  presetShell("moonshotai-cn", "Moonshot CN", "moonshotai-cn"),
  presetShell("zhipuai", "智谱 AI", "zhipuai"),
  presetShell("openrouter", "OpenRouter", "openrouter"),
  presetShell("ollama", "Ollama（本地）", "ollama", "none"),
  presetShell("hunyuan", "腾讯混元", "hunyuan"),
  presetShell("huggingface", "HuggingFace", "huggingface"),
  presetShell("amazon-bedrock", "AWS Bedrock", "amazon-bedrock"),
  {
    key: "custom",
    label: "完全自定义",
    providerCode: "custom",
    providerType: "openai",
    variant: "openai",
    authType: "api_key",
    apiBaseUrl: "",
    metadataApi: "none",
    metadataNote: "手动填写 Provider / 模型 / 定价；不参与 catalog 自动同步。",
    models: [],
  },
];

function presetShell(
  key: string,
  label: string,
  providerCode: string,
  authOverride?: OverlayAuthType
): ProviderPreset {
  const rt = runtimeProfileFor(providerCode);
  const authType = authOverride ?? rt.authType;
  return {
    key,
    label,
    providerCode,
    providerType: rt.providerType as ProviderType,
    variant: rt.variant as OpenAIVariant | undefined,
    authType: authType as AuthType,
    apiBaseUrl: rt.apiBaseUrl ?? "",
    metadataApi: rt.apiBaseUrl ? "partial" : "limited",
    metadataNote: "选择后将自动从 models.dev 加载模型列表与定价；运行时类型由 overlay 决定。",
    models: [],
  };
}

export function findProviderPreset(keyOrCode: string) {
  return PROVIDER_PRESETS.find((preset) => preset.key === keyOrCode || preset.providerCode === keyOrCode);
}

export function findModelPreset(providerKeyOrCode: string, modelId: string) {
  return findProviderPreset(providerKeyOrCode)?.models.find((model) => model.id === modelId);
}

export function getAuthTypeForProviderType(providerType: ProviderType): AuthType {
  switch (providerType) {
  case "hunyuan":
    return "secret_id_key";
  case "ollama":
    return "none";
  case "bedrock":
    return "aws_config";
  default:
    return "api_key";
  }
}
