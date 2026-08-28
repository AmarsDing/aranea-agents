import { createAdminServiceClient } from './kratos/admin/v1/index';
import { createIndustryTaxonomyServiceClient } from './kratos/industry_taxonomy/v1/index';
import { createTaxonomyServiceClient } from './kratos/taxonomy/v1/index';
import { createOrganizationServiceClient } from './kratos/organization/v1/index';
import { createAgentServiceClient } from './kratos/agent/v1/index';
import { createArtifactServiceClient } from './kratos/artifact/v1/index';
import { createA2AServiceClient, createFederationServiceClient } from './kratos/a2a/v1/index';
import { createAIRefineServiceClient } from './kratos/ai_refine/v1/index';
import { createEcosystemServiceClient } from './kratos/ecosystem/v1/index';
import { createGatewayServiceClient } from './kratos/gateway/v1/index';
import { createAvatarServiceClient } from './kratos/avatar/v1/index';
import { createChannelServiceClient } from './kratos/channel/v1/index';
import { createCronServiceClient } from './kratos/cron/v1/index';
import { createEvaluationServiceClient } from './kratos/evaluation/v1/index';
import { createHookServiceClient } from './kratos/hook/v1/index';
import { createKnowledgeServiceClient } from './kratos/knowledge/v1/index';
import { createLearningLoopServiceClient } from './kratos/learning_loop/v1/index';
import { createLlmProviderModelServiceClient } from './kratos/llm_provider_model/v1/index';
import { createMCPServerServiceClient } from './kratos/mcp_server/v1/index';
import { createPluginServiceClient } from './kratos/plugin/v1/index';
import { createSessionServiceClient } from './kratos/session/v1/index';
import { createSkillServiceClient } from './kratos/skill/v1/index';
import { createSystemSettingServiceClient } from './kratos/system_setting/v1/index';
import { createTeamServiceClient } from './kratos/team/v1/index';
import { createToolServiceClient } from './kratos/tool/v1/index';
import { createUsageServiceClient } from './kratos/usage/v1/index';
import { createModelCatalogServiceClient } from './kratos/model_catalog/v1/index';
import { createMonitorServiceClient } from './kratos/monitor/v1/index';
import { createMemoryServiceClient } from './kratos/memory/v1/index';
import { createGraphServiceClient } from './kratos/graph/v1/index';
import { createSkillIntelligenceServiceClient } from './kratos/skill_intelligence/v1/index';
import { createSkillEvolutionSuggestionServiceClient } from './kratos/skill_evolution_suggestion/v1/index';
import { createSelfImprovementServiceClient } from './kratos/self_improvement/v1/index';
import { createPackServiceClient } from './kratos/pack/v1/index';
import { createRuntimeProfileServiceClient } from './kratos/runtime_profile/v1/index';
import { createChatServiceClient } from './kratos/chat/v1/index';
import { createComputerUseServiceClient } from './kratos/computeruse/v1/index';
import { requestHandler, kratosApi } from './axiosHandler';

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

export function createIndustryTaxonomyService() {
  return createIndustryTaxonomyServiceClient(requestHandler);
}

export function createTaxonomyService() {
  return createTaxonomyServiceClient(requestHandler);
}

export function createOrganizationService() {
  return createOrganizationServiceClient(requestHandler);
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

export function createComputerUseService() {
  return createComputerUseServiceClient(requestHandler);
}

export function createArtifactService() {
  return createArtifactServiceClient(requestHandler);
}

export function createKnowledgeService() {
  return createKnowledgeServiceClient(requestHandler);
}

export function createLearningLoopService() {
  return createLearningLoopServiceClient(requestHandler);
}

export function createEvaluationService() {
  return createEvaluationServiceClient(requestHandler);
}

export function createA2AService() {
  return createA2AServiceClient(requestHandler);
}

export function createFederationService() {
  return createFederationServiceClient(requestHandler);
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

export function createSkillIntelligenceService() {
  return createSkillIntelligenceServiceClient(requestHandler);
}

export function createSkillEvolutionSuggestionService() {
  return createSkillEvolutionSuggestionServiceClient(requestHandler);
}

export function createSelfImprovementService() {
  return createSelfImprovementServiceClient(requestHandler);
}

export function createWebhookService() {
  return createGatewayServiceClient(requestHandler);
}

export function createPackService() {
  return createPackServiceClient(requestHandler);
}

export function createRuntimeProfileService() {
  return createRuntimeProfileServiceClient(requestHandler);
}

export { kratosApi, requestHandler, syncHttpClients } from './axiosHandler';

// TODO: Replace createSpiritService with proto-generated client from spirit/v1
// once the spirit proto definition is available.
// Routes must match api/kratos/team/v1/team.proto HTTP annotations.
export function createSpiritService() {
  return {
    listTeams(spiritSessionId: string) {
      return kratosApi.get(`/v1/spirit/${encodeURIComponent(spiritSessionId)}/teams`);
    },
    getTeamDetail(teamId: string) {
      return kratosApi.get(`/v1/teams/${encodeURIComponent(teamId)}`);
    },
    listTeamRuns(teamId: string) {
      return kratosApi.get(`/v1/team-runs?team_id=${encodeURIComponent(teamId)}&limit=1`);
    },
    cancelTeamRun(teamRunId: string) {
      return kratosApi.post(`/v1/team-runs/${encodeURIComponent(teamRunId)}/cancel`);
    },
    resumeTeamRun(teamRunId: string) {
      return kratosApi.post(`/v1/team-runs/${encodeURIComponent(teamRunId)}/resume`);
    },
    // pauseTeamRun transitions a running team run to paused state (B.5.3).
    // MVP: cancels in-flight member step + marks run as paused.
    pauseTeamRun(teamRunId: string) {
      return kratosApi.post(`/v1/team-runs/${encodeURIComponent(teamRunId)}/pause`);
    },
    // unpauseTeamRun transitions a paused team run back to running (B.5.3).
    // MVP: flips status marker; user must inject message to resume execution.
    unpauseTeamRun(teamRunId: string) {
      return kratosApi.post(`/v1/team-runs/${encodeURIComponent(teamRunId)}/unpause`);
    },
    // injectTeamMessage enqueues a user message to the team's active/paused
    // run, processed at the next step boundary (B.5.3).
    injectTeamMessage(teamId: string, message: string) {
      return kratosApi.post(`/v1/teams/${encodeURIComponent(teamId)}/inject`, { message });
    },
    archiveTeam(teamId: string) {
      return kratosApi.post(`/v1/teams/${encodeURIComponent(teamId)}/archive`);
    },
    retryTeam(teamId: string) {
      return kratosApi.post(`/v1/teams/${encodeURIComponent(teamId)}/retry`);
    },
    // Agent cancel/retry — reuses chat RPCs (B.5.2 childSessionId scheme).
    // stopGeneration cancels an in-flight sub-agent run (StopGeneration RPC).
    stopGeneration(sessionId: string) {
      return kratosApi.post('/v1/chat/stop', { session_id: sessionId });
    },
    // retrySession re-triggers the last failed/interrupted turn for a session
    // by re-enqueuing the latest user message (RetrySession RPC).
    retrySession(sessionId: string) {
      return kratosApi.post(`/v1/chat/sessions/${encodeURIComponent(sessionId)}/retry`);
    },
    // pauseSession transitions a running chat session to paused state (B.5.3).
    // Used by AgentCard pause button; MVP cancels in-flight turn.
    pauseSession(sessionId: string) {
      return kratosApi.post(`/v1/chat/sessions/${encodeURIComponent(sessionId)}/pause`);
    },
    // resumeSession transitions a paused chat session back to running (B.5.3).
    // Used by AgentCard resume button; MVP flips status marker.
    resumeSession(sessionId: string) {
      return kratosApi.post(`/v1/chat/sessions/${encodeURIComponent(sessionId)}/resume`);
    },
    // enqueueUserMessage enqueues a user message to a session's pending queue
    // (POST /v1/chat/enqueue). Used by MemberSessionPanel input bar to inject
    // messages to individual sub-agent sessions.
    enqueueUserMessage(sessionId: string, content: string, kind?: 'steer' | 'followup' | 'inject') {
      return kratosApi.post('/v1/chat/enqueue', { session_id: sessionId, content, kind }, { skipErrorNotify: true });
    },
  };
}
