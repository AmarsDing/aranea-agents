import { createSystemSettingService } from '../../services/index';
import type { SystemSettings } from '../../services/kratos/system_setting/v1/index';
import type { UpdateSystemSettingsInput, TestWebResearchInput, TestWebResearchResult } from './types';

const api = createSystemSettingService();

export async function getSystemSettings(): Promise<SystemSettings> {
  return api.GetSystemSettings({});
}

export async function testWebResearch(input: TestWebResearchInput): Promise<TestWebResearchResult> {
  const res = await api.TestWebResearch({
    provider: input.provider ?? 'tavily',
    apiKey: input.apiKey,
    maxResults: input.maxResults ?? 8,
    fetchTop: input.fetchTop ?? 5,
    searchDepth: input.searchDepth ?? 'basic',
    timeoutSec: input.timeoutSec ?? 15,
    httpProxy: input.httpProxy ?? '',
  });
  return {
    ok: res.ok,
    message: res.message,
    provider: res.provider,
    resultCount: res.resultCount,
    latencyMs: res.latencyMs,
  };
}

export async function updateSystemSettings(input: UpdateSystemSettingsInput): Promise<SystemSettings> {
  const {
    rootDirectory,
    workDirectory,
    globalMonthlyMicroUsd = 0,
    a2aPublicBaseUrl = '',
    mcpAllowAdhocHttp = false,
    knowledgeEmbed,
    evalLLM,
    webResearch,
  } = input;
  return api.UpdateSystemSettings({
    rootDirectory,
    workDirectory,
    globalMonthlyMicroUsd,
    a2aPublicBaseUrl,
    mcpAllowAdhocHttp,
    knowledgeEmbedProvider: knowledgeEmbed?.provider,
    knowledgeEmbedBaseUrl: knowledgeEmbed?.baseUrl,
    knowledgeEmbedModel: knowledgeEmbed?.model,
    knowledgeEmbedDim: knowledgeEmbed?.dim,
    knowledgeEmbedApiKey: knowledgeEmbed?.apiKey,
    evalSimProvider: evalLLM?.simProvider?.trim() ?? '',
    evalSimModel: evalLLM?.simModel?.trim() ?? '',
    evalJudgeProvider: evalLLM?.judgeProvider?.trim() ?? '',
    evalJudgeModel: evalLLM?.judgeModel?.trim() ?? '',
    webResearchProvider: webResearch?.provider ?? 'tavily',
    webResearchApiKey: webResearch?.apiKey,
    webResearchMaxResults: webResearch?.maxResults ?? 8,
    webResearchFetchTop: webResearch?.fetchTop ?? 5,
    webResearchSearchDepth: webResearch?.searchDepth ?? 'basic',
    webResearchTimeoutSec: webResearch?.timeoutSec ?? 15,
    webResearchHttpProxy: webResearch?.httpProxy ?? '',
  });
}
