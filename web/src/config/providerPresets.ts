export type ProviderModelPreset = {
  id: string;
  label: string;
  sizeLabel?: string;
  contextWindowK?: number;
  maxOutputTokens?: number;
  inputPriceMicroUsdPer1K?: number;
  outputPriceMicroUsdPer1K?: number;
  cachedInputPriceMicroUsdPer1K?: number;
  reasoningPriceMicroUsdPer1K?: number;
  embeddingPriceMicroUsdPer1K?: number;
};

export type ProviderPreset = {
  key: string;
  label: string;
  providerCode: string;
  providerType: string;
  apiBaseUrl: string;
  metadataApi: "full" | "partial" | "limited" | "none";
  metadataNote: string;
  models: ProviderModelPreset[];
};

export const PROVIDER_PRESETS: ProviderPreset[] = [
  {
    key: "openai",
    label: "OpenAI",
    providerCode: "openai",
    providerType: "OpenAI Compatible",
    apiBaseUrl: "https://api.openai.com/v1",
    metadataApi: "full",
    metadataNote: "支持标准 /models 查询；价格和上下文使用预设兜底。",
    models: [
      { id: "gpt-5.5", label: "GPT-5.5", contextWindowK: 1000, maxOutputTokens: 32768, inputPriceMicroUsdPer1K: 2000, outputPriceMicroUsdPer1K: 8000 },
      { id: "gpt-5.5-pro", label: "GPT-5.5 Pro", contextWindowK: 1000, maxOutputTokens: 32768, inputPriceMicroUsdPer1K: 10000, outputPriceMicroUsdPer1K: 30000 },
      { id: "gpt-5", label: "GPT-5", contextWindowK: 1000, maxOutputTokens: 32768, inputPriceMicroUsdPer1K: 2000, outputPriceMicroUsdPer1K: 8000 },
      { id: "o3", label: "o3", contextWindowK: 200, maxOutputTokens: 100000, inputPriceMicroUsdPer1K: 2000, outputPriceMicroUsdPer1K: 8000 }
    ]
  },
  {
    key: "anthropic",
    label: "Anthropic (Claude)",
    providerCode: "anthropic",
    providerType: "Anthropic",
    apiBaseUrl: "https://api.anthropic.com",
    metadataApi: "full",
    metadataNote: "官方 SDK 提供 ListModels；代理环境不可用时使用 Claude 预设。",
    models: [
      { id: "claude-opus-4-6", label: "Claude Opus 4.6", contextWindowK: 200, maxOutputTokens: 8192, inputPriceMicroUsdPer1K: 15000, outputPriceMicroUsdPer1K: 75000 },
      { id: "claude-sonnet-4-6", label: "Claude Sonnet 4.6", contextWindowK: 200, maxOutputTokens: 8192, inputPriceMicroUsdPer1K: 3000, outputPriceMicroUsdPer1K: 15000 },
      { id: "claude-haiku-4-5", label: "Claude Haiku 4.5", contextWindowK: 200, maxOutputTokens: 8192, inputPriceMicroUsdPer1K: 800, outputPriceMicroUsdPer1K: 4000 }
    ]
  },
  {
    key: "gemini",
    label: "Google (Gemini)",
    providerCode: "gemini",
    providerType: "Google Gemini",
    apiBaseUrl: "https://generativelanguage.googleapis.com",
    metadataApi: "full",
    metadataNote: "支持 GET /v1beta/models 查询模型列表。",
    models: [
      { id: "gemini-2.5-pro", label: "Gemini 2.5 Pro", contextWindowK: 1000, maxOutputTokens: 65536, inputPriceMicroUsdPer1K: 1250, outputPriceMicroUsdPer1K: 10000 },
      { id: "gemini-2.5-flash-lite", label: "Gemini 2.5 Flash Lite", contextWindowK: 1000, maxOutputTokens: 65536, inputPriceMicroUsdPer1K: 100, outputPriceMicroUsdPer1K: 400 }
    ]
  },
  {
    key: "baidu-qianfan",
    label: "百度智能云 (千帆)",
    providerCode: "baidu-qianfan",
    providerType: "Custom",
    apiBaseUrl: "https://aip.baidubce.com/rpc/2.0/ai_custom/v1/wenxinworkshop",
    metadataApi: "full",
    metadataNote: "千帆平台提供模型列表和详情等管理 API。",
    models: [
      { id: "ERNIE-4.5-Turbo", label: "ERNIE 4.5 Turbo", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "ERNIE-X1-Turbo", label: "ERNIE X1 Turbo", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "DeepSeek R1", label: "DeepSeek R1 on Qianfan", contextWindowK: 64, maxOutputTokens: 8192 },
      { id: "DeepSeek V3", label: "DeepSeek V3 on Qianfan", contextWindowK: 64, maxOutputTokens: 8192 }
    ]
  },
  {
    key: "aliyun-qwen",
    label: "阿里云 (通义千问)",
    providerCode: "aliyun-qwen",
    providerType: "OpenAI Compatible",
    apiBaseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    metadataApi: "full",
    metadataNote: "阿里云官方 SDK 提供 ListModels 等接口。",
    models: [
      { id: "qwen3.6-plus", label: "Qwen 3.6 Plus", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "qwen3.5-plus", label: "Qwen 3.5 Plus", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "qwen3.5-flash", label: "Qwen 3.5 Flash", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "qwen-max", label: "Qwen Max", contextWindowK: 128, maxOutputTokens: 8192 }
    ]
  },
  {
    key: "tencent-hunyuan",
    label: "腾讯云 (混元)",
    providerCode: "tencent-hunyuan",
    providerType: "Custom",
    apiBaseUrl: "https://hunyuan.tencentcloudapi.com",
    metadataApi: "full",
    metadataNote: "腾讯云标准云 API 可进行调用和管理。",
    models: [
      { id: "hunyuan-2.0-thinking", label: "Hunyuan 2.0 Thinking", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "hunyuan-2.0-instruct", label: "Hunyuan 2.0 Instruct", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "hunyuan-a13b", label: "Hunyuan A13B", sizeLabel: "13B", contextWindowK: 32, maxOutputTokens: 4096 },
      { id: "hunyuan-lite", label: "Hunyuan Lite", contextWindowK: 32, maxOutputTokens: 4096 }
    ]
  },
  {
    key: "iflytek-spark",
    label: "科大讯飞 (星火)",
    providerCode: "iflytek-spark",
    providerType: "Custom",
    apiBaseUrl: "wss://spark-api.xf-yun.com/v4.0/chat",
    metadataApi: "limited",
    metadataNote: "WebSocket 调用接口不直接提供 RESTful 模型列表查询。",
    models: [
      { id: "Spark4.0 Ultra", label: "Spark 4.0 Ultra", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "Spark Max", label: "Spark Max", contextWindowK: 32, maxOutputTokens: 4096 },
      { id: "Spark Pro", label: "Spark Pro", contextWindowK: 32, maxOutputTokens: 4096 },
      { id: "Spark Lite", label: "Spark Lite", contextWindowK: 8, maxOutputTokens: 4096 },
      { id: "Spark X2", label: "Spark X2", contextWindowK: 128, maxOutputTokens: 8192 }
    ]
  },
  {
    key: "zhipu-glm",
    label: "智谱AI (GLM)",
    providerCode: "zhipu-glm",
    providerType: "OpenAI Compatible",
    apiBaseUrl: "https://open.bigmodel.cn/api/paas/v4",
    metadataApi: "full",
    metadataNote: "兼容 OpenAI 规范，支持编程获取模型列表。",
    models: [
      { id: "GLM-5.1", label: "GLM 5.1", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "GLM-5", label: "GLM 5", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "GLM-4.7-Flash", label: "GLM 4.7 Flash", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "GLM-Z1-Rumination", label: "GLM Z1 Rumination", contextWindowK: 128, maxOutputTokens: 8192 }
    ]
  },
  {
    key: "meta-llama",
    label: "Meta (Llama)",
    providerCode: "meta-llama",
    providerType: "OpenAI Compatible",
    apiBaseUrl: "https://api.llama.com",
    metadataApi: "full",
    metadataNote: "兼容 OpenAI 规范，可通过 /models 获取列表。",
    models: [
      { id: "Llama-4-Scout", label: "Llama 4 Scout", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "Llama-4-Maverick", label: "Llama 4 Maverick", contextWindowK: 128, maxOutputTokens: 8192 }
    ]
  },
  {
    key: "mistral",
    label: "Mistral AI",
    providerCode: "mistral",
    providerType: "OpenAI Compatible",
    apiBaseUrl: "https://api.mistral.ai",
    metadataApi: "full",
    metadataNote: "官方 API 提供 List Models 接口。",
    models: [
      { id: "mistral-large-latest", label: "Mistral Large Latest", contextWindowK: 128, maxOutputTokens: 8192, inputPriceMicroUsdPer1K: 2000, outputPriceMicroUsdPer1K: 6000 },
      { id: "mistral-small-latest", label: "Mistral Small Latest", contextWindowK: 128, maxOutputTokens: 8192, inputPriceMicroUsdPer1K: 200, outputPriceMicroUsdPer1K: 600 }
    ]
  },
  {
    key: "deepseek",
    label: "DeepSeek",
    providerCode: "deepseek",
    providerType: "OpenAI Compatible",
    apiBaseUrl: "https://api.deepseek.com",
    metadataApi: "full",
    metadataNote: "兼容 OpenAI 规范，提供 /models 接口。",
    models: [
      { id: "deepseek-chat", label: "DeepSeek Chat / V3", contextWindowK: 64, maxOutputTokens: 8192, inputPriceMicroUsdPer1K: 270, outputPriceMicroUsdPer1K: 1100 },
      { id: "deepseek-reasoner", label: "DeepSeek Reasoner / R1", contextWindowK: 64, maxOutputTokens: 8192, inputPriceMicroUsdPer1K: 550, outputPriceMicroUsdPer1K: 2190, reasoningPriceMicroUsdPer1K: 2190 }
    ]
  },
  {
    key: "cohere",
    label: "Cohere",
    providerCode: "cohere",
    providerType: "Custom",
    apiBaseUrl: "https://api.cohere.ai",
    metadataApi: "full",
    metadataNote: "提供 List Models 端点。",
    models: [
      { id: "command-r-plus", label: "Command R Plus", contextWindowK: 128, maxOutputTokens: 4096 },
      { id: "command-r", label: "Command R", contextWindowK: 128, maxOutputTokens: 4096 }
    ]
  },
  {
    key: "moonshot-kimi",
    label: "月之暗面 (Kimi)",
    providerCode: "moonshot-kimi",
    providerType: "OpenAI Compatible",
    apiBaseUrl: "https://api.moonshot.cn/v1",
    metadataApi: "full",
    metadataNote: "兼容 OpenAI 规范，可查询模型列表。",
    models: [
      { id: "kimi-k2-instruct", label: "Kimi K2 Instruct", contextWindowK: 128, maxOutputTokens: 8192 }
    ]
  },
  {
    key: "stability",
    label: "Stability AI",
    providerCode: "stability",
    providerType: "Custom",
    apiBaseUrl: "https://api.stability.ai",
    metadataApi: "none",
    metadataNote: "未提供公开模型列表 API，需手动维护。",
    models: [
      { id: "stable-diffusion-xl", label: "Stable Diffusion XL" },
      { id: "stable-video-diffusion", label: "Stable Video Diffusion" }
    ]
  },
  {
    key: "volcengine-doubao",
    label: "字节跳动 (豆包)",
    providerCode: "volcengine-doubao",
    providerType: "OpenAI Compatible",
    apiBaseUrl: "https://ark.cn-beijing.volces.com/api/v3",
    metadataApi: "full",
    metadataNote: "火山方舟平台标准 API 支持模型管理。",
    models: [
      { id: "Doubao-pro", label: "Doubao Pro", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "Doubao-lite", label: "Doubao Lite", contextWindowK: 32, maxOutputTokens: 4096 }
    ]
  },
  {
    key: "openrouter",
    label: "OpenRouter",
    providerCode: "openrouter",
    providerType: "OpenAI Compatible",
    apiBaseUrl: "https://openrouter.ai/api/v1",
    metadataApi: "full",
    metadataNote: "聚合平台，支持 /models 查询上下文长度和价格。",
    models: [
      { id: "openai/gpt-5", label: "OpenAI GPT-5", contextWindowK: 1000, maxOutputTokens: 32768, inputPriceMicroUsdPer1K: 2000, outputPriceMicroUsdPer1K: 8000 },
      { id: "anthropic/claude-sonnet-4-6", label: "Claude Sonnet 4.6", contextWindowK: 200, maxOutputTokens: 8192, inputPriceMicroUsdPer1K: 3000, outputPriceMicroUsdPer1K: 15000 },
      { id: "deepseek/deepseek-chat-v3-0324", label: "DeepSeek V3", contextWindowK: 64, maxOutputTokens: 8192, inputPriceMicroUsdPer1K: 270, outputPriceMicroUsdPer1K: 1100 },
      { id: "deepseek/deepseek-r1", label: "DeepSeek R1", contextWindowK: 64, maxOutputTokens: 8192, inputPriceMicroUsdPer1K: 550, outputPriceMicroUsdPer1K: 2190 }
    ]
  },
  {
    key: "ollama",
    label: "Ollama",
    providerCode: "ollama",
    providerType: "Ollama",
    apiBaseUrl: "http://localhost:11434/v1",
    metadataApi: "partial",
    metadataNote: "本地模型通常可列出模型名，但上下文和价格需按本地配置维护。",
    models: [
      { id: "llama3.1", label: "Llama 3.1", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "qwen2.5", label: "Qwen 2.5", contextWindowK: 128, maxOutputTokens: 8192 },
      { id: "deepseek-r1", label: "DeepSeek R1 Local", contextWindowK: 128, maxOutputTokens: 8192 }
    ]
  },
  {
    key: "custom",
    label: "自定义",
    providerCode: "custom",
    providerType: "Custom",
    apiBaseUrl: "",
    metadataApi: "none",
    metadataNote: "手动维护连接、上下文和价格。",
    models: []
  }
];

export function findProviderPreset(keyOrCode: string) {
  return PROVIDER_PRESETS.find((preset) => preset.key === keyOrCode || preset.providerCode === keyOrCode);
}

export function findModelPreset(providerKeyOrCode: string, modelId: string) {
  return findProviderPreset(providerKeyOrCode)?.models.find((model) => model.id === modelId);
}
