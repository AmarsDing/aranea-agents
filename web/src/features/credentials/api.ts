import { createCredentialService } from '../../services';
import type { CredentialExecutionRow, CredentialRow, CredentialSecretKind, CredentialSensitivity } from './types';

function mapSecretKind(raw: string | undefined): CredentialSecretKind {
  switch (raw) {
    case 'SECRET_KIND_TOKEN':
      return 'token';
    case 'SECRET_KIND_COOKIE':
      return 'cookie';
    default:
      return 'password';
  }
}

function mapSensitivity(raw: string | undefined): CredentialSensitivity {
  return raw === 'SENSITIVITY_HIGH' ? 'high' : 'normal';
}

function toProtoKind(kind: CredentialSecretKind): string {
  switch (kind) {
    case 'token':
      return 'SECRET_KIND_TOKEN';
    case 'cookie':
      return 'SECRET_KIND_COOKIE';
    default:
      return 'SECRET_KIND_PASSWORD';
  }
}

function toProtoSensitivity(s: CredentialSensitivity | ''): string | undefined {
  if (s === 'high') return 'SENSITIVITY_HIGH';
  if (s === 'normal') return 'SENSITIVITY_NORMAL';
  return undefined;
}

function wireToCredential(raw: Record<string, unknown>): CredentialRow {
  return {
    id: String(raw.id ?? ''),
    workspace_id: String(raw.workspaceId ?? raw.workspace_id ?? ''),
    name: String(raw.name ?? ''),
    site: String(raw.site ?? ''),
    url: String(raw.url ?? ''),
    username: String(raw.username ?? ''),
    secret_kind: mapSecretKind(String(raw.secretKind ?? raw.secret_kind ?? '')),
    sensitivity: mapSensitivity(String(raw.sensitivity ?? '')),
    note: String(raw.note ?? ''),
    created_at: String(raw.createdAt ?? raw.created_at ?? ''),
    updated_at: String(raw.updatedAt ?? raw.updated_at ?? ''),
  };
}

function wireToExecution(raw: Record<string, unknown>): CredentialExecutionRow {
  return {
    id: String(raw.id ?? ''),
    credential_id: String(raw.credentialId ?? raw.credential_id ?? ''),
    site: String(raw.site ?? ''),
    action: String(raw.action ?? ''),
    actor_type: String(raw.actorType ?? raw.actor_type ?? ''),
    actor_key: String(raw.actorKey ?? raw.actor_key ?? ''),
    agent_id: String(raw.agentId ?? raw.agent_id ?? ''),
    session_id: String(raw.sessionId ?? raw.session_id ?? ''),
    verdict: String(raw.verdict ?? ''),
    reason: String(raw.reason ?? ''),
    created_at: String(raw.createdAt ?? raw.created_at ?? ''),
  };
}

export type CredentialListResult = {
  items: CredentialRow[];
  total: number;
};

export async function listCredentials(query: {
  site?: string;
  page?: number;
  pageSize?: number;
}): Promise<CredentialListResult> {
  const svc = createCredentialService();
  const res = await svc.ListCredentials({
    site: query.site?.trim() || undefined,
    page: query.page ?? 1,
    pageSize: query.pageSize ?? 20,
  });
  return {
    items: (res.items ?? []).map((row) => wireToCredential(row as Record<string, unknown>)),
    total: Number(res.total ?? 0),
  };
}

export async function createCredential(input: {
  name: string;
  site: string;
  url?: string;
  username?: string;
  secret_kind: CredentialSecretKind;
  sensitivity?: CredentialSensitivity | '';
  note?: string;
  secret: string;
}): Promise<CredentialRow> {
  const svc = createCredentialService();
  const res = await svc.CreateCredential({
    name: input.name,
    site: input.site,
    url: input.url || undefined,
    username: input.username || undefined,
    secretKind: toProtoKind(input.secret_kind) as never,
    sensitivity: toProtoSensitivity(input.sensitivity ?? '') as never,
    note: input.note || undefined,
    secret: input.secret,
  });
  return wireToCredential(res as Record<string, unknown>);
}

export async function updateCredential(
  id: string,
  patch: {
    name?: string;
    url?: string;
    username?: string;
    sensitivity?: CredentialSensitivity | '';
    note?: string;
    /** 非空 = 轮换 secret。 */
    secret?: string;
  },
): Promise<CredentialRow> {
  const svc = createCredentialService();
  const sensitivity = toProtoSensitivity(patch.sensitivity ?? '');
  const res = await svc.UpdateCredential({
    id,
    name: patch.name,
    url: patch.url,
    username: patch.username,
    sensitivity: sensitivity as never,
    note: patch.note,
    secret: patch.secret?.trim() ? patch.secret : undefined,
  });
  return wireToCredential(res as Record<string, unknown>);
}

export async function deleteCredential(id: string): Promise<void> {
  const svc = createCredentialService();
  await svc.DeleteCredential({ id });
}

export async function importBitwarden(vaultJson: string): Promise<{ imported: number; skipped: number }> {
  const svc = createCredentialService();
  const res = await svc.ImportBitwarden({ vaultJson });
  return {
    imported: Number(res.importedCount ?? 0),
    skipped: Number(res.skippedCount ?? 0),
  };
}

export type CredentialExecutionListResult = {
  items: CredentialExecutionRow[];
  total: number;
};

export async function listCredentialExecutions(query: {
  credential_id?: string;
  site?: string;
  page?: number;
  pageSize?: number;
}): Promise<CredentialExecutionListResult> {
  const svc = createCredentialService();
  const res = await svc.ListExecutions({
    credentialId: query.credential_id || undefined,
    site: query.site?.trim() || undefined,
    page: query.page ?? 1,
    pageSize: query.pageSize ?? 20,
  });
  return {
    items: (res.items ?? []).map((row) => wireToExecution(row as Record<string, unknown>)),
    total: Number(res.total ?? 0),
  };
}
