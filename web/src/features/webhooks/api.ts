import { createWebhookService } from '../../services';
import type { WebhookRow } from './types';

function wireToWebhook(raw: Record<string, unknown>): WebhookRow {
  return {
    id: String(raw.id ?? ''),
    name: String(raw.name ?? ''),
    url: String(raw.url ?? ''),
    event_types_json: String(raw.eventTypesJson ?? raw.event_types_json ?? '[]'),
    secret: String(raw.secret ?? ''),
    headers: (raw.headers ?? {}) as Record<string, string>,
    enabled: Boolean(raw.enabled),
    created_at: String(raw.createdAt ?? raw.created_at ?? ''),
    updated_at: String(raw.updatedAt ?? raw.updated_at ?? ''),
  };
}

export type WebhookListQuery = {
  page?: number;
  page_size?: number;
  search?: string;
};

export type WebhookListResult = {
  items: WebhookRow[];
  total: number;
  page: number;
  page_size: number;
};

/** Full catalog (no page params). */
export async function listWebhooks(): Promise<WebhookRow[]> {
  const svc = createWebhookService();
  const res = await svc.ListWebhooks({});
  return (res.items ?? []).map((row) => wireToWebhook(row as Record<string, unknown>));
}

/** Admin registry page — server pagination. */
export async function listWebhooksPaged(query: WebhookListQuery = {}): Promise<WebhookListResult> {
  const svc = createWebhookService();
  const page = query.page ?? 1;
  const pageSize = query.page_size ?? 20;
  const res = await svc.ListWebhooks({
    page,
    pageSize,
    search: query.search?.trim() || undefined,
  });
  return {
    items: (res.items ?? []).map((row) => wireToWebhook(row as Record<string, unknown>)),
    total: Number(res.total ?? 0),
    page: Number(res.page ?? page),
    page_size: Number(res.pageSize ?? pageSize),
  };
}

export async function createWebhook(input: {
  name: string;
  url: string;
  event_types_json?: string;
  secret?: string;
  headers?: Record<string, string>;
  enabled?: boolean;
}): Promise<WebhookRow> {
  const svc = createWebhookService();
  const row = await svc.CreateWebhook({
    name: input.name,
    url: input.url,
    eventTypesJson: input.event_types_json ?? '[]',
    secret: input.secret ?? '',
    headers: input.headers ?? {},
    enabled: input.enabled ?? true,
  });
  return wireToWebhook(row as Record<string, unknown>);
}

export async function updateWebhook(
  id: string,
  patch: {
    name?: string;
    url?: string;
    event_types_json?: string;
    secret?: string;
    headers?: Record<string, string>;
    enabled?: boolean;
  },
): Promise<WebhookRow> {
  const svc = createWebhookService();
  const row = await svc.UpdateWebhook({
    id,
    name: patch.name,
    url: patch.url,
    eventTypesJson: patch.event_types_json,
    secret: patch.secret,
    headers: patch.headers,
    enabled: patch.enabled,
  });
  return wireToWebhook(row as Record<string, unknown>);
}

export async function deleteWebhook(id: string): Promise<void> {
  const svc = createWebhookService();
  await svc.DeleteWebhook({ id });
}

export type WebhookTestResult = {
  success: boolean;
  status_code: number;
  error: string;
  duration_ms: number;
};

/** Send one synthetic webhook.test event to verify the stored config end-to-end. */
export async function testWebhook(id: string): Promise<WebhookTestResult> {
  const svc = createWebhookService();
  const res = await svc.TestWebhook({ id });
  return {
    success: Boolean(res.success),
    status_code: Number(res.statusCode ?? 0),
    error: String(res.error ?? ''),
    duration_ms: Number(res.durationMs ?? 0),
  };
}
