import { createSystemSettingService } from "../../services/index";
import type { SystemSettings } from "../../services/kratos/system_setting/v1/index";
import type { KnowledgeEmbedPatch } from "./knowledge-embed";
import type { EvalLLMForm } from "./eval-llm";

const api = createSystemSettingService();

export async function getSystemSettings(): Promise<SystemSettings> {
  return api.GetSystemSettings({});
}

export type UpdateSystemSettingsInput = {
  rootDirectory: string;
  workDirectory: string;
  globalMonthlyMicroUsd?: number;
  a2aPublicBaseUrl?: string;
  mcpAllowAdhocHttp?: boolean;
  knowledgeEmbed?: KnowledgeEmbedPatch;
  evalLLM?: EvalLLMForm;
};

export async function updateSystemSettings(input: UpdateSystemSettingsInput): Promise<SystemSettings> {
  const { rootDirectory, workDirectory, globalMonthlyMicroUsd = 0, a2aPublicBaseUrl = "", mcpAllowAdhocHttp = false, knowledgeEmbed, evalLLM } =
    input;
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
    evalSimProvider: evalLLM?.simProvider ?? "",
    evalSimModel: evalLLM?.simModel ?? "",
    evalJudgeProvider: evalLLM?.judgeProvider ?? "",
    evalJudgeModel: evalLLM?.judgeModel ?? ""
  });
}
