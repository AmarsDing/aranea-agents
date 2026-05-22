import { createChannelService } from "../../services/index";
import { requestHandler } from "../../services/axiosHandler";
import { asRecord, pickStr } from "../../shared/wireJson";
import type {
  Channel as KratosChannel,
  ChannelCatalogItem as KratosCatalogItem,
  ChannelCredential as KratosCredential,
  ChannelTestResult as KratosTestResult
} from "../../services/kratos/channel/v1/index";
import type {
  ChannelCatalogItem,
  ChannelCredential,
  ChannelCredentialInput,
  ChannelResourceInput,
  ChannelRow,
  ChannelTestResult,
  ChannelTurnJobRow
} from "./types";

const channelApi = createChannelService();

function parseRecord(raw: string | undefined): Record<string, unknown> {
  const s = raw?.trim();
  if (!s) {
    return {};
  }
  try {
    return JSON.parse(s) as Record<string, unknown>;
  } catch {
    return {};
  }
}

function kratosCatalogToLegacy(k: KratosCatalogItem): ChannelCatalogItem {
  const cred = parseRecord(k.credentialSchemaJson);
  return {
    type: k.type ?? "",
    label: k.label ?? "",
    description: k.description ?? "",
    group: k.group ?? "",
    receive_modes: [...(k.receiveModes ?? [])],
    icon: k.icon ?? "",
    bundled: Boolean(k.bundled),
    supports_test: Boolean(k.supportsTest),
    supports_webhook: Boolean(k.supportsWebhook),
    config_schema: parseRecord(k.configSchemaJson),
    credential_schema: cred as ChannelCatalogItem["credential_schema"],
    ui_hints: parseRecord(k.uiHintsJson),
    sort_order: k.sortOrder ?? 0
  };
}

function kratosChannelToLegacy(k: KratosChannel): ChannelRow {
  return {
    id: k.id ?? "",
    resource: "channels",
    key: k.key ?? "",
    name: k.name ?? "",
    description: k.description ?? "",
    status: k.status ?? "",
    enabled: Boolean(k.enabled),
    sort_order: k.sortOrder ?? 0,
    parent_id: k.parentId ?? "",
    level: k.level ?? "",
    agent_id: k.agentId ?? "",
    provider: k.provider ?? "",
    model: k.model ?? "",
    config_json: k.configJson ?? "",
    metadata_json: k.metadataJson ?? "",
    created_at: k.createdAt ?? "",
    updated_at: k.updatedAt ?? "",
    deleted_at: k.deletedAt ?? ""
  };
}

function kratosCredentialToLegacy(c: KratosCredential): ChannelCredential {
  return {
    id: c.id ?? "",
    channel_id: c.channelId ?? "",
    credential_key: c.credentialKey ?? "",
    status: c.status ?? "",
    metadata_json: c.metadataJson ?? "",
    configured: Boolean(c.configured),
    masked_preview: c.maskedPreview ?? "",
    created_at: c.createdAt ?? "",
    updated_at: c.updatedAt ?? ""
  };
}

function kratosTestToLegacy(t: KratosTestResult): ChannelTestResult {
  let details: Record<string, unknown> | undefined;
  const dj = t.detailsJson?.trim();
  if (dj) {
    try {
      details = JSON.parse(dj) as Record<string, unknown>;
    } catch {
      details = undefined;
    }
  }
  return {
    ok: Boolean(t.ok),
    status: t.status ?? "",
    message: t.message ?? "",
    details
  };
}

function inputsToKratos(creds: ChannelCredentialInput[]) {
  return creds.map((c) => ({
    credentialKey: c.credential_key,
    secret: c.secret,
    secretRef: c.secret_ref,
    status: c.status,
    metadataJson: c.metadata_json ?? "{}"
  }));
}

export async function listChannelCatalog(): Promise<ChannelCatalogItem[]> {
  const data = await channelApi.ListChannelCatalog({});
  return (data.items ?? []).map(kratosCatalogToLegacy);
}

export async function listChannels(): Promise<ChannelRow[]> {
  const data = await channelApi.ListChannels({});
  return (data.items ?? []).map(kratosChannelToLegacy);
}

export async function createChannel(payload: ChannelResourceInput): Promise<ChannelRow> {
  const data = await channelApi.CreateChannel({
    key: payload.key,
    name: payload.name,
    description: payload.description,
    status: payload.status,
    enabled: payload.enabled,
    sortOrder: payload.sort_order,
    configJson: payload.config_json,
    metadataJson: payload.metadata_json,
    credentials: inputsToKratos(payload.credentials ?? [])
  });
  return kratosChannelToLegacy(data);
}

export async function updateChannel(id: string, payload: Partial<ChannelResourceInput>): Promise<ChannelRow> {
  const cur = await channelApi.GetChannel({ id });
  const base = kratosChannelToLegacy(cur);
  const data = await channelApi.UpdateChannel({
    id,
    key: payload.key ?? base.key,
    name: payload.name ?? base.name,
    description: payload.description ?? base.description,
    status: payload.status ?? base.status,
    enabled: payload.enabled ?? base.enabled,
    sortOrder: payload.sort_order ?? base.sort_order,
    configJson: payload.config_json ?? base.config_json,
    metadataJson: payload.metadata_json ?? base.metadata_json,
    credentials: inputsToKratos(payload.credentials ?? [])
  });
  return kratosChannelToLegacy(data);
}

export async function deleteChannel(id: string): Promise<void> {
  await channelApi.DeleteChannel({ id });
}

export async function toggleChannel(id: string, enabled: boolean): Promise<ChannelRow> {
  const data = await channelApi.ToggleChannel({ id, enabled });
  return kratosChannelToLegacy(data);
}

export async function testChannel(id: string): Promise<ChannelTestResult> {
  const data = await channelApi.TestChannel({ id });
  return kratosTestToLegacy(data);
}

export async function listChannelCredentials(id: string): Promise<ChannelCredential[]> {
  const data = await channelApi.ListChannelCredentials({ id });
  return (data.items ?? []).map(kratosCredentialToLegacy);
}

export async function updateChannelCredentials(id: string, credentials: ChannelCredentialInput[]): Promise<ChannelCredential[]> {
  const data = await channelApi.UpsertChannelCredentials({
    channelId: id,
    credentials: inputsToKratos(credentials)
  });
  return (data.items ?? []).map(kratosCredentialToLegacy);
}

export async function deleteChannelCredential(channelId: string, credentialKey: string): Promise<void> {
  await channelApi.DeleteChannelCredential({ channelId, credentialKey });
}

export type {
  ChannelRow,
  ChannelCatalogItem,
  ChannelResourceInput,
  ChannelCredential,
  ChannelCredentialInput,
  ChannelTestResult,
  ChannelTurnJobRow
} from "./types";

function wireChannelTurnJob(raw: unknown): ChannelTurnJobRow {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    channel_id: pickStr(r, "channel_id", "channelId"),
    session_id: pickStr(r, "session_id", "sessionId"),
    peer_id: pickStr(r, "peer_id", "peerId"),
    peer_key: pickStr(r, "peer_key", "peerKey"),
    idempotency_key: pickStr(r, "idempotency_key", "idempotencyKey"),
    status: pickStr(r, "status", "status"),
    preview_message_id: pickStr(r, "preview_message_id", "previewMessageId"),
    content_preview: pickStr(r, "content_preview", "contentPreview"),
    async_target_type: pickStr(r, "async_target_type", "asyncTargetType"),
    async_target_id: pickStr(r, "async_target_id", "asyncTargetId"),
    error_message: pickStr(r, "error_message", "errorMessage"),
    started_at: pickStr(r, "started_at", "startedAt"),
    finished_at: pickStr(r, "finished_at", "finishedAt"),
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt")
  };
}

/** TECH-DEBT: use channelApi.ListChannelTurnJobs after `make api` regenerates web client. */
export async function listChannelTurnJobs(channelId: string, limit = 30): Promise<{ items: ChannelTurnJobRow[] }> {
  const q = limit > 0 ? `?limit=${limit}` : "";
  const res = await requestHandler({
    path: `v1/channels/${encodeURIComponent(channelId)}/turn-jobs${q}`,
    method: "GET",
    body: null
  });
  const items = (res as { items?: unknown[] }).items ?? [];
  return { items: items.map(wireChannelTurnJob) };
}

export type ChannelDeliveryRow = {
  id: string;
  channel_id: string;
  agent_id: string;
  status: string;
  payload_json: string;
  error_message: string;
  created_at: string;
  updated_at: string;
};

export async function listChannelDeliveries(id: string, limit?: number): Promise<{ items: ChannelDeliveryRow[] }> {
  const data = await channelApi.ListChannelDeliveries({ id, limit: limit ?? 50 });
  return {
    items: (data.items ?? []).map((d) => ({
      id: d.id ?? "",
      channel_id: d.channelId ?? "",
      agent_id: d.agentId ?? "",
      status: d.status ?? "",
      payload_json: d.payloadJson ?? "",
      error_message: d.errorMessage ?? "",
      created_at: d.createdAt ?? "",
      updated_at: d.updatedAt ?? ""
    }))
  };
}
