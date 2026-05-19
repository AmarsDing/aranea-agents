import { createHookService } from "../../services";
import type { HookRow } from "./types";
import { parseHookConfig, serializeHookConfig, type HookRuleConfig } from "./types";

function wireToHook(raw: Record<string, unknown>): HookRow {
  return {
    id: String(raw.id ?? ""),
    key: String(raw.key ?? ""),
    name: String(raw.name ?? ""),
    description: String(raw.description ?? ""),
    status: String(raw.status ?? "active"),
    enabled: Boolean(raw.enabled),
    sort_order: Number(raw.sortOrder ?? raw.sort_order ?? 0),
    config_json: String(raw.configJson ?? raw.config_json ?? "{}"),
    metadata_json: String(raw.metadataJson ?? raw.metadata_json ?? "{}"),
    created_at: String(raw.createdAt ?? raw.created_at ?? ""),
    updated_at: String(raw.updatedAt ?? raw.updated_at ?? "")
  };
}

export async function listHooks(): Promise<HookRow[]> {
  const svc = createHookService();
  const res = await svc.ListHooks({});
  return (res.items ?? []).map((row) => wireToHook(row as Record<string, unknown>));
}

export async function createHook(input: {
  key: string;
  name: string;
  description?: string;
  enabled?: boolean;
  sort_order?: number;
  rule: HookRuleConfig;
}): Promise<HookRow> {
  const svc = createHookService();
  const row = await svc.CreateHook({
    key: input.key,
    name: input.name,
    description: input.description ?? "",
    status: "active",
    enabled: input.enabled ?? true,
    sortOrder: input.sort_order ?? 0,
    configJson: serializeHookConfig(input.rule),
    metadataJson: "{}"
  });
  return wireToHook(row as Record<string, unknown>);
}

export async function updateHook(
  id: string,
  patch: Partial<Pick<HookRow, "key" | "name" | "description" | "enabled" | "sort_order" | "status">> & {
    rule?: HookRuleConfig;
  }
): Promise<HookRow> {
  const svc = createHookService();
  const cur = await svc.GetHook({ id });
  const merged = {
    id: cur.id,
    key: patch.key ?? cur.key,
    name: patch.name ?? cur.name,
    description: patch.description ?? cur.description,
    status: patch.status ?? cur.status,
    enabled: patch.enabled ?? cur.enabled,
    sortOrder: patch.sort_order ?? cur.sortOrder,
    configJson: patch.rule ? serializeHookConfig(patch.rule) : cur.configJson,
    metadataJson: cur.metadataJson,
    createdAt: cur.createdAt,
    updatedAt: cur.updatedAt,
    deletedAt: cur.deletedAt
  };
  const row = await svc.UpdateHook({ id, hook: merged });
  return wireToHook(row as Record<string, unknown>);
}

export async function deleteHook(id: string): Promise<void> {
  const svc = createHookService();
  await svc.DeleteHook({ id });
}

export function hookRuleFromRow(row: HookRow): HookRuleConfig {
  return parseHookConfig(row.config_json);
}
