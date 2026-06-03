import type { KnowledgeEmbedPatch } from './knowledge-embed';
import type { EvalLLMForm } from './eval-llm';
import type { WebResearchPatch } from './web-research';

export type UpdateSystemSettingsInput = {
  rootDirectory: string;
  workDirectory: string;
  globalMonthlyMicroUsd?: number;
  a2aPublicBaseUrl?: string;
  mcpAllowAdhocHttp?: boolean;
  knowledgeEmbed?: KnowledgeEmbedPatch;
  evalLLM?: EvalLLMForm;
  webResearch?: WebResearchPatch;
};

export type TestWebResearchInput = {
  provider?: string;
  apiKey?: string;
  maxResults?: number;
  fetchTop?: number;
  searchDepth?: string;
  timeoutSec?: number;
  httpProxy?: string;
};

export type TestWebResearchResult = {
  ok?: boolean;
  message?: string;
  provider?: string;
  resultCount?: number;
  latencyMs?: number;
};
