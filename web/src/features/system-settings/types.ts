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

// Ecosystem preset types

export interface IndustryLoadInfo {
  loaded: boolean;
  loaded_at?: string;
  agents?: number;
  teams?: number;
  taxonomy_nodes?: number;
}

export type EcosystemLoadedStatus = Record<string, IndustryLoadInfo>;

export interface EcosystemLoadResult {
  agents_created: number;
  teams_created: number;
  taxonomy_nodes: number;
}

export interface EcosystemLoadResponse {
  results: Record<string, EcosystemLoadResult>;
  already_loaded?: string[];
  errors?: Record<string, string>;
}

export interface EcosystemUnloadResult {
  agents_deleted: number;
  teams_deleted: number;
  taxonomy_nodes_deleted: number;
  teams_modified?: number;
}

export interface EcosystemUnloadResponse {
  results: Record<string, EcosystemUnloadResult>;
  errors?: Record<string, string>;
}
