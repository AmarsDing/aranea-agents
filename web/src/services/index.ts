import { createAdminServiceClient } from "./kratos/admin/v1/index";
import { createAgentCategoryServiceClient } from "./kratos/agent_category/v1/index";
import { createAgentServiceClient } from "./kratos/agent/v1/index";
import { createAvatarServiceClient } from "./kratos/avatar/v1/index";
import { createChannelServiceClient } from "./kratos/channel/v1/index";
import { createCronServiceClient } from "./kratos/cron/v1/index";
import { createHookServiceClient } from "./kratos/hook/v1/index";
import { createLlmProviderModelServiceClient } from "./kratos/llm_provider_model/v1/index";
import { createMCPServerServiceClient } from "./kratos/mcp_server/v1/index";
import { createPluginServiceClient } from "./kratos/plugin/v1/index";
import { createSessionServiceClient } from "./kratos/session/v1/index";
import { createSkillServiceClient } from "./kratos/skill/v1/index";
import { createTeamServiceClient } from "./kratos/team/v1/index";
import { createToolServiceClient } from "./kratos/tool/v1/index";
import { createUsageServiceClient } from "./kratos/usage/v1/index";
import { requestHandler } from "./axiosHandler";

// 每个功能模块的 service（proto 生成客户端 + requestHandler → kratosApi）

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

export function createTeamService() {
  return createTeamServiceClient(requestHandler);
}

export function createToolService() {
  return createToolServiceClient(requestHandler);
}

export function createUsageService() {
  return createUsageServiceClient(requestHandler);
}

export { kratosApi, legacyRestApi, requestHandler, syncHttpClients } from "./axiosHandler";
