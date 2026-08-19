/**
 * MCP 服务器：**`createMCPServerService()`** 生成客户端 → **`/v1/mcp-servers`**。
 * 更新操作先 GET 获取当前状态，merge 后再 PATCH，与 channel 保持一致的全字段替换语义。
 */
import { createMCPServerService } from '../../services';
import type { PlatformResourceInput } from '../platform/types';
import { asRecord, pickBool, pickI32, pickStr } from '../../shared/wireJson';
import type {
  McpServerRow,
  McpServerTestResult,
  McpServerValidateResult,
  McpUserCredential,
  McpUserCredentialInput,
} from './types';

export type { PlatformResource, PlatformResourceInput } from '../platform/types';

const svc = createMCPServerService();

function mcpRowToPlatform(raw: unknown): McpServerRow {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    resource: 'mcp-servers',
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
    shared: pickBool(r, 'shared', 'shared'),
    config_json: pickStr(r, 'config_json', 'configJson'),
    metadata_json: pickStr(r, 'metadata_json', 'metadataJson'),
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    deleted_at: pickStr(r, 'deleted_at', 'deletedAt'),
  };
}

export type McpServerListQuery = {
  page?: number;
  page_size?: number;
  search?: string;
};

/** MCP 采纳汇总：多少 Agent 的有效工具策略开启了 MCP 门禁。 */
export type McpUsageSummary = {
  enabled_agent_count: number;
  total_agent_count: number;
};

export type McpServerListResult = {
  items: McpServerRow[];
  total: number;
  page: number;
  page_size: number;
  /** best-effort：后端未注入 AgentUsecase 或计算失败时缺省。 */
  summary?: McpUsageSummary;
};

/** Full catalog (no page params) — pickers / platform resource manager. */
export async function listMcpServers(): Promise<McpServerRow[]> {
  const res = asRecord(await svc.ListMCPServers({}));
  const items = res.items ?? res.Items;
  return Array.isArray(items) ? items.map(mcpRowToPlatform) : [];
}

/** Admin registry page — server pagination. */
export async function listMcpServersPaged(query: McpServerListQuery = {}): Promise<McpServerListResult> {
  const page = query.page ?? 1;
  const pageSize = query.page_size ?? 20;
  const res = asRecord(
    await svc.ListMCPServers({
      page,
      pageSize,
      search: query.search?.trim() || undefined,
    }),
  );
  const items = res.items ?? res.Items;
  const summary = extractUsageSummary(res);
  return {
    items: Array.isArray(items) ? items.map(mcpRowToPlatform) : [],
    total: Number(res.total ?? 0),
    page: Number(res.page ?? page),
    page_size: Number(res.pageSize ?? res.page_size ?? pageSize),
    summary,
  };
}

/** 从 list 响应提取 MCP 采纳汇总；字段缺省/非数时返回 undefined（best-effort）。 */
function extractUsageSummary(res: Record<string, unknown>): McpUsageSummary | undefined {
  const raw = asRecord(res.summary ?? (res as { Summary?: unknown }).Summary);
  if (!raw) return undefined;
  const enabled = Number(raw.enabled_agent_count ?? raw.enabledAgentCount ?? NaN);
  const total = Number(raw.total_agent_count ?? raw.totalAgentCount ?? NaN);
  if (!Number.isFinite(enabled) || !Number.isFinite(total)) return undefined;
  return { enabled_agent_count: enabled, total_agent_count: total };
}

export async function createMcpServer(payload: PlatformResourceInput): Promise<McpServerRow> {
  const row = await svc.CreateMCPServer({
    key: payload.key,
    name: payload.name,
    description: payload.description ?? '',
    status: payload.status ?? 'active',
    enabled: payload.enabled ?? true,
    sortOrder: payload.sort_order ?? 0,
    configJson: payload.config_json ?? '{}',
    metadataJson: payload.metadata_json ?? '{}',
  });
  return mcpRowToPlatform(row);
}

export async function updateMcpServer(id: string, payload: Partial<PlatformResourceInput>): Promise<McpServerRow> {
  const cur = asRecord(await svc.GetMCPServer({ id }));
  const row = await svc.UpdateMCPServer({
    id,
    mcpServer: {
      id,
      key: payload.key ?? pickStr(cur, 'key', 'key'),
      name: payload.name ?? pickStr(cur, 'name', 'name'),
      description: payload.description ?? pickStr(cur, 'description', 'description'),
      status: payload.status ?? pickStr(cur, 'status', 'status'),
      enabled: payload.enabled !== undefined ? payload.enabled : pickBool(cur, 'enabled', 'enabled'),
      sortOrder: payload.sort_order !== undefined ? payload.sort_order : pickI32(cur, 'sort_order', 'sortOrder'),
      configJson: payload.config_json ?? pickStr(cur, 'config_json', 'configJson'),
      metadataJson: payload.metadata_json ?? pickStr(cur, 'metadata_json', 'metadataJson'),
      createdAt: pickStr(cur, 'created_at', 'createdAt'),
      updatedAt: pickStr(cur, 'updated_at', 'updatedAt'),
      deletedAt: pickStr(cur, 'deleted_at', 'deletedAt'),
    },
  });
  return mcpRowToPlatform(row);
}

export async function deleteMcpServer(id: string): Promise<void> {
  await svc.DeleteMCPServer({ id });
}

export async function validateMcpServer(enabled: boolean, configJson: string): Promise<McpServerValidateResult> {
  const res = asRecord(await svc.ValidateMCPServer({ enabled, configJson }));
  const detailsJson = pickStr(res, 'details_json', 'detailsJson');
  let details: Record<string, unknown> | undefined;
  if (detailsJson) {
    try {
      details = JSON.parse(detailsJson) as Record<string, unknown>;
    } catch {
      details = undefined;
    }
  }
  return {
    ok: pickBool(res, 'ok', 'ok'),
    status: pickStr(res, 'status', 'status'),
    message: pickStr(res, 'message', 'message'),
    details,
  };
}

export async function testMcpServer(id: string): Promise<McpServerTestResult> {
  const res = asRecord(await svc.TestMCPServer({ id }));
  const detailsJson = pickStr(res, 'details_json', 'detailsJson');
  let details: Record<string, unknown> | undefined;
  if (detailsJson) {
    try {
      details = JSON.parse(detailsJson) as Record<string, unknown>;
    } catch {
      details = undefined;
    }
  }
  return {
    ok: pickBool(res, 'ok', 'ok'),
    status: pickStr(res, 'status', 'status'),
    message: pickStr(res, 'message', 'message'),
    details,
  };
}

function mcpUserCredFromWire(raw: unknown): McpUserCredential {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    mcp_server_id: pickStr(r, 'mcp_server_id', 'mcpServerId'),
    user_id: pickStr(r, 'user_id', 'userId'),
    credential_key: pickStr(r, 'credential_key', 'credentialKey'),
    status: pickStr(r, 'status', 'status'),
    configured: pickBool(r, 'configured', 'configured'),
    masked_preview: pickStr(r, 'masked_preview', 'maskedPreview'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
  };
}

export async function listMcpUserCredentials(mcpServerId: string, userId: string): Promise<McpUserCredential[]> {
  const res = asRecord(await svc.ListMCPServerUserCredentials({ mcpServerId, userId }));
  const items = res.items ?? res.Items;
  return Array.isArray(items) ? items.map(mcpUserCredFromWire) : [];
}

export async function upsertMcpUserCredential(
  mcpServerId: string,
  userId: string,
  input: McpUserCredentialInput,
): Promise<McpUserCredential> {
  const row = await svc.UpsertMCPServerUserCredential({
    mcpServerId,
    userId,
    credentialKey: input.credential_key,
    secret: input.secret,
    status: input.status ?? 'active',
  });
  return mcpUserCredFromWire(row);
}

export async function deleteMcpUserCredential(
  mcpServerId: string,
  userId: string,
  credentialKey: string,
): Promise<void> {
  await svc.DeleteMCPServerUserCredential({ mcpServerId, userId, credentialKey });
}
