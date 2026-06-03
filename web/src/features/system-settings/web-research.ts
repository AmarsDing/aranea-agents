import type { WebResearchSettings } from '../../services/kratos/system_setting/v1/index';

export type WebResearchFormState = {
  provider: string;
  api_key: string;
  max_results: number;
  fetch_top: number;
  search_depth: string;
  timeout_sec: number;
  http_proxy: string;
};

export const DEFAULT_WEB_RESEARCH_FORM: WebResearchFormState = {
  provider: 'tavily',
  api_key: '',
  max_results: 8,
  fetch_top: 5,
  search_depth: 'basic',
  timeout_sec: 15,
  http_proxy: '',
};

export const WEB_RESEARCH_PROVIDER_OPTIONS = [
  { label: 'Tavily', value: 'tavily' },
  { label: 'SerpAPI', value: 'serpapi' },
] as const;

export const WEB_RESEARCH_DEPTH_OPTIONS = [
  { label: 'basic', value: 'basic' },
  { label: 'advanced', value: 'advanced' },
  { label: 'fast', value: 'fast' },
  { label: 'ultra-fast', value: 'ultra-fast' },
] as const;

export function webResearchFromSettings(row?: WebResearchSettings | null): WebResearchFormState {
  if (!row) return { ...DEFAULT_WEB_RESEARCH_FORM };
  return {
    provider: row.provider || DEFAULT_WEB_RESEARCH_FORM.provider,
    api_key: '',
    max_results: (row.maxResults ?? 0) > 0 ? row.maxResults! : DEFAULT_WEB_RESEARCH_FORM.max_results,
    fetch_top: (row.fetchTop ?? 0) > 0 ? row.fetchTop! : DEFAULT_WEB_RESEARCH_FORM.fetch_top,
    search_depth: row.searchDepth || DEFAULT_WEB_RESEARCH_FORM.search_depth,
    timeout_sec: (row.timeoutSec ?? 0) > 0 ? row.timeoutSec! : DEFAULT_WEB_RESEARCH_FORM.timeout_sec,
    http_proxy: row.httpProxy || '',
  };
}

export type WebResearchPatch = {
  provider: string;
  maxResults: number;
  fetchTop: number;
  searchDepth: string;
  timeoutSec: number;
  httpProxy: string;
  apiKey?: string;
};

export function webResearchToPatch(form: WebResearchFormState): WebResearchPatch {
  const patch: WebResearchPatch = {
    provider: form.provider,
    maxResults: form.max_results,
    fetchTop: form.fetch_top,
    searchDepth: form.search_depth,
    timeoutSec: form.timeout_sec,
    httpProxy: form.http_proxy.trim(),
  };
  const key = form.api_key.trim();
  if (key) patch.apiKey = key;
  return patch;
}
