/** 与后台/数据库联动的可选项；当前为前后端联调占位 */
export const CHAT_MODE_OPTIONS = [
  { label: "标准对话", value: "default" },
  { label: "深思考", value: "plan" },
  { label: "仅代码", value: "code" }
] as const;

export const CHAT_MODEL_PROVIDER_OPTIONS = [
  { label: "OpenAI 兼容", value: "openai" },
  { label: "Anthropic", value: "anthropic" },
  { label: "Gemini", value: "gemini" },
  { label: "Ollama", value: "ollama" },
  { label: "混元", value: "hunyuan" },
  { label: "HuggingFace", value: "huggingface" },
  { label: "Bedrock", value: "bedrock" }
] as const;

const LS_KEY_MODE = "chat:dialog_mode";
const LS_KEY_PROVIDER = "chat:model_provider";
const LS_KEY_MODEL = "chat:model";

export function loadDialogModeFromStorage(fallback: string) {
  if (typeof localStorage === "undefined") return fallback;
  return localStorage.getItem(LS_KEY_MODE) || fallback;
}

export function loadProviderFromStorage(fallback: string) {
  if (typeof localStorage === "undefined") return fallback;
  return localStorage.getItem(LS_KEY_PROVIDER) || fallback;
}

export function loadModelFromStorage(fallback: string) {
  if (typeof localStorage === "undefined") return fallback;
  return localStorage.getItem(LS_KEY_MODEL) || fallback;
}

export function saveDialogModeToStorage(v: string) {
  localStorage.setItem(LS_KEY_MODE, v);
}

export function saveProviderToStorage(v: string) {
  localStorage.setItem(LS_KEY_PROVIDER, v);
}

export function saveModelToStorage(v: string) {
  localStorage.setItem(LS_KEY_MODEL, v);
}
