import { createGatewayService } from "../../services";
import type { WebhookRow } from "./types";

function wireToWebhook(raw: Record<string, unknown>): WebhookRow {
  return {
    id: String(raw.id ?? ""),
    name: String(raw.name ?? ""),
    url: String(raw.url ?? ""),
    event_types_json: String(raw.eventTypesJson ?? raw.event_types_json ?? "[]"),
    secret: String(raw.secret ?? ""),
    headers: (raw.headers ?? {}) as Record<string, string>,
    enabled: Boolean(raw.enabled),
    created_at: String(raw.createdAt ?? raw.created_at ?? ""),
    updated_at: String(raw.updatedAt ?? raw.updated_at ?? "")
  };
}

export async function listWebhooks(): Promise<WebhookRow[]> {
  const svc = createGatewayService();
  const res = await svc.ListWebhooks({});
  return (res.items ?? []).map((row) => wireToWebhook(row as Record<string, unknown>));
}

export async function createWebhook(input: {
  name: string;
  url: string;
  event_types_json?: string;
  secret?: string;
  headers?: Record<string, string>;
  enabled?: boolean;
}): Promise<WebhookRow> {
  const svc = createGatewayService();
  const row = await svc.CreateWebhook({
    name: input.name,
    url: input.url,
    eventTypesJson: input.event_types_json ?? "[]",
    secret: input.secret ?? "",
    headers: input.headers ?? {},
    enabled: input.enabled ?? true
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
  }
): Promise<WebhookRow> {
  const svc = createGatewayService();
  const row = await svc.UpdateWebhook({
    id,
    name: patch.name,
    url: patch.url,
    eventTypesJson: patch.event_types_json,
    secret: patch.secret,
    headers: patch.headers,
    enabled: patch.enabled
  });
  return wireToWebhook(row as Record<string, unknown>);
}

export async function deleteWebhook(id: string): Promise<void> {
  const svc = createGatewayService();
  await svc.DeleteWebhook({ id });
}
