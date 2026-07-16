/**
 * 平台资源：**按 `PlatformResourceName` 分发**至 **`avatar` / `industry_taxonomy` / `llm_provider_model` / `hook` / `channel` / `mcp` / `cron` / `monitor` / `skill`** 等 Kratos 客户端。
 * **不再使用 `legacyRestApi`**。
 *
 * **`validateModel`** / **`inspectProviderModel`**：**`llm_provider_model/v1`**。
 */
import type { CreateProviderModelRequest, ProviderModel } from '../../services/kratos/llm_provider_model/v1/index';
import {
  createIndustryTaxonomyService,
  createTaxonomyService,
  createOrganizationService,
  createAvatarService,
  createChannelService,
  createCronService,
  createHookService,
  createLlmProviderModelService,
  createMonitorService,
  createSkillService,
} from '../../services';
import { listAvatarAssets } from '../avatar/api';
import { asRecord, pickBool, pickOptionalBoolDefaultTrue, pickI32, pickNum, pickStr } from '../../shared/wireJson';
import { createMcpServer, deleteMcpServer, listMcpServers, updateMcpServer } from '../mcp/api';
import { listSkills } from '../skills/api';
import type { Skill } from '../skills/types';
import { listModelUsageEvents } from '../usage/api';
import type { ModelTokenUsageEvent } from '../usage/types';

export type {
  PlatformResourceName,
  PlatformResource,
  PlatformResourceTreeNode,
  PlatformResourceInput,
  ValidateModelResult,
  InspectProviderModelInput,
  InspectProviderModelResult,
  RevealProviderCredentialsResult,
} from './types';

import type {
  PlatformResourceName,
  PlatformResource,
  PlatformResourceTreeNode,
  PlatformResourceInput,
  ValidateModelResult,
  InspectProviderModelInput,
  InspectProviderModelResult,
  RevealProviderCredentialsResult,
} from './types';

const llmModels = createLlmProviderModelService();

function unsupported(op: string, resource: PlatformResourceName): Error {
  return new Error(`${op} unsupported for resource "${resource}" via platform gateway`);
}

function industryTaxonomyWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    resource: 'taxonomy-nodes',
    key: pickStr(r, 'key', 'key'),
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    status: pickStr(r, 'status', 'status'),
    enabled: pickBool(r, 'enabled', 'enabled'),
    sort_order: pickI32(r, 'sort_order', 'sortOrder'),
    parent_id: pickStr(r, 'parent_id', 'parentId'),
    level: pickStr(r, 'level', 'level'),
    agent_id: '',
    provider: '',
    model: '',
    is_system: pickBool(r, 'is_system', 'isSystem'),
    config_json: pickStr(r, 'config_json', 'configJson') || '{}',
    metadata_json: pickStr(r, 'metadata_json', 'metadataJson') || '{}',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    deleted_at: pickStr(r, 'deleted_at', 'deletedAt'),
  };
}

function taxonomyWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    resource: 'taxonomy',
    key: pickStr(r, 'key', 'key'),
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    status: pickStr(r, 'status', 'status'),
    enabled: pickBool(r, 'enabled', 'enabled'),
    sort_order: pickI32(r, 'sort_order', 'sortOrder'),
    parent_id: pickStr(r, 'parent_id', 'parentId'),
    level: pickStr(r, 'level', 'level'),
    agent_id: '',
    provider: '',
    model: '',
    is_system: pickBool(r, 'is_system', 'isSystem'),
    config_json: pickStr(r, 'config_json', 'configJson') || '{}',
    metadata_json: pickStr(r, 'metadata_json', 'metadataJson') || '{}',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    deleted_at: pickStr(r, 'deleted_at', 'deletedAt'),
  };
}

function organizationWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    resource: 'organization',
    key: pickStr(r, 'orgKey', 'orgKey'),
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    status: pickStr(r, 'status', 'status'),
    enabled: pickBool(r, 'enabled', 'enabled'),
    sort_order: pickI32(r, 'sortOrder', 'sortOrder'),
    parent_id: pickStr(r, 'parentId', 'parentId'),
    level: pickStr(r, 'level', 'level'),
    agent_id: '',
    provider: '',
    model: '',
    is_system: pickBool(r, 'isSystem', 'isSystem'),
    config_json: pickStr(r, 'configJson', 'configJson') || '{}',
    metadata_json: pickStr(r, 'metadataJson', 'metadataJson') || '{}',
    dept_lead_agent_id: pickStr(r, 'deptLeadAgentId', 'deptLeadAgentId'),
    dept_lead_config_json: pickStr(r, 'deptLeadConfigJson', 'deptLeadConfigJson') || '{}',
    created_at: pickStr(r, 'createdAt', 'createdAt'),
    updated_at: pickStr(r, 'updatedAt', 'updatedAt'),
    deleted_at: pickStr(r, 'deletedAt', 'deletedAt'),
  };
}

function mapIndustryTaxonomyTreeNode(raw: unknown): PlatformResourceTreeNode {
  const o = asRecord(raw);
  const cat = asRecord(o.industryTaxonomy ?? o.IndustryTaxonomy);
  const base = industryTaxonomyWireToPlatform(cat);
  const childrenRaw = o.children ?? o.Children;
  const children = Array.isArray(childrenRaw) ? childrenRaw.map(mapIndustryTaxonomyTreeNode) : [];
  return { ...base, children };
}

function mapTaxonomyTreeNode(raw: unknown): PlatformResourceTreeNode {
  const o = asRecord(raw);
  const node = asRecord(o.node ?? o.Node);
  const base = taxonomyWireToPlatform(node);
  const childrenRaw = o.children ?? o.Children;
  const children = Array.isArray(childrenRaw) ? childrenRaw.map(mapTaxonomyTreeNode) : [];
  return { ...base, children };
}

function mapOrganizationTreeNode(raw: unknown): PlatformResourceTreeNode {
  const o = asRecord(raw);
  const node = asRecord(o.node ?? o.Node);
  const base = organizationWireToPlatform(node);
  const childrenRaw = o.children ?? o.Children;
  const children = Array.isArray(childrenRaw) ? childrenRaw.map(mapOrganizationTreeNode) : [];
  return { ...base, children };
}

function llmProviderWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  const caps = asRecord(r.capabilities ?? r.Capabilities);
  return {
    id: pickStr(r, 'id', 'id'),
    resource: 'llm-provider-models',
    key: pickStr(r, 'key', 'key'),
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    status: pickStr(r, 'status', 'status'),
    enabled: pickOptionalBoolDefaultTrue(r, 'enabled', 'enabled'),
    sort_order: pickI32(r, 'sort_order', 'sortOrder'),
    parent_id: '',
    level: '',
    agent_id: '',
    provider: pickStr(r, 'provider', 'provider'),
    model: pickStr(r, 'model', 'model'),
    is_system: false,
    config_json: pickStr(r, 'config_json', 'configJson') || '{}',
    metadata_json: pickStr(r, 'metadata_json', 'metadataJson') || '{}',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    capabilities: {
      text: pickBool(caps, 'text', 'text'),
      vision: pickBool(caps, 'vision', 'vision'),
      image: pickBool(caps, 'vision', 'vision'),
      audio: pickBool(caps, 'audio', 'audio'),
      file: pickBool(caps, 'file', 'file'),
      tool_call: pickBool(caps, 'tool_call', 'toolCall'),
      cache: pickBool(caps, 'cache', 'cache'),
      thinking: pickBool(caps, 'thinking', 'thinking'),
      text_only: pickBool(caps, 'text_only', 'textOnly'),
    },
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    deleted_at: pickStr(r, 'deleted_at', 'deletedAt'),
  };
}

function hookWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    resource: 'hooks',
    key: pickStr(r, 'key', 'key'),
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    status: pickStr(r, 'status', 'status'),
    enabled: pickBool(r, 'enabled', 'enabled'),
    sort_order: pickI32(r, 'sort_order', 'sortOrder'),
    parent_id: '',
    level: '',
    agent_id: '',
    provider: '',
    model: '',
    is_system: false,
    config_json: pickStr(r, 'config_json', 'configJson') || '{}',
    metadata_json: pickStr(r, 'metadata_json', 'metadataJson') || '{}',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    deleted_at: pickStr(r, 'deleted_at', 'deletedAt'),
  };
}

function channelWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    resource: 'channels',
    key: pickStr(r, 'key', 'key'),
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    status: pickStr(r, 'status', 'status'),
    enabled: pickBool(r, 'enabled', 'enabled'),
    sort_order: pickI32(r, 'sort_order', 'sortOrder'),
    parent_id: pickStr(r, 'parent_id', 'parentId'),
    level: pickStr(r, 'level', 'level'),
    agent_id: pickStr(r, 'agent_id', 'agentId'),
    provider: pickStr(r, 'provider', 'provider'),
    model: pickStr(r, 'model', 'model'),
    is_system: false,
    config_json: pickStr(r, 'config_json', 'configJson') || '{}',
    metadata_json: pickStr(r, 'metadata_json', 'metadataJson') || '{}',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    deleted_at: pickStr(r, 'deleted_at', 'deletedAt'),
  };
}

function cronWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    resource: 'cron-tasks',
    key: pickStr(r, 'task_key', 'taskKey'),
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    status: pickStr(r, 'status', 'status'),
    enabled: pickBool(r, 'enabled', 'enabled'),
    sort_order: pickI32(r, 'sort_order', 'sortOrder'),
    parent_id: '',
    level: '',
    agent_id: pickStr(r, 'agent_id', 'agentId'),
    provider: '',
    model: '',
    is_system: false,
    config_json: pickStr(r, 'config_json', 'configJson') || '{}',
    metadata_json: pickStr(r, 'metadata_json', 'metadataJson') || '{}',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    deleted_at: pickStr(r, 'deleted_at', 'deletedAt'),
  };
}

function monitorEventWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  const kind = pickStr(r, 'resource', 'resource');
  const resName: PlatformResourceName = kind === 'monitor-traces' ? 'monitor-traces' : 'monitor-events';
  return {
    id: pickStr(r, 'id', 'id'),
    resource: resName,
    key: pickStr(r, 'key', 'key'),
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    status: pickStr(r, 'status', 'status'),
    enabled: pickBool(r, 'enabled', 'enabled'),
    sort_order: pickI32(r, 'sort_order', 'sortOrder'),
    parent_id: pickStr(r, 'parent_id', 'parentId'),
    level: pickStr(r, 'level', 'level'),
    agent_id: pickStr(r, 'agent_id', 'agentId'),
    provider: pickStr(r, 'provider', 'provider'),
    model: pickStr(r, 'model', 'model'),
    is_system: false,
    config_json: pickStr(r, 'config_json', 'configJson') || '{}',
    metadata_json: pickStr(r, 'metadata_json', 'metadataJson') || '{}',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    deleted_at: pickStr(r, 'deleted_at', 'deletedAt'),
  };
}

function usageEventToPlatformTrace(e: ModelTokenUsageEvent): PlatformResource {
  const name = [e.model_display_name || e.model_api_id, e.agent_key].filter(Boolean).join(' · ') || e.id;
  return {
    id: e.id,
    resource: 'monitor-traces',
    key: e.message_id || e.id,
    name,
    description: e.error_message || e.status || '',
    status: e.status,
    enabled: true,
    sort_order: 0,
    parent_id: '',
    level: '',
    agent_id: e.agent_id,
    provider: e.provider_code,
    model: e.model_api_id,
    is_system: false,
    config_json: JSON.stringify({
      session_id: e.session_id,
      latency_ms: e.latency_ms,
      total_tokens: e.total_tokens,
      total_cost_micro_usd: e.total_cost_micro_usd,
    }),
    metadata_json: '{}',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: e.occurred_at,
    updated_at: e.occurred_at,
    deleted_at: '',
  };
}

function skillToPlatform(s: Skill): PlatformResource {
  return {
    id: s.id,
    resource: 'skills',
    key: s.slug,
    name: s.name,
    description: s.description,
    status: s.status,
    enabled: s.enabled,
    sort_order: 0,
    parent_id: '',
    level: '',
    agent_id: s.last_agent_id ?? '',
    provider: '',
    model: '',
    is_system: false,
    config_json: JSON.stringify({
      tags: s.tags,
      extends_skill_id: s.extends_skill_id,
      invoke_count: s.invoke_count,
      success_count: s.success_count,
      failure_count: s.failure_count,
    }),
    metadata_json: JSON.stringify(s.current_version ?? {}),
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: s.created_at,
    updated_at: s.updated_at,
    deleted_at: '',
  };
}

function avatarAssetToPlatform(row: {
  id: string;
  key: string;
  name: string;
  description: string;
  sort_order: number;
  created_at: string;
  mime_type?: string;
  workspace_id?: string;
  owner_user_id?: string;
  source?: string;
  is_system?: boolean;
  file_size_bytes?: number;
  width_px?: number;
  height_px?: number;
}): PlatformResource {
  return {
    id: row.id,
    resource: 'avatar-assets',
    key: row.key,
    name: row.name,
    description: row.description,
    status: 'active',
    enabled: true,
    sort_order: row.sort_order,
    parent_id: '',
    level: '',
    agent_id: '',
    provider: '',
    model: '',
    is_system: row.is_system ?? false,
    config_json: JSON.stringify({
      mime_type: row.mime_type,
      workspace_id: row.workspace_id,
      owner_user_id: row.owner_user_id,
      source: row.source,
      is_system: row.is_system,
      file_size_bytes: row.file_size_bytes,
      width_px: row.width_px,
      height_px: row.height_px,
    }),
    metadata_json: '{}',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: row.created_at,
    updated_at: row.created_at,
    deleted_at: '',
  };
}

function providerInputToCreateBody(payload: PlatformResourceInput): CreateProviderModelRequest {
  return {
    key: payload.key,
    name: payload.name,
    description: payload.description ?? '',
    status: payload.status ?? 'active',
    enabled: payload.enabled ?? true,
    sortOrder: payload.sort_order ?? 0,
    provider: payload.provider ?? '',
    model: payload.model ?? '',
    configJson: payload.config_json ?? '{}',
    metadataJson: payload.metadata_json ?? '{}',
    capabilities: payload.capabilities
      ? {
          text: payload.capabilities.text ?? false,
          vision: payload.capabilities.vision ?? false,
          audio: payload.capabilities.audio ?? false,
          file: payload.capabilities.file ?? false,
          toolCall: payload.capabilities.tool_call ?? false,
          cache: payload.capabilities.cache ?? false,
          thinking: payload.capabilities.thinking ?? false,
          textOnly: payload.capabilities.text_only ?? false,
        }
      : {
          text: false,
          vision: false,
          audio: false,
          file: false,
          toolCall: false,
          cache: false,
          thinking: false,
          textOnly: false,
        },
  };
}

function providerModelConfigJsonFromWire(base: ProviderModel): string | undefined {
  const r = base as unknown as Record<string, unknown>;
  const v = r.configJson ?? r.config_json;
  if (v === undefined || v === null) return undefined;
  const s = String(v);
  return s === '' ? undefined : s;
}

function providerModelMetadataJsonFromWire(base: ProviderModel): string | undefined {
  const r = base as unknown as Record<string, unknown>;
  const v = r.metadataJson ?? r.metadata_json;
  if (v === undefined || v === null) return undefined;
  const s = String(v);
  return s === '' ? undefined : s;
}

function mergeProviderModel(base: ProviderModel, patch: Partial<PlatformResourceInput>): ProviderModel {
  const baseConfig = providerModelConfigJsonFromWire(base);
  const baseMeta = providerModelMetadataJsonFromWire(base);
  return {
    id: base.id,
    key: patch.key ?? base.key,
    name: patch.name ?? base.name,
    description: patch.description ?? base.description,
    status: patch.status ?? base.status,
    enabled: patch.enabled ?? base.enabled,
    sortOrder: patch.sort_order ?? base.sortOrder,
    provider: patch.provider ?? base.provider,
    model: patch.model ?? base.model,
    configJson: patch.config_json ?? baseConfig ?? base.configJson ?? '{}',
    metadataJson: patch.metadata_json ?? baseMeta ?? base.metadataJson ?? '{}',
    capabilities: patch.capabilities
      ? {
          text: patch.capabilities.text ?? false,
          vision: patch.capabilities.vision ?? false,
          audio: patch.capabilities.audio ?? false,
          file: patch.capabilities.file ?? false,
          toolCall: patch.capabilities.tool_call ?? false,
          cache: patch.capabilities.cache ?? false,
          thinking: patch.capabilities.thinking ?? false,
          textOnly: patch.capabilities.text_only ?? false,
        }
      : (base.capabilities ?? {
          text: false,
          vision: false,
          audio: false,
          file: false,
          toolCall: false,
          cache: false,
          thinking: false,
          textOnly: false,
        }),
    pricingConfigured: patch.pricing_configured ?? base.pricingConfigured ?? false,
    createdAt: base.createdAt,
    updatedAt: base.updatedAt,
    deletedAt: base.deletedAt,
  };
}

export async function listPlatformResources(resource: PlatformResourceName): Promise<PlatformResource[]> {
  switch (resource) {
    case 'avatar-assets': {
      const rows = await listAvatarAssets();
      return rows.map(avatarAssetToPlatform);
    }
    case 'taxonomy-nodes': {
      const svc = createIndustryTaxonomyService();
      const res = await svc.ListIndustryTaxonomies({});
      return (res.items ?? []).map((row: unknown) => industryTaxonomyWireToPlatform(row));
    }
    case 'taxonomy': {
      const svc = createTaxonomyService();
      const res = await svc.ListTaxonomy({});
      return (res.items ?? []).map((row: unknown) => taxonomyWireToPlatform(row));
    }
    case 'organization': {
      const svc = createOrganizationService();
      const res = await svc.ListOrganization({});
      return (res.items ?? []).map((row: unknown) => organizationWireToPlatform(row));
    }
    case 'llm-provider-models': {
      const res = await llmModels.ListProviderModels({});
      return (res.items ?? []).map((row: unknown) => llmProviderWireToPlatform(row));
    }
    case 'hooks': {
      const svc = createHookService();
      const res = await svc.ListHooks({});
      return (res.items ?? []).map((row: unknown) => hookWireToPlatform(row));
    }
    case 'channels': {
      const svc = createChannelService();
      const res = await svc.ListChannels({});
      return (res.items ?? []).map((row: unknown) => channelWireToPlatform(row));
    }
    case 'mcp-servers': {
      return listMcpServers();
    }
    case 'skills': {
      const { items } = await listSkills({ page: 1, page_size: 500 });
      return items.map(skillToPlatform);
    }
    case 'cron-tasks': {
      const svc = createCronService();
      const res = await svc.ListCronTasks({});
      return (res.items ?? []).map((row: unknown) => cronWireToPlatform(row));
    }
    case 'monitor-events': {
      const svc = createMonitorService();
      const res = await svc.ListMonitorEvents({
        limit: undefined,
        offset: undefined,
        eventType: undefined,
        agentId: undefined,
        status: undefined,
      });
      return (res.items ?? []).map((row: unknown) => monitorEventWireToPlatform(row));
    }
    case 'monitor-traces': {
      const events = await listModelUsageEvents({ limit: 200 });
      return events.map(usageEventToPlatformTrace);
    }
    default:
      return [];
  }
}

export async function listPlatformResourceTree(
  resource: 'taxonomy-nodes' | 'taxonomy' | 'organization',
): Promise<PlatformResourceTreeNode[]> {
  if (resource === 'taxonomy') {
    const svc = createTaxonomyService();
    const res = await svc.ListTaxonomyTree({});
    const items = res.items ?? [];
    return items.map(mapTaxonomyTreeNode);
  }
  if (resource === 'organization') {
    const svc = createOrganizationService();
    const res = await svc.ListOrganizationTree({});
    const items = res.items ?? [];
    return items.map(mapOrganizationTreeNode);
  }
  const svc = createIndustryTaxonomyService();
  const res = await svc.ListIndustryTaxonomyTree({});
  const items = res.items ?? [];
  return items.map(mapIndustryTaxonomyTreeNode);
}

export async function createPlatformResource(
  resource: PlatformResourceName,
  payload: PlatformResourceInput,
): Promise<PlatformResource> {
  switch (resource) {
    case 'avatar-assets':
      throw unsupported('create', resource);
    case 'taxonomy-nodes': {
      const svc = createIndustryTaxonomyService();
      const row = await svc.CreateIndustryTaxonomy({
        key: payload.key,
        name: payload.name,
        description: payload.description,
        status: payload.status ?? 'active',
        enabled: payload.enabled ?? true,
        sortOrder: payload.sort_order ?? 0,
        parentId: payload.parent_id || undefined,
        level: payload.level || undefined,
        workspaceId: undefined,
        ownerUserId: undefined,
        configJson: payload.config_json ?? '{}',
        metadataJson: payload.metadata_json ?? '{}',
      });
      return industryTaxonomyWireToPlatform(row);
    }
    case 'taxonomy': {
      const svc = createTaxonomyService();
      const row = await svc.CreateTaxonomy({
        key: payload.key,
        name: payload.name,
        description: payload.description,
        status: payload.status ?? 'active',
        enabled: payload.enabled ?? true,
        sortOrder: payload.sort_order ?? 0,
        parentId: payload.parent_id || undefined,
        level: payload.level || undefined,
        workspaceId: undefined,
        ownerUserId: undefined,
        configJson: payload.config_json ?? '{}',
        metadataJson: payload.metadata_json ?? '{}',
      });
      return taxonomyWireToPlatform(row);
    }
    case 'organization': {
      const svc = createOrganizationService();
      const row = await svc.CreateOrganization({
        orgKey: payload.key,
        name: payload.name,
        description: payload.description,
        status: payload.status ?? 'active',
        enabled: payload.enabled ?? true,
        sortOrder: payload.sort_order ?? 0,
        parentId: payload.parent_id || undefined,
        level: payload.level || undefined,
        workspaceId: undefined,
        ownerUserId: undefined,
        configJson: payload.config_json ?? '{}',
        metadataJson: payload.metadata_json ?? '{}',
        deptLeadAgentId: undefined,
        deptLeadConfigJson: '{}',
      });
      return organizationWireToPlatform(row);
    }
    case 'llm-provider-models': {
      const row = await llmModels.CreateProviderModel(providerInputToCreateBody(payload));
      return llmProviderWireToPlatform(row);
    }
    case 'hooks': {
      const svc = createHookService();
      const row = await svc.CreateHook({
        key: payload.key,
        name: payload.name,
        description: payload.description,
        status: payload.status ?? 'active',
        enabled: payload.enabled ?? true,
        sortOrder: payload.sort_order ?? 0,
        configJson: payload.config_json ?? '{}',
        metadataJson: payload.metadata_json ?? '{}',
      });
      return hookWireToPlatform(row);
    }
    case 'channels': {
      const svc = createChannelService();
      const row = await svc.CreateChannel({
        key: payload.key,
        name: payload.name,
        description: payload.description ?? '',
        status: payload.status ?? 'active',
        enabled: payload.enabled ?? true,
        sortOrder: payload.sort_order ?? 0,
        configJson: payload.config_json ?? '{}',
        metadataJson: payload.metadata_json ?? '{}',
        credentials: [],
      });
      return channelWireToPlatform(row);
    }
    case 'mcp-servers': {
      return createMcpServer(payload);
    }
    case 'skills':
      throw new Error('Create skill via Skills page or ZIP import (`features/skills/api`).');
    case 'cron-tasks': {
      const svc = createCronService();
      const row = await svc.CreateCronTask({
        taskKey: payload.key,
        name: payload.name,
        description: payload.description,
        status: payload.status,
        enabled: payload.enabled,
        sortOrder: payload.sort_order,
        agentId: payload.agent_id,
        configJson: payload.config_json,
        metadataJson: payload.metadata_json,
      });
      return cronWireToPlatform(row);
    }
    case 'monitor-events':
    case 'monitor-traces':
      throw unsupported('create', resource);
    default:
      throw unsupported('create', resource);
  }
}

export async function updatePlatformResource(
  resource: PlatformResourceName,
  id: string,
  payload: Partial<PlatformResourceInput>,
): Promise<PlatformResource> {
  switch (resource) {
    case 'avatar-assets':
      throw unsupported('update', resource);
    case 'taxonomy-nodes': {
      const svc = createIndustryTaxonomyService();
      const cur = await svc.GetIndustryTaxonomy({ id });
      const curIsSystem = pickBool(asRecord(cur), 'is_system', 'isSystem');
      const curEnabled = pickBool(asRecord(cur), 'enabled', 'enabled');
      const merged = {
        id: cur.id,
        key: payload.key ?? cur.key,
        name: payload.name ?? cur.name,
        description: payload.description ?? cur.description,
        status: payload.status ?? cur.status,
        enabled: payload.enabled ?? curEnabled,
        sortOrder: payload.sort_order ?? cur.sortOrder,
        parentId: payload.parent_id !== undefined ? payload.parent_id || undefined : cur.parentId,
        level: payload.level ?? cur.level,
        workspaceId: cur.workspaceId,
        ownerUserId: cur.ownerUserId,
        isSystem: curIsSystem,
        configJson: payload.config_json ?? cur.configJson,
        metadataJson: payload.metadata_json ?? cur.metadataJson,
        createdAt: cur.createdAt,
        updatedAt: cur.updatedAt,
        deletedAt: cur.deletedAt,
      };
      const row = await svc.UpdateIndustryTaxonomy({ id, industryTaxonomy: merged });
      return industryTaxonomyWireToPlatform(row);
    }
    case 'taxonomy': {
      const svc = createTaxonomyService();
      const cur = await svc.GetTaxonomy({ id });
      const curIsSystem = pickBool(asRecord(cur), 'is_system', 'isSystem');
      const curEnabled = pickBool(asRecord(cur), 'enabled', 'enabled');
      const merged = {
        id: cur.id,
        key: payload.key ?? cur.key,
        name: payload.name ?? cur.name,
        description: payload.description ?? cur.description,
        status: payload.status ?? cur.status,
        enabled: payload.enabled ?? curEnabled,
        sortOrder: payload.sort_order ?? cur.sortOrder,
        parentId: payload.parent_id !== undefined ? payload.parent_id || undefined : cur.parentId,
        level: payload.level ?? cur.level,
        workspaceId: cur.workspaceId,
        ownerUserId: cur.ownerUserId,
        isSystem: curIsSystem,
        configJson: payload.config_json ?? cur.configJson,
        metadataJson: payload.metadata_json ?? cur.metadataJson,
        createdAt: cur.createdAt,
        updatedAt: cur.updatedAt,
        deletedAt: cur.deletedAt,
      };
      const row = await svc.UpdateTaxonomy({ id, node: merged });
      return taxonomyWireToPlatform(row);
    }
    case 'organization': {
      const svc = createOrganizationService();
      const cur = await svc.GetOrganization({ id });
      const curIsSystem = pickBool(asRecord(cur), 'isSystem', 'isSystem');
      const curEnabled = pickBool(asRecord(cur), 'enabled', 'enabled');
      const merged = {
        id: cur.id,
        orgKey: payload.key ?? cur.orgKey,
        name: payload.name ?? cur.name,
        description: payload.description ?? cur.description,
        status: payload.status ?? cur.status,
        enabled: payload.enabled ?? curEnabled,
        sortOrder: payload.sort_order ?? cur.sortOrder,
        parentId: payload.parent_id !== undefined ? payload.parent_id || undefined : cur.parentId,
        level: payload.level ?? cur.level,
        workspaceId: cur.workspaceId,
        ownerUserId: cur.ownerUserId,
        isSystem: curIsSystem,
        configJson: payload.config_json ?? cur.configJson,
        metadataJson: payload.metadata_json ?? cur.metadataJson,
        deptLeadAgentId: cur.deptLeadAgentId,
        deptLeadConfigJson: cur.deptLeadConfigJson,
        createdAt: cur.createdAt,
        updatedAt: cur.updatedAt,
        deletedAt: cur.deletedAt,
      };
      const row = await svc.UpdateOrganization({ id, node: merged });
      return organizationWireToPlatform(row);
    }
    case 'llm-provider-models': {
      const cur = (await llmModels.GetProviderModel({ id })) as ProviderModel;
      const merged = mergeProviderModel(cur, payload);
      const row = await llmModels.UpdateProviderModel({ id, providerModel: merged });
      return llmProviderWireToPlatform(row);
    }
    case 'hooks': {
      const svc = createHookService();
      const cur = await svc.GetHook({ id });
      const merged = {
        id: cur.id,
        key: payload.key ?? cur.key,
        name: payload.name ?? cur.name,
        description: payload.description ?? cur.description,
        status: payload.status ?? cur.status,
        enabled: payload.enabled ?? cur.enabled,
        sortOrder: payload.sort_order ?? cur.sortOrder,
        configJson: payload.config_json ?? cur.configJson,
        metadataJson: payload.metadata_json ?? cur.metadataJson,
        createdAt: cur.createdAt,
        updatedAt: cur.updatedAt,
        deletedAt: cur.deletedAt,
      };
      const row = await svc.UpdateHook({ id, hook: merged });
      return hookWireToPlatform(row);
    }
    case 'channels': {
      const svc = createChannelService();
      const cur = await svc.GetChannel({ id });
      const row = await svc.UpdateChannel({
        id,
        key: payload.key ?? cur.key,
        name: payload.name ?? cur.name,
        description: payload.description ?? cur.description,
        status: payload.status ?? cur.status,
        enabled: payload.enabled ?? cur.enabled,
        sortOrder: payload.sort_order ?? cur.sortOrder,
        configJson: payload.config_json ?? cur.configJson,
        metadataJson: payload.metadata_json ?? cur.metadataJson,
        credentials: [],
      });
      return channelWireToPlatform(row);
    }
    case 'mcp-servers': {
      return updateMcpServer(id, payload);
    }
    case 'skills':
      throw new Error('Update skill via Skills management UI (`features/skills/api`).');
    case 'cron-tasks': {
      const svc = createCronService();
      const cur = await svc.GetCronTask({ id });
      const row = await svc.UpdateCronTask({
        id,
        task: {
          id: cur.id,
          taskKey: payload.key ?? cur.taskKey,
          name: payload.name ?? cur.name,
          description: payload.description ?? cur.description,
          status: payload.status ?? cur.status,
          enabled: payload.enabled ?? cur.enabled,
          sortOrder: payload.sort_order ?? cur.sortOrder,
          agentId: payload.agent_id ?? cur.agentId,
          configJson: payload.config_json ?? cur.configJson,
          metadataJson: payload.metadata_json ?? cur.metadataJson,
          createdAt: cur.createdAt,
          updatedAt: cur.updatedAt,
          deletedAt: cur.deletedAt,
        },
      });
      return cronWireToPlatform(row);
    }
    case 'monitor-events':
    case 'monitor-traces':
      throw unsupported('update', resource);
    default:
      throw unsupported('update', resource);
  }
}

export async function deletePlatformResource(resource: PlatformResourceName, id: string): Promise<void> {
  switch (resource) {
    case 'avatar-assets': {
      const svc = createAvatarService();
      await svc.DeleteAvatarAsset({ id });
      return;
    }
    case 'taxonomy-nodes': {
      const svc = createIndustryTaxonomyService();
      await svc.DeleteIndustryTaxonomy({ id });
      return;
    }
    case 'taxonomy': {
      const svc = createTaxonomyService();
      await svc.DeleteTaxonomy({ id });
      return;
    }
    case 'organization': {
      const svc = createOrganizationService();
      await svc.DeleteOrganization({ id });
      return;
    }
    case 'llm-provider-models': {
      await llmModels.DeleteProviderModel({ id });
      return;
    }
    case 'hooks': {
      const svc = createHookService();
      await svc.DeleteHook({ id });
      return;
    }
    case 'channels': {
      const svc = createChannelService();
      await svc.DeleteChannel({ id });
      return;
    }
    case 'mcp-servers': {
      await deleteMcpServer(id);
      return;
    }
    case 'skills': {
      const svc = createSkillService();
      await svc.DeleteSkill({ id });
      return;
    }
    case 'cron-tasks': {
      const svc = createCronService();
      await svc.DeleteCronTask({ id });
      return;
    }
    case 'monitor-events':
    case 'monitor-traces':
      throw unsupported('delete', resource);
    default:
      throw unsupported('delete', resource);
  }
}

export async function validateModel(provider: string, model: string): Promise<ValidateModelResult> {
  const raw = await llmModels.ValidateProviderPair({ provider, model });
  const r = asRecord(raw);
  return { ok: pickBool(r, 'ok', 'ok'), message: pickStr(r, 'message', 'message') };
}

export async function revealProviderModelCredentials(id: string): Promise<RevealProviderCredentialsResult> {
  const raw = await llmModels.RevealProviderModelCredentials({ id });
  const r = asRecord(raw);
  const haRaw = r.ha_candidates ?? r.haCandidates;
  const haList = Array.isArray(haRaw) ? haRaw : [];
  return {
    api_key: pickStr(r, 'api_key', 'apiKey'),
    secret_key: pickStr(r, 'secret_key', 'secretKey'),
    has_api_key: pickBool(r, 'has_api_key', 'hasApiKey'),
    has_secret_key: pickBool(r, 'has_secret_key', 'hasSecretKey'),
    ha_candidates: haList.map((item) => {
      const row = asRecord(item);
      return {
        name: pickStr(row, 'name', 'name'),
        api_key: pickStr(row, 'api_key', 'apiKey'),
      };
    }),
  };
}

export async function inspectProviderModel(payload: InspectProviderModelInput): Promise<InspectProviderModelResult> {
  const raw = await llmModels.InspectProviderModel({
    resourceId: payload.resource_id,
    providerCode: payload.provider_code,
    providerType: payload.provider_type,
    modelApiId: payload.model_api_id,
    apiBaseUrl: payload.api_base_url,
    apiKey: payload.api_key,
    variant: payload.variant,
    secretId: payload.secret_id,
    secretKey: payload.secret_key,
    awsRegion: payload.aws_region,
  });
  const r = asRecord(raw);
  return {
    ok: pickBool(r, 'ok', 'ok'),
    message: pickStr(r, 'message', 'message'),
    provider_code: pickStr(r, 'provider_code', 'providerCode'),
    provider_type: pickStr(r, 'provider_type', 'providerType'),
    model_api_id: pickStr(r, 'model_api_id', 'modelApiId'),
    model_display_name: pickStr(r, 'model_display_name', 'modelDisplayName'),
    model_size_label: pickStr(r, 'model_size_label', 'modelSizeLabel'),
    context_window_k: pickNum(r, 'context_window_k', 'contextWindowK'),
    max_output_tokens: pickNum(r, 'max_output_tokens', 'maxOutputTokens'),
    input_price_micro_usd_per_1k: pickNum(r, 'input_price_micro_usd_per_1k', 'inputPriceMicroUsdPer1k'),
    output_price_micro_usd_per_1k: pickNum(r, 'output_price_micro_usd_per_1k', 'outputPriceMicroUsdPer1k'),
    cached_input_price_micro_usd_per_1k: pickNum(
      r,
      'cached_input_price_micro_usd_per_1k',
      'cachedInputPriceMicroUsdPer1k',
    ),
    reasoning_price_micro_usd_per_1k: pickNum(r, 'reasoning_price_micro_usd_per_1k', 'reasoningPriceMicroUsdPer1k'),
    embedding_price_micro_usd_per_1k: pickNum(r, 'embedding_price_micro_usd_per_1k', 'embeddingPriceMicroUsdPer1k'),
    source: pickStr(r, 'source', 'source'),
    raw_metadata_json: pickStr(r, 'raw_metadata_json', 'rawMetadataJson'),
    variant: pickStr(r, 'variant', 'variant'),
    enable_token_tailoring: pickBool(r, 'enable_token_tailoring', 'enableTokenTailoring'),
    supports_cache: pickBool(r, 'supports_cache', 'supportsCache'),
    supports_thinking: pickBool(r, 'supports_thinking', 'supportsThinking'),
  };
}

/** @deprecated 请从 `features/avatar/types` 导入 */
export type { AvatarAsset } from '../avatar/types';
/** @deprecated 请从 `features/avatar/api` 导入 */
export { listAvatarAssets, uploadAvatarAsset } from '../avatar/api';

export async function reorderTaxonomy(ids: string[]): Promise<void> {
  const svc = createTaxonomyService();
  await svc.ReorderTaxonomy({ ids });
}

export async function reorderOrganization(ids: string[]): Promise<void> {
  const svc = createOrganizationService();
  await svc.ReorderOrganization({ ids });
}
