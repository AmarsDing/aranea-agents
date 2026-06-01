import { createAdminServiceClient } from "./kratos/admin/v1/index";
import { createAgentCategoryServiceClient } from "./kratos/agent_category/v1/index";
import { createAgentServiceClient } from "./kratos/agent/v1/index";
import { createArtifactServiceClient } from "./kratos/artifact/v1/index";
import { createA2AServiceClient } from "./kratos/a2a/v1/index";
import { createAIRefineServiceClient } from "./kratos/ai_refine/v1/index";
import { createEcosystemServiceClient } from "./kratos/ecosystem/v1/index";
import { createGatewayServiceClient } from "./kratos/gateway/v1/index";
import { createAvatarServiceClient } from "./kratos/avatar/v1/index";
import { createChannelServiceClient } from "./kratos/channel/v1/index";
import { createCronServiceClient } from "./kratos/cron/v1/index";
import { createEvaluationServiceClient } from "./kratos/evaluation/v1/index";
import { createEventServiceClient } from "./kratos/event/v1/index";
import { createHookServiceClient } from "./kratos/hook/v1/index";
import { createKnowledgeServiceClient } from "./kratos/knowledge/v1/index";
import { createLlmProviderModelServiceClient } from "./kratos/llm_provider_model/v1/index";
import { createMCPServerServiceClient } from "./kratos/mcp_server/v1/index";
import { createPluginServiceClient } from "./kratos/plugin/v1/index";
import { createSessionServiceClient } from "./kratos/session/v1/index";
import { createSkillServiceClient } from "./kratos/skill/v1/index";
import { createSystemSettingServiceClient } from "./kratos/system_setting/v1/index";
import { createTeamServiceClient } from "./kratos/team/v1/index";
import { createToolServiceClient } from "./kratos/tool/v1/index";
import { createUsageServiceClient } from "./kratos/usage/v1/index";
import { createModelCatalogServiceClient } from "./kratos/model_catalog/v1/index";
import { createMonitorServiceClient } from "./kratos/monitor/v1/index";
import { createMemoryServiceClient } from "./kratos/memory/v1/index";
import { createGraphServiceClient } from "./kratos/graph/v1/index";
import { createChatServiceClient } from "./kratos/chat/v1/index";
import { requestHandler } from "./axiosHandler";

// 每个功能模块的 service（proto 生成客户端 + requestHandler → kratosApi）。
export function createAdminService() {
  return createAdminServiceClient(requestHandler);
}

export function createAvatarService() {
  return createAvatarServiceClient(requestHandler);
}

export function createChannelService() {
  return createChannelServiceClient(requestHandler);
}

export function createAgentCategoryService() {
  return createAgentCategoryServiceClient(requestHandler);
}

export function createAgentService() {
  return createAgentServiceClient(requestHandler);
}

export function createLlmProviderModelService() {
  return createLlmProviderModelServiceClient(requestHandler);
}

export function createCronService() {
  return createCronServiceClient(requestHandler);
}

export function createHookService() {
  return createHookServiceClient(requestHandler);
}

export function createMCPServerService() {
  return createMCPServerServiceClient(requestHandler);
}

export function createPluginService() {
  return createPluginServiceClient(requestHandler);
}

export function createSessionService() {
  return createSessionServiceClient(requestHandler);
}

export function createSkillService() {
  return createSkillServiceClient(requestHandler);
}

export function createSystemSettingService() {
  return createSystemSettingServiceClient(requestHandler);
}

export function createModelCatalogService() {
  return createModelCatalogServiceClient(requestHandler);
}

export function createTeamService() {
  return createTeamServiceClient(requestHandler);
}

export function createToolService() {
  return createToolServiceClient(requestHandler);
}

export function createUsageService() {
  return createUsageServiceClient(requestHandler);
}

export function createMonitorService() {
  return createMonitorServiceClient(requestHandler);
}

export function createMemoryService() {
  return createMemoryServiceClient(requestHandler);
}

export function createGraphService() {
  return createGraphServiceClient(requestHandler);
}

export function createChatService() {
  return createChatServiceClient(requestHandler);
}

export function createArtifactService() {
  return createArtifactServiceClient(requestHandler);
}

export function createKnowledgeService() {
  return createKnowledgeServiceClient(requestHandler);
}

export function createEvaluationService() {
  return createEvaluationServiceClient(requestHandler);
}

export function createEventService() {
  return createEventServiceClient(requestHandler);
}

export function createA2AService() {
  return createA2AServiceClient(requestHandler);
}

export function createAIRefineService() {
  return createAIRefineServiceClient(requestHandler);
}

export function createEcosystemService() {
  return createEcosystemServiceClient(requestHandler);
}

export function createGatewayService() {
  return createGatewayServiceClient(requestHandler);
}

export { kratosApi, requestHandler, syncHttpClients } from "./axiosHandler";

export function createSpiritService() {
  const basePath = "/v1/spirit";
  return {
    listTeams(spiritSessionId: string) {
      return kratosApi.get(`${basePath}/${encodeURIComponent(spiritSessionId)}/teams`);
    },
    getTeamDetail(teamId: string) {
      return kratosApi.get(`${basePath}/teams/${encodeURIComponent(teamId)}`);
    },
  };
}
