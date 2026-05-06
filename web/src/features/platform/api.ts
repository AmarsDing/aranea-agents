/**
 * 平台资源：**按 `PlatformResourceName` 分发**至 **`avatar` / `agent_category` / `llm_provider_model` / `hook` / `channel` / `mcp` / `cron` / `monitor` / `skill`** 等 Kratos 客户端。
 * **不再使用 `legacyRestApi`**。
 *
 * **`validateModel`** / **`inspectProviderModel`**：**`llm_provider_model/v1`**。
 */
import type { CreateProviderModelRequest, ProviderModel } from "../../services/kratos/llm_provider_model/v1/index";
import {
  createAgentCategoryService,
  createAvatarService,
  createChannelService,
  createCronService,
  createHookService,
  createLlmProviderModelService,
  createMonitorService,
  createSkillService
} from "../../services";
import { kratosApi } from "../../services/axiosHandler";
import { listAvatarAssets } from "../avatar/api";
import { asRecord, pickBool, pickOptionalBoolDefaultTrue, pickI32, pickNum, pickStr } from "../memory/wireJson";
import { listSkills } from "../skills/api";
import type { Skill } from "../skills/types";
import { listModelUsageEvents } from "../usage/api";
import type { ModelTokenUsageEvent } from "../usage/types";

const llmModels = createLlmProviderModelService();

export type PlatformResourceName =
  | "avatar-assets"
  | "agent-categories"
  | "llm-provider-models"
  | "hooks"
  | "channels"
  | "mcp-servers"
  | "skills"
  | "cron-tasks"
  | "monitor-events"
  | "monitor-traces";

export type PlatformResource = {
  id: string;
  resource: PlatformResourceName;
  key: string;
  name: string;
  description: string;
  status: string;
  enabled: boolean;
  sort_order: number;
  parent_id: string;
  level: string;
  agent_id: string;
  provider: string;
  model: string;
  config_json: string;
  metadata_json: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
};

export type PlatformResourceTreeNode = PlatformResource & {
  children?: PlatformResourceTreeNode[];
};

export type PlatformResourceInput = Partial<Omit<PlatformResource, "id" | "resource" | "created_at" | "updated_at" | "deleted_at">> & {
  key: string;
  name: string;
};

export type ValidateModelResult = {
  ok: boolean;
  message: string;
};

export type InspectProviderModelInput = {
  resource_id?: string;
  provider_code: string;
  provider_type: string;
  model_api_id: string;
  api_base_url: string;
  api_key?: string;
};

export type InspectProviderModelResult = {
  ok: boolean;
  message: string;
  provider_code: string;
  provider_type: string;
  model_api_id: string;
  model_display_name: string;
  model_size_label: string;
  context_window_k: number;
  max_output_tokens: number;
  input_price_micro_usd_per_1k: number;
  output_price_micro_usd_per_1k: number;
  cached_input_price_micro_usd_per_1k: number;
  reasoning_price_micro_usd_per_1k: number;
  embedding_price_micro_usd_per_1k: number;
  source: string;
  raw_metadata_json: string;
};

function unsupported(op: string, resource: PlatformResourceName): Error {
  return new Error(`${op} unsupported for resource "${resource}" via platform gateway`);
}

function agentCategoryWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    resource: "agent-categories",
    key: pickStr(r, "key", "key"),
    name: pickStr(r, "name", "name"),
    description: pickStr(r, "description", "description"),
    status: pickStr(r, "status", "status"),
    enabled: pickBool(r, "enabled", "enabled"),
    sort_order: pickI32(r, "sort_order", "sortOrder"),
    parent_id: pickStr(r, "parent_id", "parentId"),
    level: pickStr(r, "level", "level"),
    agent_id: "",
    provider: "",
    model: "",
    config_json: pickStr(r, "config_json", "configJson") || "{}",
    metadata_json: pickStr(r, "metadata_json", "metadataJson") || "{}",
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt"),
    deleted_at: pickStr(r, "deleted_at", "deletedAt")
  };
}

function mapAgentCategoryTreeNode(raw: unknown): PlatformResourceTreeNode {
  const o = asRecord(raw);
  const cat = asRecord(o.category ?? o.Category);
  const base = agentCategoryWireToPlatform(cat);
  const childrenRaw = o.children ?? o.Children;
  const children = Array.isArray(childrenRaw) ? childrenRaw.map(mapAgentCategoryTreeNode) : [];
  return { ...base, children };
}

function llmProviderWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    resource: "llm-provider-models",
    key: pickStr(r, "key", "key"),
    name: pickStr(r, "name", "name"),
    description: pickStr(r, "description", "description"),
    status: pickStr(r, "status", "status"),
    enabled: pickOptionalBoolDefaultTrue(r, "enabled", "enabled"),
    sort_order: pickI32(r, "sort_order", "sortOrder"),
    parent_id: "",
    level: "",
    agent_id: "",
    provider: pickStr(r, "provider", "provider"),
    model: pickStr(r, "model", "model"),
    config_json: pickStr(r, "config_json", "configJson") || "{}",
    metadata_json: pickStr(r, "metadata_json", "metadataJson") || "{}",
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt"),
    deleted_at: pickStr(r, "deleted_at", "deletedAt")
  };
}

function hookWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    resource: "hooks",
    key: pickStr(r, "key", "key"),
    name: pickStr(r, "name", "name"),
    description: pickStr(r, "description", "description"),
    status: pickStr(r, "status", "status"),
    enabled: pickBool(r, "enabled", "enabled"),
    sort_order: pickI32(r, "sort_order", "sortOrder"),
    parent_id: "",
    level: "",
    agent_id: "",
    provider: "",
    model: "",
    config_json: pickStr(r, "config_json", "configJson") || "{}",
    metadata_json: pickStr(r, "metadata_json", "metadataJson") || "{}",
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt"),
    deleted_at: pickStr(r, "deleted_at", "deletedAt")
  };
}

function channelWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    resource: "channels",
    key: pickStr(r, "key", "key"),
    name: pickStr(r, "name", "name"),
    description: pickStr(r, "description", "description"),
    status: pickStr(r, "status", "status"),
    enabled: pickBool(r, "enabled", "enabled"),
    sort_order: pickI32(r, "sort_order", "sortOrder"),
    parent_id: pickStr(r, "parent_id", "parentId"),
    level: pickStr(r, "level", "level"),
    agent_id: pickStr(r, "agent_id", "agentId"),
    provider: pickStr(r, "provider", "provider"),
    model: pickStr(r, "model", "model"),
    config_json: pickStr(r, "config_json", "configJson") || "{}",
    metadata_json: pickStr(r, "metadata_json", "metadataJson") || "{}",
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt"),
    deleted_at: pickStr(r, "deleted_at", "deletedAt")
  };
}

function mcpWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    resource: "mcp-servers",
    key: pickStr(r, "key", "key"),
    name: pickStr(r, "name", "name"),
    description: pickStr(r, "description", "description"),
    status: pickStr(r, "status", "status"),
    enabled: pickBool(r, "enabled", "enabled"),
    sort_order: pickI32(r, "sort_order", "sortOrder"),
    parent_id: "",
    level: "",
    agent_id: "",
    provider: "",
    model: "",
    config_json: pickStr(r, "config_json", "configJson") || "{}",
    metadata_json: pickStr(r, "metadata_json", "metadataJson") || "{}",
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt"),
    deleted_at: pickStr(r, "deleted_at", "deletedAt")
  };
}

function cronWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    resource: "cron-tasks",
    key: pickStr(r, "task_key", "taskKey"),
    name: pickStr(r, "name", "name"),
    description: pickStr(r, "description", "description"),
    status: pickStr(r, "status", "status"),
    enabled: pickBool(r, "enabled", "enabled"),
    sort_order: pickI32(r, "sort_order", "sortOrder"),
    parent_id: "",
    level: "",
    agent_id: pickStr(r, "agent_id", "agentId"),
    provider: "",
    model: "",
    config_json: pickStr(r, "config_json", "configJson") || "{}",
    metadata_json: pickStr(r, "metadata_json", "metadataJson") || "{}",
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt"),
    deleted_at: pickStr(r, "deleted_at", "deletedAt")
  };
}

function monitorEventWireToPlatform(raw: unknown): PlatformResource {
  const r = asRecord(raw);
  const kind = pickStr(r, "resource", "resource");
  const resName: PlatformResourceName =
    kind === "monitor-traces" ? "monitor-traces" : "monitor-events";
  return {
    id: pickStr(r, "id", "id"),
    resource: resName,
    key: pickStr(r, "key", "key"),
    name: pickStr(r, "name", "name"),
    description: pickStr(r, "description", "description"),
    status: pickStr(r, "status", "status"),
    enabled: pickBool(r, "enabled", "enabled"),
    sort_order: pickI32(r, "sort_order", "sortOrder"),
    parent_id: pickStr(r, "parent_id", "parentId"),
    level: pickStr(r, "level", "level"),
    agent_id: pickStr(r, "agent_id", "agentId"),
    provider: pickStr(r, "provider", "provider"),
    model: pickStr(r, "model", "model"),
    config_json: pickStr(r, "config_json", "configJson") || "{}",
    metadata_json: pickStr(r, "metadata_json", "metadataJson") || "{}",
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt"),
    deleted_at: pickStr(r, "deleted_at", "deletedAt")
  };
}

function usageEventToPlatformTrace(e: ModelTokenUsageEvent): PlatformResource {
  const name = [e.model_display_name || e.model_api_id, e.agent_key].filter(Boolean).join(" · ") || e.id;
  return {
    id: e.id,
    resource: "monitor-traces",
    key: e.message_id || e.id,
    name,
    description: e.error_message || e.status || "",
    status: e.status,
    enabled: true,
    sort_order: 0,
    parent_id: "",
    level: "",
    agent_id: e.agent_id,
    provider: e.provider_code,
    model: e.model_api_id,
    config_json: JSON.stringify({
      session_id: e.session_id,
      latency_ms: e.latency_ms,
      total_tokens: e.total_tokens,
      total_cost_micro_usd: e.total_cost_micro_usd
    }),
    metadata_json: "{}",
    created_at: e.occurred_at,
    updated_at: e.occurred_at,
    deleted_at: ""
  };
}

function skillToPlatform(s: Skill): PlatformResource {
  return {
    id: s.id,
    resource: "skills",
    key: s.slug,
    name: s.name,
    description: s.description,
    status: s.status,
    enabled: s.enabled,
    sort_order: 0,
    parent_id: "",
    level: "",
    agent_id: s.last_agent_id ?? "",
    provider: "",
    model: "",
    config_json: JSON.stringify({
      tags: s.tags,
      extends_skill_id: s.extends_skill_id,
      invoke_count: s.invoke_count,
      success_count: s.success_count,
      failure_count: s.failure_count
    }),
    metadata_json: JSON.stringify(s.current_version ?? {}),
    created_at: s.created_at,
    updated_at: s.updated_at,
    deleted_at: ""
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
    resource: "avatar-assets",
    key: row.key,
    name: row.name,
    description: row.description,
    status: "active",
    enabled: true,
    sort_order: row.sort_order,
    parent_id: "",
    level: "",
    agent_id: "",
    provider: "",
    model: "",
    config_json: JSON.stringify({
      mime_type: row.mime_type,
      workspace_id: row.workspace_id,
      owner_user_id: row.owner_user_id,
      source: row.source,
      is_system: row.is_system,
      file_size_bytes: row.file_size_bytes,
      width_px: row.width_px,
      height_px: row.height_px
    }),
    metadata_json: "{}",
    created_at: row.created_at,
    updated_at: row.created_at,
    deleted_at: ""
  };
}

function providerInputToCreateBody(payload: PlatformResourceInput): CreateProviderModelRequest {
  return {
    key: payload.key,
    name: payload.name,
    description: payload.description ?? "",
    status: payload.status ?? "active",
    enabled: payload.enabled ?? true,
    sortOrder: payload.sort_order ?? 0,
    provider: payload.provider ?? "",
    model: payload.model ?? "",
    configJson: payload.config_json ?? "{}",
    metadataJson: payload.metadata_json ?? "{}"
  };
}

function providerModelConfigJsonFromWire(base: ProviderModel): string | undefined {
  const r = base as unknown as Record<string, unknown>;
  const v = r.configJson ?? r.config_json;
  if (v === undefined || v === null) return undefined;
  const s = String(v);
  return s === "" ? undefined : s;
}

function providerModelMetadataJsonFromWire(base: ProviderModel): string | undefined {
  const r = base as unknown as Record<string, unknown>;
  const v = r.metadataJson ?? r.metadata_json;
  if (v === undefined || v === null) return undefined;
  const s = String(v);
  return s === "" ? undefined : s;
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
    configJson: patch.config_json ?? baseConfig ?? base.configJson ?? "{}",
    metadataJson: patch.metadata_json ?? baseMeta ?? base.metadataJson ?? "{}",
    createdAt: base.createdAt,
    updatedAt: base.updatedAt,
    deletedAt: base.deletedAt
  };
}

export async function listPlatformResources(resource: PlatformResourceName): Promise<PlatformResource[]> {
  switch (resource) {
    case "avatar-assets": {
      const rows = await listAvatarAssets();
      return rows.map(avatarAssetToPlatform);
    }
    case "agent-categories": {
      const svc = createAgentCategoryService();
      const res = await svc.ListAgentCategories({});
      return (res.items ?? []).map((row: unknown) => agentCategoryWireToPlatform(row));
    }
    case "llm-provider-models": {
      const res = await llmModels.ListProviderModels({});
      return (res.items ?? []).map((row: unknown) => llmProviderWireToPlatform(row));
    }
    case "hooks": {
      const svc = createHookService();
      const res = await svc.ListHooks({});
      return (res.items ?? []).map((row: unknown) => hookWireToPlatform(row));
    }
    case "channels": {
      const svc = createChannelService();
      const res = await svc.ListChannels({});
      return (res.items ?? []).map((row: unknown) => channelWireToPlatform(row));
    }
    case "mcp-servers": {
      const { data } = await kratosApi.get<{ items?: unknown[] }>("v1/mcp-servers");
      return (data?.items ?? []).map(mcpWireToPlatform);
    }
    case "skills": {
      const { items } = await listSkills({ page: 1, page_size: 500 });
      return items.map(skillToPlatform);
    }
    case "cron-tasks": {
      const svc = createCronService();
      const res = await svc.ListCronTasks({});
      return (res.items ?? []).map((row: unknown) => cronWireToPlatform(row));
    }
    case "monitor-events": {
      const svc = createMonitorService();
      const res = await svc.ListMonitorEvents({});
      return (res.items ?? []).map((row: unknown) => monitorEventWireToPlatform(row));
    }
    case "monitor-traces": {
      const events = await listModelUsageEvents({ limit: 200 });
      return events.map(usageEventToPlatformTrace);
    }
    default:
      return [];
  }
}

export async function listPlatformResourceTree(resource: "agent-categories"): Promise<PlatformResourceTreeNode[]> {
  if (resource !== "agent-categories") return [];
  const svc = createAgentCategoryService();
  const res = await svc.ListAgentCategoryTree({});
  const items = res.items ?? [];
  return items.map(mapAgentCategoryTreeNode);
}

export async function createPlatformResource(
  resource: PlatformResourceName,
  payload: PlatformResourceInput
): Promise<PlatformResource> {
  switch (resource) {
    case "avatar-assets":
      throw unsupported("create", resource);
    case "agent-categories": {
      const svc = createAgentCategoryService();
      const row = await svc.CreateAgentCategory({
        key: payload.key,
        name: payload.name,
        description: payload.description,
        status: payload.status ?? "active",
        enabled: payload.enabled ?? true,
        sortOrder: payload.sort_order ?? 0,
        parentId: payload.parent_id || undefined,
        level: payload.level || undefined,
        workspaceId: undefined,
        ownerUserId: undefined,
        configJson: payload.config_json ?? "{}",
        metadataJson: payload.metadata_json ?? "{}"
      });
      return agentCategoryWireToPlatform(row);
    }
    case "llm-provider-models": {
      const row = await llmModels.CreateProviderModel(providerInputToCreateBody(payload));
      return llmProviderWireToPlatform(row);
    }
    case "hooks": {
      const svc = createHookService();
      const row = await svc.CreateHook({
        key: payload.key,
        name: payload.name,
        description: payload.description,
        status: payload.status ?? "active",
        enabled: payload.enabled ?? true,
        sortOrder: payload.sort_order ?? 0,
        configJson: payload.config_json ?? "{}",
        metadataJson: payload.metadata_json ?? "{}"
      });
      return hookWireToPlatform(row);
    }
    case "channels": {
      const svc = createChannelService();
      const row = await svc.CreateChannel({
        key: payload.key,
        name: payload.name,
        description: payload.description ?? "",
        status: payload.status ?? "active",
        enabled: payload.enabled ?? true,
        sortOrder: payload.sort_order ?? 0,
        configJson: payload.config_json ?? "{}",
        metadataJson: payload.metadata_json ?? "{}",
        credentials: []
      });
      return channelWireToPlatform(row);
    }
    case "mcp-servers": {
      const body = {
        key: payload.key,
        name: payload.name,
        description: payload.description ?? "",
        status: payload.status ?? "active",
        enabled: payload.enabled ?? true,
        sort_order: payload.sort_order ?? 0,
        config_json: payload.config_json ?? "{}",
        metadata_json: payload.metadata_json ?? "{}"
      };
      const { data } = await kratosApi.post<unknown>("v1/mcp-servers", body);
      return mcpWireToPlatform(data);
    }
    case "skills":
      throw new Error('Create skill via Skills page or ZIP import (`features/skills/api`).');
    case "cron-tasks": {
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
        metadataJson: payload.metadata_json
      });
      return cronWireToPlatform(row);
    }
    case "monitor-events":
    case "monitor-traces":
      throw unsupported("create", resource);
    default:
      throw unsupported("create", resource);
  }
}

export async function updatePlatformResource(
  resource: PlatformResourceName,
  id: string,
  payload: Partial<PlatformResourceInput>
): Promise<PlatformResource> {
  switch (resource) {
    case "avatar-assets":
      throw unsupported("update", resource);
    case "agent-categories": {
      const svc = createAgentCategoryService();
      const cur = await svc.GetAgentCategory({ id });
      const merged = {
        id: cur.id,
        key: payload.key ?? cur.key,
        name: payload.name ?? cur.name,
        description: payload.description ?? cur.description,
        status: payload.status ?? cur.status,
        enabled: payload.enabled ?? cur.enabled,
        sortOrder: payload.sort_order ?? cur.sortOrder,
        parentId: payload.parent_id !== undefined ? payload.parent_id || undefined : cur.parentId,
        level: payload.level ?? cur.level,
        workspaceId: cur.workspaceId,
        ownerUserId: cur.ownerUserId,
        isSystem: cur.isSystem,
        configJson: payload.config_json ?? cur.configJson,
        metadataJson: payload.metadata_json ?? cur.metadataJson,
        createdAt: cur.createdAt,
        updatedAt: cur.updatedAt,
        deletedAt: cur.deletedAt
      };
      const row = await svc.UpdateAgentCategory({ id, category: merged });
      return agentCategoryWireToPlatform(row);
    }
    case "llm-provider-models": {
      const cur = (await llmModels.GetProviderModel({ id })) as ProviderModel;
      const merged = mergeProviderModel(cur, payload);
      const row = await llmModels.UpdateProviderModel({ id, providerModel: merged });
      return llmProviderWireToPlatform(row);
    }
    case "hooks": {
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
        deletedAt: cur.deletedAt
      };
      const row = await svc.UpdateHook({ id, hook: merged });
      return hookWireToPlatform(row);
    }
    case "channels": {
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
        credentials: []
      });
      return channelWireToPlatform(row);
    }
    case "mcp-servers": {
      const o: Record<string, unknown> = {};
      if (payload.key !== undefined) o.key = payload.key;
      if (payload.name !== undefined) o.name = payload.name;
      if (payload.description !== undefined) o.description = payload.description;
      if (payload.status !== undefined) o.status = payload.status;
      if (payload.enabled !== undefined) o.enabled = payload.enabled;
      if (payload.sort_order !== undefined) o.sort_order = payload.sort_order;
      if (payload.config_json !== undefined) o.config_json = payload.config_json;
      if (payload.metadata_json !== undefined) o.metadata_json = payload.metadata_json;
      const { data } = await kratosApi.patch<unknown>(`v1/mcp-servers/${encodeURIComponent(id)}`, o);
      return mcpWireToPlatform(data);
    }
    case "skills":
      throw new Error('Update skill via Skills management UI (`features/skills/api`).');
    case "cron-tasks": {
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
          deletedAt: cur.deletedAt
        }
      });
      return cronWireToPlatform(row);
    }
    case "monitor-events":
    case "monitor-traces":
      throw unsupported("update", resource);
    default:
      throw unsupported("update", resource);
  }
}

export async function deletePlatformResource(resource: PlatformResourceName, id: string): Promise<void> {
  switch (resource) {
    case "avatar-assets": {
      const svc = createAvatarService();
      await svc.DeleteAvatarAsset({ id });
      return;
    }
    case "agent-categories": {
      const svc = createAgentCategoryService();
      await svc.DeleteAgentCategory({ id });
      return;
    }
    case "llm-provider-models": {
      await llmModels.DeleteProviderModel({ id });
      return;
    }
    case "hooks": {
      const svc = createHookService();
      await svc.DeleteHook({ id });
      return;
    }
    case "channels": {
      const svc = createChannelService();
      await svc.DeleteChannel({ id });
      return;
    }
    case "mcp-servers": {
      await kratosApi.delete(`v1/mcp-servers/${encodeURIComponent(id)}`);
      return;
    }
    case "skills": {
      const svc = createSkillService();
      await svc.DeleteSkill({ id });
      return;
    }
    case "cron-tasks": {
      const svc = createCronService();
      await svc.DeleteCronTask({ id });
      return;
    }
    case "monitor-events":
    case "monitor-traces":
      throw unsupported("delete", resource);
    default:
      throw unsupported("delete", resource);
  }
}

export async function validateModel(provider: string, model: string): Promise<ValidateModelResult> {
  const raw = await llmModels.ValidateProviderPair({ provider, model });
  const r = asRecord(raw);
  return { ok: pickBool(r, "ok", "ok"), message: pickStr(r, "message", "message") };
}

export async function inspectProviderModel(payload: InspectProviderModelInput): Promise<InspectProviderModelResult> {
  const raw = await llmModels.InspectProviderModel({
    resourceId: payload.resource_id,
    providerCode: payload.provider_code,
    providerType: payload.provider_type,
    modelApiId: payload.model_api_id,
    apiBaseUrl: payload.api_base_url,
    apiKey: payload.api_key
  });
  const r = asRecord(raw);
  return {
    ok: pickBool(r, "ok", "ok"),
    message: pickStr(r, "message", "message"),
    provider_code: pickStr(r, "provider_code", "providerCode"),
    provider_type: pickStr(r, "provider_type", "providerType"),
    model_api_id: pickStr(r, "model_api_id", "modelApiId"),
    model_display_name: pickStr(r, "model_display_name", "modelDisplayName"),
    model_size_label: pickStr(r, "model_size_label", "modelSizeLabel"),
    context_window_k: pickNum(r, "context_window_k", "contextWindowK"),
    max_output_tokens: pickNum(r, "max_output_tokens", "maxOutputTokens"),
    input_price_micro_usd_per_1k: pickNum(r, "input_price_micro_usd_per_1k", "inputPriceMicroUsdPer1k"),
    output_price_micro_usd_per_1k: pickNum(r, "output_price_micro_usd_per_1k", "outputPriceMicroUsdPer1k"),
    cached_input_price_micro_usd_per_1k: pickNum(r, "cached_input_price_micro_usd_per_1k", "cachedInputPriceMicroUsdPer1k"),
    reasoning_price_micro_usd_per_1k: pickNum(r, "reasoning_price_micro_usd_per_1k", "reasoningPriceMicroUsdPer1k"),
    embedding_price_micro_usd_per_1k: pickNum(r, "embedding_price_micro_usd_per_1k", "embeddingPriceMicroUsdPer1k"),
    source: pickStr(r, "source", "source"),
    raw_metadata_json: pickStr(r, "raw_metadata_json", "rawMetadataJson")
  };
}

/** @deprecated 请从 `features/avatar/api` 导入 */
export type { AvatarAsset } from "../avatar/api";
/** @deprecated 请从 `features/avatar/api` 导入 */
export { listAvatarAssets, uploadAvatarAsset } from "../avatar/api";
