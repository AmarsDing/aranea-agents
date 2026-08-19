import { createChannelService } from '../../services/index';
import type {
  Channel as KratosChannel,
  ChannelTypeItem as KratosCatalogItem,
  ChannelCredential as KratosCredential,
  ChannelTestResult as KratosTestResult,
} from '../../services/kratos/channel/v1/index';
import type {
  ChannelTypeItem,
  ChannelCredential,
  ChannelCredentialInput,
  ChannelDeliveryRow,
  ChannelResourceInput,
  ChannelRow,
  ChannelTestResult,
  ChannelTurnJobRow,
} from './types';

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

function kratosCatalogToLegacy(k: KratosCatalogItem): ChannelTypeItem {
  const cred = parseRecord(k.credentialSchemaJson);
  return {
    type: k.type ?? '',
    label: k.label ?? '',
    description: k.description ?? '',
    group: k.group ?? '',
    receive_modes: [...(k.receiveModes ?? [])],
    icon: k.icon ?? '',
    bundled: Boolean(k.bundled),
    supports_test: Boolean(k.supportsTest),
    supports_webhook: Boolean(k.supportsWebhook),
    config_schema: parseRecord(k.configSchemaJson),
    credential_schema: cred as ChannelTypeItem['credential_schema'],
    ui_hints: parseRecord(k.uiHintsJson),
    sort_order: k.sortOrder ?? 0,
  };
}

function kratosChannelToLegacy(k: KratosChannel): ChannelRow {
  return {
    id: k.id ?? '',
    resource: 'channels',
    key: k.key ?? '',
    name: k.name ?? '',
    description: k.description ?? '',
    status: k.status ?? '',
    enabled: Boolean(k.enabled),
    sort_order: k.sortOrder ?? 0,
    parent_id: k.parentId ?? '',
    level: k.level ?? '',
    agent_id: k.agentId ?? '',
    provider: k.provider ?? '',
    model: k.model ?? '',
    is_system: false,
    config_json: k.configJson ?? '',
    metadata_json: k.metadataJson ?? '',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: k.createdAt ?? '',
    updated_at: k.updatedAt ?? '',
    deleted_at: k.deletedAt ?? '',
    workspace_id: k.workspaceId ?? '',
  };
}

function kratosCredentialToLegacy(c: KratosCredential): ChannelCredential {
  return {
    id: c.id ?? '',
    channel_id: c.channelId ?? '',
    credential_key: c.credentialKey ?? '',
    status: c.status ?? '',
    metadata_json: c.metadataJson ?? '',
    configured: Boolean(c.configured),
    masked_preview: c.maskedPreview ?? '',
    created_at: c.createdAt ?? '',
    updated_at: c.updatedAt ?? '',
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
    status: t.status ?? '',
    message: t.message ?? '',
    details,
  };
}

function inputsToKratos(creds: ChannelCredentialInput[]) {
  return creds.map((c) => ({
    credentialKey: c.credential_key,
    secret: c.secret,
    secretRef: c.secret_ref,
    status: c.status,
    metadataJson: c.metadata_json ?? '{}',
  }));
}

export async function listChannelCatalog(): Promise<ChannelTypeItem[]> {
  const data = await channelApi.ListChannelTypes({});
  return (data.items ?? []).map(kratosCatalogToLegacy);
}

export type ChannelListQuery = {
  page?: number;
  page_size?: number;
  search?: string;
  type?: string;
  status?: string;
};

export type ChannelListResult = {
  items: ChannelRow[];
  total: number;
  page: number;
  page_size: number;
};

/** Full catalog (no page params) — pickers / monitor / routing. */
export async function listChannels(): Promise<ChannelRow[]> {
  const data = await channelApi.ListChannels({
    page: undefined,
    pageSize: undefined,
    search: undefined,
    type: undefined,
    status: undefined,
  });
  return (data.items ?? []).map(kratosChannelToLegacy);
}

/** Admin registry page — server pagination. */
export async function listChannelsPaged(query: ChannelListQuery = {}): Promise<ChannelListResult> {
  const page = query.page ?? 1;
  const pageSize = query.page_size ?? 20;
  const data = await channelApi.ListChannels({
    page,
    pageSize,
    search: query.search?.trim() || undefined,
    type: query.type?.trim() || undefined,
    status: query.status?.trim() || undefined,
  });
  return {
    items: (data.items ?? []).map(kratosChannelToLegacy),
    total: Number(data.total ?? 0),
    page: Number(data.page ?? page),
    page_size: Number(data.pageSize ?? pageSize),
  };
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
    credentials: inputsToKratos(payload.credentials ?? []),
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
    credentials: inputsToKratos(payload.credentials ?? []),
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

export type WechatILinkLoginResult = {
  qrcode_data_url: string;
  qrcode_session: string;
  status: string;
};

export async function wechatILinkLogin(channelId: string): Promise<WechatILinkLoginResult> {
  const data = await channelApi.WechatILinkLogin({ channelId });
  return {
    qrcode_data_url: data.qrcodeDataUrl ?? '',
    qrcode_session: data.qrcodeSession ?? '',
    status: data.status ?? 'wait',
  };
}

export type WechatILinkPollResult = {
  status: string;
  error_message: string;
};

export async function wechatILinkPoll(channelId: string, qrcodeSession: string): Promise<WechatILinkPollResult> {
  const data = await channelApi.WechatILinkPoll({ channelId, qrcodeSession });
  return {
    status: data.status ?? 'wait',
    error_message: data.errorMessage ?? '',
  };
}

export async function listChannelCredentials(id: string): Promise<ChannelCredential[]> {
  const data = await channelApi.ListChannelCredentials({ id });
  return (data.items ?? []).map(kratosCredentialToLegacy);
}

export async function updateChannelCredentials(
  id: string,
  credentials: ChannelCredentialInput[],
): Promise<ChannelCredential[]> {
  const data = await channelApi.UpsertChannelCredentials({
    channelId: id,
    credentials: inputsToKratos(credentials),
  });
  return (data.items ?? []).map(kratosCredentialToLegacy);
}

export async function deleteChannelCredential(channelId: string, credentialKey: string): Promise<void> {
  await channelApi.DeleteChannelCredential({ channelId, credentialKey });
}

export type {
  ChannelRow,
  ChannelTypeItem,
  ChannelResourceInput,
  ChannelCredential,
  ChannelCredentialInput,
  ChannelDeliveryRow,
  ChannelTestResult,
  ChannelTurnJobRow,
} from './types';

function wireChannelTurnJob(k: {
  id?: string;
  channelId?: string;
  sessionId?: string;
  peerId?: string;
  peerKey?: string;
  idempotencyKey?: string;
  status?: string;
  previewMessageId?: string;
  contentPreview?: string;
  asyncTargetType?: string;
  asyncTargetId?: string;
  errorMessage?: string;
  startedAt?: string;
  finishedAt?: string;
  createdAt?: string;
  updatedAt?: string;
}): ChannelTurnJobRow {
  return {
    id: k.id ?? '',
    channel_id: k.channelId ?? '',
    session_id: k.sessionId ?? '',
    peer_id: k.peerId ?? '',
    peer_key: k.peerKey ?? '',
    idempotency_key: k.idempotencyKey ?? '',
    status: k.status ?? '',
    preview_message_id: k.previewMessageId ?? '',
    content_preview: k.contentPreview ?? '',
    async_target_type: k.asyncTargetType ?? '',
    async_target_id: k.asyncTargetId ?? '',
    error_message: k.errorMessage ?? '',
    started_at: k.startedAt ?? '',
    finished_at: k.finishedAt ?? '',
    created_at: k.createdAt ?? '',
    updated_at: k.updatedAt ?? '',
  };
}

export async function listChannelTurnJobs(channelId: string, limit = 30): Promise<{ items: ChannelTurnJobRow[] }> {
  const data = await channelApi.ListChannelTurnJobs({ id: channelId, limit });
  return { items: (data.items ?? []).map(wireChannelTurnJob) };
}

export async function listChannelDeliveries(id: string, limit?: number): Promise<{ items: ChannelDeliveryRow[] }> {
  const data = await channelApi.ListChannelDeliveries({ id, limit: limit ?? 50 });
  return {
    items: (data.items ?? []).map((d) => ({
      id: d.id ?? '',
      channel_id: d.channelId ?? '',
      agent_id: d.agentId ?? '',
      status: d.status ?? '',
      payload_json: d.payloadJson ?? '',
      error_message: d.errorMessage ?? '',
      created_at: d.createdAt ?? '',
      updated_at: d.updatedAt ?? '',
    })),
  };
}
