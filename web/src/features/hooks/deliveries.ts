import { createHookService } from "../../services";
import type { PaginatedResponse } from "../plugins/types";

export type HookDeliveryRow = {
  id: string;
  hook_key: string;
  hook_id: string;
  webhook_url: string;
  payload_json: string;
  status: string;
  attempt_count: number;
  max_attempts: number;
  last_error: string;
  created_at: string;
  updated_at: string;
};

export type HookDeliveryListQuery = {
  hook_key?: string;
  status?: string;
  from?: string;
  to?: string;
  page?: number;
  page_size?: number;
};

function mapRow(raw: Record<string, unknown>): HookDeliveryRow {
  const s = (k: string, alt: string) => String(raw[k] ?? raw[alt] ?? "");
  const n = (k: string, alt: string) => Number(raw[k] ?? raw[alt] ?? 0);
  return {
    id: s("id", "id"),
    hook_key: s("hook_key", "hookKey"),
    hook_id: s("hook_id", "hookId"),
    webhook_url: s("webhook_url", "webhookUrl"),
    payload_json: s("payload_json", "payloadJson"),
    status: s("status", "status"),
    attempt_count: n("attempt_count", "attemptCount"),
    max_attempts: n("max_attempts", "maxAttempts"),
    last_error: s("last_error", "lastError"),
    created_at: s("created_at", "createdAt"),
    updated_at: s("updated_at", "updatedAt")
  };
}

export async function listHookDeliveries(
  query: HookDeliveryListQuery = {}
): Promise<PaginatedResponse<HookDeliveryRow>> {
  const svc = createHookService();
  const page = query.page ?? 1;
  const pageSize = query.page_size ?? 20;
  const res = await svc.ListHookDeliveries({
    hookKey: query.hook_key?.trim() || undefined,
    status: query.status?.trim() || undefined,
    from: query.from?.trim() || undefined,
    to: query.to?.trim() || undefined,
    page,
    pageSize
  });
  return {
    items: (res.items ?? []).map((row) => mapRow(row as Record<string, unknown>)),
    total: Number(res.total ?? 0),
    page: Number(res.page ?? page),
    page_size: Number(res.pageSize ?? pageSize)
  };
}
