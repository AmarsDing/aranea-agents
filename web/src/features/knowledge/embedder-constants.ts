export type KnowledgeEmbedFormState = {
  provider: string;
  base_url: string;
  model: string;
  dim: number;
  api_key: string;
};

export const KNOWLEDGE_EMBED_PROVIDER_OPTIONS = [
  { label: 'OpenAI / 兼容', value: 'openai' },
  { label: 'Ollama', value: 'ollama' },
  { label: 'Gemini', value: 'gemini' },
  { label: 'HuggingFace TEI', value: 'huggingface' },
] as const;

export const DEFAULT_KNOWLEDGE_EMBED_FORM: KnowledgeEmbedFormState = {
  provider: 'openai',
  base_url: '',
  model: 'text-embedding-3-small',
  dim: 1536,
  api_key: '',
};
