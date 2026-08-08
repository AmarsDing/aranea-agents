import type { RefineLLMSettings } from '../../services/kratos/system_setting/v1/index';

// PGO-3-PROTO-02: 平台默认 Refine LLM 表单。apiKey 为 write-only
// （空 = 保留存值，非空 = 轮换）；加载时永不回填。
export type RefineLLMForm = {
  provider: string;
  model: string;
  baseUrl: string;
  apiKey: string;
};

export const DEFAULT_REFINE_LLM_FORM: RefineLLMForm = {
  provider: '',
  model: '',
  baseUrl: '',
  apiKey: '',
};

export function refineLLMFromSettings(raw?: RefineLLMSettings | null): RefineLLMForm {
  return {
    provider: raw?.provider ?? '',
    model: raw?.model ?? '',
    baseUrl: raw?.baseUrl ?? '',
    apiKey: '',
  };
}
