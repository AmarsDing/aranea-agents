import { createModelCatalogService } from "../../services/index";
import type {
  ModelCatalogPolicy,
  ModelCatalogStatus,
  ModelCatalogSyncLogEntry,
  SyncModelCatalogResponse
} from "../../services/kratos/model_catalog/v1/index";

const api = createModelCatalogService();

export type { ModelCatalogPolicy, ModelCatalogStatus, ModelCatalogSyncLogEntry };

export async function getModelCatalogStatus(): Promise<ModelCatalogStatus> {
  return api.GetModelCatalogStatus({});
}

export async function getModelCatalogPolicy(): Promise<ModelCatalogPolicy> {
  return api.GetModelCatalogPolicy({});
}

export async function updateModelCatalogPolicy(policy: ModelCatalogPolicy): Promise<ModelCatalogPolicy> {
  return api.UpdateModelCatalogPolicy({
    sourceUrl: policy.sourceUrl ?? "",
    syncPolicy: policy.syncPolicy ?? "scheduled",
    syncIntervalHours: policy.syncIntervalHours ?? 24,
    autoApply: policy.autoApply ?? "metadata_and_pricing"
  });
}

export async function syncModelCatalog(dryRun = false): Promise<SyncModelCatalogResponse> {
  return api.SyncModelCatalog({ dryRun });
}

export async function getModelCatalogRaw(): Promise<{ jsonPretty: string; bytes: number }> {
  const res = await api.GetModelCatalogRaw({});
  return { jsonPretty: res.jsonPretty ?? "", bytes: res.bytes ?? 0 };
}

export async function listModelCatalogSyncLogs(limit = 30): Promise<ModelCatalogSyncLogEntry[]> {
  const res = await api.ListModelCatalogSyncLogs({ limit });
  return res.items ?? [];
}

export async function listCatalogProviders(q = "", limit = 200, offset = 0) {
  const res = await api.ListCatalogProviders({ q, limit, offset });
  return { items: res.items ?? [], total: res.total ?? 0 };
}

export async function listCatalogModels(providerId: string, q = "", includeDeprecated = false, limit = 500, offset = 0) {
  const res = await api.ListCatalogModels({ providerId, q, includeDeprecated, limit, offset });
  return { items: res.items ?? [], total: res.total ?? 0 };
}

export async function searchCatalogRaw(q = "", limit = 200, offset = 0) {
  const res = await api.SearchCatalogRaw({ q, limit, offset });
  return { lines: res.lines ?? [], total: res.total ?? 0, offset: res.offset ?? 0 };
}

export async function previewModelCatalogMigration() {
  return api.PreviewMigration({});
}

export async function getProviderMigrationRules() {
  return api.GetProviderMigrationRules({});
}

export async function applyProviderMigration() {
  return api.ApplyProviderMigration({});
}
