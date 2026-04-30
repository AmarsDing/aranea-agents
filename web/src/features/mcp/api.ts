import { legacyRestApi as api } from "../../services";
import type { PlatformResource, PlatformResourceInput } from "../platform/api";
import type { McpServerTestResult } from "./types";

type ListResponse<T> = {
  items: T[];
};

export async function listMcpServers(): Promise<PlatformResource[]> {
  const { data } = await api.get<ListResponse<PlatformResource>>("/mcp-servers");
  return data.items ?? [];
}

export async function createMcpServer(payload: PlatformResourceInput): Promise<PlatformResource> {
  const { data } = await api.post<PlatformResource>("/mcp-servers", payload);
  return data;
}

export async function updateMcpServer(id: string, payload: Partial<PlatformResourceInput>): Promise<PlatformResource> {
  const { data } = await api.patch<PlatformResource>(`/mcp-servers/${id}`, payload);
  return data;
}

export async function deleteMcpServer(id: string): Promise<void> {
  await api.delete(`/mcp-servers/${id}`);
}

export async function testMcpServer(id: string): Promise<McpServerTestResult> {
  const { data } = await api.post<McpServerTestResult>(`/mcp-servers/${id}/test`);
  return data;
}
