import { createPackService } from '../../services';
import type {
  ExportPackResponse,
  ImportPackResponse,
  ValidatePackResponse,
  ImportFailure,
  ConflictItem,
} from '../../services/kratos/pack/v1/index';

export type { ImportFailure, ConflictItem };

function mapExportResponse(raw: ExportPackResponse) {
  return {
    data: String(raw.data ?? ''),
    name: String(raw.name ?? ''),
    kind: String(raw.kind ?? ''),
  };
}

export type ExportPackResult = ReturnType<typeof mapExportResponse>;

function mapImportFailure(raw: ImportFailure) {
  return {
    entity_type: String(raw.entityType ?? ''),
    key: String(raw.key ?? ''),
    reason: String(raw.reason ?? ''),
  };
}

export type ImportFailureRow = ReturnType<typeof mapImportFailure>;

function mapImportResponse(raw: ImportPackResponse) {
  return {
    taxonomy_nodes: Number(raw.orgNodes ?? 0),
    agents_created: Number(raw.agentsCreated ?? 0),
    agents_updated: Number(raw.agentsUpdated ?? 0),
    agents_skipped: Number(raw.agentsSkipped ?? 0),
    graphs_created: Number(raw.graphsCreated ?? 0),
    teams_created: Number(raw.teamsCreated ?? 0),
    teams_updated: Number(raw.teamsUpdated ?? 0),
    teams_skipped: Number(raw.teamsSkipped ?? 0),
    conflict_strategy: String(raw.conflictStrategy ?? ''),
    failures: (raw.failures ?? []).map(mapImportFailure),
  };
}

export type ImportPackResult = ReturnType<typeof mapImportResponse>;

function mapConflictItem(raw: ConflictItem) {
  return {
    entity_type: String(raw.entityType ?? ''),
    key: String(raw.key ?? ''),
  };
}

export type ConflictItemRow = ReturnType<typeof mapConflictItem>;

function mapValidateResponse(raw: ValidatePackResponse) {
  return {
    valid: Boolean(raw.valid),
    errors: (raw.errors ?? []) as string[],
    warnings: (raw.warnings ?? []) as string[],
    missing_skills: (raw.missingSkills ?? []) as string[],
    missing_func_refs: (raw.missingFuncRefs ?? []) as string[],
    conflicts: (raw.conflicts ?? []).map(mapConflictItem),
  };
}

export type ValidatePackResult = ReturnType<typeof mapValidateResponse>;

export async function exportPack(kind: string, ref: string): Promise<ExportPackResult> {
  const svc = createPackService();
  const res = await svc.ExportPack({ kind, ref });
  return mapExportResponse(res);
}

export async function importPack(data: string, conflictStrategy: string = 'skip'): Promise<ImportPackResult> {
  const svc = createPackService();
  const res = await svc.ImportPack({ data, conflictStrategy });
  return mapImportResponse(res);
}

export async function validatePack(data: string): Promise<ValidatePackResult> {
  const svc = createPackService();
  const res = await svc.ValidatePack({ data });
  return mapValidateResponse(res);
}
