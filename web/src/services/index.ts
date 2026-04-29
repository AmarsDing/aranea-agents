import { createAdminServiceClient } from "./kratos/admin/v1/index";
import { createAgentCategoryServiceClient } from "./kratos/agent_category/v1/index";
import { createAgentServiceClient } from "./kratos/agent/v1/index";
import { createAvatarServiceClient } from "./kratos/avatar/v1/index";
import { createHookServiceClient } from "./kratos/hook/v1/index";
import { createLlmProviderModelServiceClient } from "./kratos/llm_provider_model/v1/index";
import { createMCPServerServiceClient } from "./kratos/mcp_server/v1/index";
import { requestHandler } from "./axiosHandler";

// 每个功能模块的 service（proto 生成客户端 + requestHandler → kratosApi）

export function createAdminService() {
  return createAdminServiceClient(requestHandler);
}

export function createAvatarService() {
  return createAvatarServiceClient(requestHandler);
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

export function createHookService() {
  return createHookServiceClient(requestHandler);
}

export function createMCPServerService() {
  return createMCPServerServiceClient(requestHandler);
}

/** Kratos HTTP 传输层（与 proto 生成路径 `v1/...` 配套）。遗留 `/api/v1/*` 见 {@link ./clientLegacy} / {@link ../api/client} */
export { kratosApi, legacyRestApi, requestHandler, syncHttpClients } from "./axiosHandler";
