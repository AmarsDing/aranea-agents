import { createRuntimeProfileService } from '../../services';
import type {
  RuntimeProfile,
  CreateRuntimeProfileRequest,
  UpdateRuntimeProfileRequest,
  PromptConfig,
  ToolPolicy,
  SkillPolicy,
  KnowledgePolicy,
  WorkspacePolicy,
  CredentialPolicy,
  IsolationPolicy,
} from '../../services/kratos/runtime_profile/v1/index';

export type {
  RuntimeProfile,
  PromptConfig,
  ToolPolicy,
  SkillPolicy,
  KnowledgePolicy,
  WorkspacePolicy,
  CredentialPolicy,
  IsolationPolicy,
};

function normalizeProfile(row: RuntimeProfile): RuntimeProfile {
  return {
    id: row.id ?? '',
    agentId: row.agentId ?? '',
    name: row.name ?? '',
    description: row.description ?? '',
    version: row.version ?? '',
    isActive: row.isActive ?? false,
    priority: row.priority ?? 0,
    promptConfig: row.promptConfig,
    toolPolicy: row.toolPolicy,
    skillPolicy: row.skillPolicy,
    knowledgePolicy: row.knowledgePolicy,
    workspacePolicy: row.workspacePolicy,
    credentialPolicy: row.credentialPolicy,
    isolationPolicy: row.isolationPolicy,
    extraModelConfig: row.extraModelConfig,
    createdAt: row.createdAt ?? '',
    updatedAt: row.updatedAt ?? '',
  };
}

export async function listRuntimeProfiles(agentId: string, activeOnly = false): Promise<RuntimeProfile[]> {
  const svc = createRuntimeProfileService();
  const res = await svc.ListRuntimeProfiles({ agentId, activeOnly });
  return (res.items ?? []).map(normalizeProfile);
}

export async function getRuntimeProfile(id: string): Promise<RuntimeProfile> {
  const svc = createRuntimeProfileService();
  return normalizeProfile(await svc.GetRuntimeProfile({ id }));
}

export async function createRuntimeProfile(req: CreateRuntimeProfileRequest): Promise<RuntimeProfile> {
  const svc = createRuntimeProfileService();
  return normalizeProfile(await svc.CreateRuntimeProfile(req));
}

export async function updateRuntimeProfile(req: UpdateRuntimeProfileRequest): Promise<RuntimeProfile> {
  const svc = createRuntimeProfileService();
  return normalizeProfile(await svc.UpdateRuntimeProfile(req));
}

export async function deleteRuntimeProfile(id: string): Promise<void> {
  const svc = createRuntimeProfileService();
  await svc.DeleteRuntimeProfile({ id });
}

export async function setRuntimeProfileActive(id: string, active: boolean): Promise<RuntimeProfile> {
  const svc = createRuntimeProfileService();
  return normalizeProfile(await svc.SetActive({ id, active }));
}
