import { createAgentCategoryService } from '../../services';
import type { AgentCategory, AgentCategoryTreeNode } from '../../services/kratos/agent_category/v1/index';

export type { AgentCategory, AgentCategoryTreeNode };

function mapCategory(raw: AgentCategory) {
  return {
    id: String(raw.id ?? ''),
    key: String(raw.key ?? ''),
    name: String(raw.name ?? ''),
    description: String(raw.description ?? ''),
    status: String(raw.status ?? ''),
    enabled: Boolean(raw.enabled),
    sort_order: Number(raw.sortOrder ?? 0),
    parent_id: String(raw.parentId ?? ''),
    level: String(raw.level ?? ''),
    workspace_id: String(raw.workspaceId ?? ''),
    owner_user_id: String(raw.ownerUserId ?? ''),
    is_system: Boolean(raw.isSystem),
    config_json: String(raw.configJson ?? ''),
    metadata_json: String(raw.metadataJson ?? ''),
    created_at: String(raw.createdAt ?? ''),
    updated_at: String(raw.updatedAt ?? ''),
  };
}

export type AgentCategoryRow = ReturnType<typeof mapCategory>;

export type AgentCategoryTreeNodeRow = {
  category: AgentCategoryRow;
  children: AgentCategoryTreeNodeRow[];
};

function mapTreeNode(raw: AgentCategoryTreeNode): AgentCategoryTreeNodeRow {
  return {
    category: mapCategory(raw.category!),
    children: (raw.children ?? []).map(mapTreeNode),
  };
}

export async function listAgentCategories(): Promise<AgentCategoryRow[]> {
  const svc = createAgentCategoryService();
  const res = await svc.ListAgentCategories({});
  return (res.items ?? []).map(mapCategory);
}

export async function listAgentCategoryTree(): Promise<AgentCategoryTreeNodeRow[]> {
  const svc = createAgentCategoryService();
  const res = await svc.ListAgentCategoryTree({});
  return (res.items ?? []).map(mapTreeNode);
}

export async function createAgentCategory(input: {
  key: string;
  name: string;
  description?: string;
  status?: string;
  enabled?: boolean;
  sort_order?: number;
  parent_id?: string;
  level?: string;
  workspace_id?: string;
  owner_user_id?: string;
  config_json?: string;
  metadata_json?: string;
}): Promise<AgentCategoryRow> {
  const svc = createAgentCategoryService();
  const res = await svc.CreateAgentCategory({
    key: input.key,
    name: input.name,
    description: input.description,
    status: input.status,
    enabled: input.enabled,
    sortOrder: input.sort_order,
    parentId: input.parent_id,
    level: input.level,
    workspaceId: input.workspace_id,
    ownerUserId: input.owner_user_id,
    configJson: input.config_json,
    metadataJson: input.metadata_json,
  });
  return mapCategory(res);
}

export async function getAgentCategory(id: string): Promise<AgentCategoryRow> {
  const svc = createAgentCategoryService();
  const res = await svc.GetAgentCategory({ id });
  return mapCategory(res);
}

export async function updateAgentCategory(
  id: string,
  patch: Partial<Omit<AgentCategoryRow, 'id' | 'created_at' | 'updated_at'>>,
): Promise<AgentCategoryRow> {
  const svc = createAgentCategoryService();
  const res = await svc.UpdateAgentCategory({
    id,
    category: {
      id: undefined,
      key: patch.key,
      name: patch.name,
      description: patch.description,
      status: patch.status,
      enabled: patch.enabled,
      sortOrder: patch.sort_order,
      parentId: patch.parent_id,
      level: patch.level,
      workspaceId: patch.workspace_id,
      ownerUserId: patch.owner_user_id,
      isSystem: undefined,
      configJson: patch.config_json,
      metadataJson: patch.metadata_json,
      createdAt: undefined,
      updatedAt: undefined,
      deletedAt: undefined,
    },
  });
  return mapCategory(res);
}

export async function deleteAgentCategory(id: string): Promise<void> {
  const svc = createAgentCategoryService();
  await svc.DeleteAgentCategory({ id });
}
