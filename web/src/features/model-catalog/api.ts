import { createModelCatalogService } from '../../services/index';
import { normalizeCatalogSearchBlocks } from './catalogSearchUtils';
import { normalizeCatalogModelSummary, normalizeCatalogProviderSummary } from './catalogWire';
import type {
  ModelCatalogPolicy,
  ModelCatalogStatus,
  ModelCatalogSyncLogEntry,
  SyncModelCatalogResponse,
} from '../../services/kratos/model_catalog/v1/index';

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
    sourceUrl: policy.sourceUrl ?? '',
    syncPolicy: policy.syncPolicy ?? 'scheduled',
    syncIntervalHours: policy.syncIntervalHours ?? 24,
    autoApply: policy.autoApply ?? 'metadata_and_pricing',
  });
}

export async function syncModelCatalog(dryRun = false): Promise<SyncModelCatalogResponse> {
  return api.SyncModelCatalog({ dryRun });
}

export async function getModelCatalogRaw(): Promise<{ jsonPretty: string; bytes: number }> {
  const res = await api.GetModelCatalogRaw({});
  return { jsonPretty: res.jsonPretty ?? '', bytes: res.bytes ?? 0 };
}

export async function listModelCatalogSyncLogs(limit = 30): Promise<ModelCatalogSyncLogEntry[]> {
  const res = await api.ListModelCatalogSyncLogs({ limit });
  return res.items ?? [];
}

export async function listCatalogProviders(q = '', limit = 200, offset = 0) {
  const res = await api.ListCatalogProviders({ q, limit, offset });
  return {
    items: (res.items ?? []).map(normalizeCatalogProviderSummary),
    total: res.total ?? 0,
  };
}

export async function listCatalogModels(
  providerId: string,
  q = '',
  includeDeprecated = false,
  limit = 500,
  offset = 0,
) {
  const res = await api.ListCatalogModels({ providerId, q, includeDeprecated, limit, offset });
  return {
    items: (res.items ?? []).map(normalizeCatalogModelSummary),
    total: res.total ?? 0,
  };
}

/** Paginated provider browse for empty search — builds full provider JSON client-side. */
export async function browseCatalogProviderBlocks(offset = 0, limit = 1) {
  const res = await api.ListCatalogProviders({ q: '', limit, offset });
  const blocks: string[] = [];
  for (const p of res.items ?? []) {
    const pid = (p.id ?? '').trim();
    if (!pid) continue;
    const modelsRes = await api.ListCatalogModels({
      providerId: pid,
      q: '',
      includeDeprecated: true,
      limit: 500,
      offset: 0,
    });
    const models: Record<string, unknown> = {};
    for (const m of modelsRes.items ?? []) {
      const mid = (m.id ?? '').trim();
      if (mid) models[mid] = m;
    }
    blocks.push(
      JSON.stringify(
        {
          id: p.id,
          name: p.name,
          env: p.env,
          npm: p.npm,
          api: p.api,
          doc: p.doc,
          models,
        },
        null,
        2,
      ),
    );
  }
  return {
    blocks,
    total: res.total ?? 0,
    offset,
    truncated: false,
    legacyLineMode: false,
  };
}

export async function searchCatalogBlocks(q = '', limit = 10, offset = 0) {
  const query = q.trim();
  if (!query) {
    return browseCatalogProviderBlocks(offset, limit);
  }
  const res = await api.SearchCatalogRaw({ q: query, limit, offset });
  const raw = res.lines ?? [];
  const { blocks, legacyLineMode } = normalizeCatalogSearchBlocks(raw);
  return {
    blocks,
    total: res.total ?? 0,
    offset: res.offset ?? 0,
    truncated: Boolean(res.truncated),
    legacyLineMode,
  };
}

/** @deprecated use searchCatalogBlocks */
export async function searchCatalogRaw(q = '', limit = 10, offset = 0) {
  const res = await searchCatalogBlocks(q, limit, offset);
  return {
    lines: res.blocks,
    total: res.total,
    offset: res.offset,
    truncated: res.truncated,
    legacyLineMode: res.legacyLineMode,
  };
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
