/**
 * MCP 服务器：**`mcp_server/v1`** → **`kratosApi`** **`GET|POST|PATCH|DELETE /v1/mcp-servers`**（不再使用 **`legacyRestApi`**）。
 */
import { kratosApi } from "../../services/axiosHandler";
import type { PlatformResource, PlatformResourceInput } from "../platform/api";
import { asRecord, pickBool, pickI32, pickStr } from "../../shared/wireJson";
import type { McpServerTestResult } from "./types";

function mcpRowToPlatform(raw: unknown): PlatformResource {
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
    config_json: pickStr(r, "config_json", "configJson"),
    metadata_json: pickStr(r, "metadata_json", "metadataJson"),
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt"),
    deleted_at: pickStr(r, "deleted_at", "deletedAt")
  };
}

function createBody(p: PlatformResourceInput): Record<string, unknown> {
  return {
    key: p.key,
    name: p.name,
    description: p.description ?? "",
    status: p.status ?? "active",
    enabled: p.enabled ?? true,
    sort_order: p.sort_order ?? 0,
    config_json: p.config_json ?? "{}",
    metadata_json: p.metadata_json ?? "{}"
  };
}

/** PATCH body binds to **`MCPServer`**（snake_case JSON）。 */
function patchBody(p: Partial<PlatformResourceInput>): Record<string, unknown> {
  const o: Record<string, unknown> = {};
  if (p.key !== undefined) o.key = p.key;
  if (p.name !== undefined) o.name = p.name;
  if (p.description !== undefined) o.description = p.description;
  if (p.status !== undefined) o.status = p.status;
  if (p.enabled !== undefined) o.enabled = p.enabled;
  if (p.sort_order !== undefined) o.sort_order = p.sort_order;
  if (p.config_json !== undefined) o.config_json = p.config_json;
  if (p.metadata_json !== undefined) o.metadata_json = p.metadata_json;
  return o;
}

export async function listMcpServers(): Promise<PlatformResource[]> {
  const { data } = await kratosApi.get<{ items?: unknown[] }>("v1/mcp-servers");
  const items = data?.items ?? [];
  return items.map(mcpRowToPlatform);
}

export async function createMcpServer(payload: PlatformResourceInput): Promise<PlatformResource> {
  const { data } = await kratosApi.post<unknown>("v1/mcp-servers", createBody(payload));
  return mcpRowToPlatform(data);
}

export async function updateMcpServer(id: string, payload: Partial<PlatformResourceInput>): Promise<PlatformResource> {
  const { data } = await kratosApi.patch<unknown>(`v1/mcp-servers/${encodeURIComponent(id)}`, patchBody(payload));
  return mcpRowToPlatform(data);
}

export async function deleteMcpServer(id: string): Promise<void> {
  await kratosApi.delete(`v1/mcp-servers/${encodeURIComponent(id)}`);
}

export async function testMcpServer(id: string): Promise<McpServerTestResult> {
  const { data } = await kratosApi.post<unknown>(`v1/mcp-servers/${encodeURIComponent(id)}/test`, {});
  const r = asRecord(data);
  const detailsJson = pickStr(r, "details_json", "detailsJson");
  let details: Record<string, unknown> | undefined;
  if (detailsJson) {
    try {
      details = JSON.parse(detailsJson) as Record<string, unknown>;
    } catch {
      details = undefined;
    }
  }
  return {
    ok: pickBool(r, "ok", "ok"),
    status: pickStr(r, "status", "status"),
    message: pickStr(r, "message", "message"),
    details
  };
}
