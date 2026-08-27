import { createSystemSettingService } from '../../services/index';
import { kratosApi } from '../../services/axiosHandler';
import type { SystemSettings } from '../../services/kratos/system_setting/v1/index';
import type {
  UpdateSystemSettingsInput,
  TestWebResearchInput,
  TestWebResearchResult,
  EcosystemLoadResponse,
  EcosystemUnloadResponse,
  EcosystemLoadedStatus,
  DiagnosticsReport,
} from './types';

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
    speech,
    refineLLM,
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
    speechAsrDriver: speech?.asrDriver,
    speechAsrEndpoint: speech?.asrEndpoint,
    speechAsrResourceId: speech?.asrResourceId,
    speechAsrLanguage: speech?.asrLanguage,
    speechAsrAppKey: speech?.asrAppKey,
    speechAsrAccessKey: speech?.asrAccessKey,
    speechTtsDriver: speech?.ttsDriver,
    speechTtsEndpoint: speech?.ttsEndpoint,
    speechTtsResourceId: speech?.ttsResourceId,
    speechTtsVoice: speech?.ttsVoice,
    speechTtsSpeedRatio: speech?.ttsSpeedRatio,
    speechTtsAppKey: speech?.ttsAppKey,
    speechTtsAccessKey: speech?.ttsAccessKey,
    // Tri-state: undefined → key omitted from JSON → proto3 optional unset →
    // backend keeps stored value (env fallback preserved).
    speechArchiveUserAudio: speech?.archiveUserAudio,
    // Refine LLM（PGO-3-PROTO-02）：apiKey 空 = 保留存值。
    refineLlmProvider: refineLLM?.provider?.trim() ?? '',
    refineLlmModel: refineLLM?.model?.trim() ?? '',
    refineLlmBaseUrl: refineLLM?.baseUrl?.trim() ?? '',
    refineLlmApiKey: refineLLM?.apiKey?.trim() ?? '',
  });
}

// Ecosystem preset APIs
// TODO(debt): migrate to protobuf service client once proto definitions are added

export async function loadEcosystemPreset(industries?: string[], force?: boolean): Promise<EcosystemLoadResponse> {
  const body: Record<string, unknown> = {};
  if (industries && industries.length > 0) body.industries = industries;
  if (force) body.force = true;
  const { data } = await kratosApi.post<EcosystemLoadResponse>('/api/v1/admin/ecosystem/preset/load', body);
  return data;
}

export async function unloadEcosystemPreset(industries: string[]): Promise<EcosystemUnloadResponse> {
  const { data } = await kratosApi.post<EcosystemUnloadResponse>('/api/v1/admin/ecosystem/preset/unload', {
    industries,
  });
  return data;
}

export async function getEcosystemPresetStatus(): Promise<EcosystemLoadedStatus> {
  const { data } = await kratosApi.get<EcosystemLoadedStatus>('/api/v1/admin/ecosystem/preset/status');
  return data;
}

// Runtime diagnostics (79-runtime-governance R8 doctor)

export async function getDiagnostics(): Promise<DiagnosticsReport> {
  // skipErrorNotify——面板自带错误横幅内联呈现，避免全局 toast 重复告警。
  const { data } = await kratosApi.get<DiagnosticsReport>('/api/v1/admin/diagnostics', {
    skipErrorNotify: true,
  });
  return data;
}
