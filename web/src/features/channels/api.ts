import { api } from "../../api/http";
import type { ChannelCatalogItem, ChannelCredential, ChannelCredentialInput, ChannelResourceInput, ChannelRow, ChannelTestResult } from "./types";

type ListResponse<T> = {
  items: T[];
};

export async function listChannelCatalog(): Promise<ChannelCatalogItem[]> {
  const { data } = await api.get<ListResponse<ChannelCatalogItem>>("/channels/catalog");
  return data.items ?? [];
}

export async function listChannels(): Promise<ChannelRow[]> {
  const { data } = await api.get<ListResponse<ChannelRow>>("/channels");
  return data.items ?? [];
}

export async function createChannel(payload: ChannelResourceInput): Promise<ChannelRow> {
  const { data } = await api.post<ChannelRow>("/channels", payload);
  return data;
}

export async function updateChannel(id: string, payload: Partial<ChannelResourceInput>): Promise<ChannelRow> {
  const { data } = await api.patch<ChannelRow>(`/channels/${id}`, payload);
  return data;
}

export async function deleteChannel(id: string): Promise<void> {
  await api.delete(`/channels/${id}`);
}

export async function toggleChannel(id: string, enabled: boolean): Promise<ChannelRow> {
  const { data } = await api.post<ChannelRow>(`/channels/${id}/toggle`, { enabled });
  return data;
}

export async function testChannel(id: string): Promise<ChannelTestResult> {
  const { data } = await api.post<ChannelTestResult>(`/channels/${id}/test`);
  return data;
}

export async function listChannelCredentials(id: string): Promise<ChannelCredential[]> {
  const { data } = await api.get<ListResponse<ChannelCredential>>(`/channels/${id}/credentials`);
  return data.items ?? [];
}

export async function updateChannelCredentials(id: string, credentials: ChannelCredentialInput[]): Promise<ChannelCredential[]> {
  const { data } = await api.put<ListResponse<ChannelCredential>>(`/channels/${id}/credentials`, { credentials });
  return data.items ?? [];
}
