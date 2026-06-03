import type { KnowledgeEmbedSettings } from '../../services/kratos/system_setting/v1/index';
import { DEFAULT_KNOWLEDGE_EMBED_FORM, type KnowledgeEmbedFormState } from '../knowledge/embedder-constants';

export function knowledgeEmbedFromSettings(embed?: KnowledgeEmbedSettings | null): KnowledgeEmbedFormState {
  if (!embed) return { ...DEFAULT_KNOWLEDGE_EMBED_FORM };
  return {
    provider: embed.provider || DEFAULT_KNOWLEDGE_EMBED_FORM.provider,
    base_url: embed.baseUrl || '',
    model: embed.model || DEFAULT_KNOWLEDGE_EMBED_FORM.model,
    dim: embed.dim || DEFAULT_KNOWLEDGE_EMBED_FORM.dim,
    api_key: '',
  };
}

export type KnowledgeEmbedPatch = {
  provider: string;
  baseUrl: string;
  model: string;
  dim: number;
  apiKey?: string;
};

export function knowledgeEmbedToPatch(form: KnowledgeEmbedFormState): KnowledgeEmbedPatch {
  const patch: KnowledgeEmbedPatch = {
    provider: form.provider,
    baseUrl: form.base_url.trim(),
    model: form.model.trim(),
    dim: form.dim,
  };
  const key = form.api_key.trim();
  if (key) patch.apiKey = key;
  return patch;
}
