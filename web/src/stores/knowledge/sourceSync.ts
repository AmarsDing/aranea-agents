import { defineStore } from 'pinia';
import { reactive, ref } from 'vue';
import {
  createSourceConnector, deleteSourceConnector, deleteSourceCredential,
  listSourceConnectors, listSourceCredentials, listSourceSyncItems, listSourceSyncRuns,
  triggerSourceSync, updateSourceConnector, upsertSourceCredential,
} from '../../features/knowledge/api';
import type {
  SourceConnector, SourceConnectorInput, SourceCredentialView, SourceSyncItem, SourceSyncRun,
} from '../../features/knowledge/types';

export const useSourceSyncStore = defineStore('knowledge-source-sync', () => {
  const connectors = ref<SourceConnector[]>([]);
  const loading = ref(false);
  const loaded = ref(false);
  const loadError = ref('');
  const actionError = ref('');
  const busy = reactive<Record<string, string>>({});
  const runsByConn = reactive<Record<string, SourceSyncRun[]>>({});
  const missingByConn = reactive<Record<string, SourceSyncItem[]>>({});
  const credsByConn = reactive<Record<string, SourceCredentialView[]>>({});
  const detailLoading = reactive<Record<string, boolean>>({});
  const detailErrors = reactive<Record<string, string>>({});
  const versions = new Map<string, number>();
  let listVersion = 0;
  const errorMessage = (reason: unknown) => reason instanceof Error ? reason.message : String(reason);

  async function refresh() {
    const version = ++listVersion;
    loading.value = true;
    loadError.value = '';
    try {
      const result = await listSourceConnectors();
      if (version !== listVersion) return;
      connectors.value = result;
      loaded.value = true;
    } catch (reason) {
      if (version === listVersion) loadError.value = errorMessage(reason);
    } finally {
      if (version === listVersion) loading.value = false;
    }
  }

  async function loadDetail<T>(id: string, kind: string, request: () => Promise<T>, apply: (value: T) => void) {
    const key = `${id}:${kind}`;
    const version = (versions.get(key) ?? 0) + 1;
    versions.set(key, version);
    detailLoading[key] = true;
    detailErrors[key] = '';
    try {
      const result = await request();
      if (versions.get(key) === version) apply(result);
    } catch (reason) {
      if (versions.get(key) === version) detailErrors[key] = errorMessage(reason);
    } finally {
      if (versions.get(key) === version) detailLoading[key] = false;
    }
  }

  const loadRuns = (id: string) => loadDetail(id, 'runs', () => listSourceSyncRuns(id), (rows) => { runsByConn[id] = rows; });
  const loadMissing = (id: string) => loadDetail(id, 'missing', () => listSourceSyncItems(id, 'missing'), (rows) => { missingByConn[id] = rows; });
  const loadCredentials = (id: string) => loadDetail(id, 'credentials', () => listSourceCredentials(id), (rows) => { credsByConn[id] = rows; });

  // One mutation per connector, including edits, sync, delete and credential writes.
  async function change(id: string, operation: string, request: () => Promise<unknown>): Promise<boolean> {
    if (busy[id]) return false;
    busy[id] = operation;
    actionError.value = '';
    try {
      await request();
      return true;
    } catch (reason) {
      actionError.value = errorMessage(reason);
      return false;
    } finally {
      delete busy[id];
    }
  }

  async function save(id: string | null, input: SourceConnectorInput) {
    const ok = await change(id || '$create', 'save', () => id ? updateSourceConnector(id, input) : createSourceConnector(input));
    if (ok) await refresh();
    return ok;
  }

  async function trigger(id: string) {
    if (connectors.value.find((item) => item.id === id)?.sync_state === 'running') return false;
    const ok = await change(id, 'sync', () => triggerSourceSync(id));
    if (ok) await refresh();
    return ok;
  }

  async function remove(id: string) {
    const ok = await change(id, 'delete', () => deleteSourceConnector(id));
    if (!ok) return false;
    connectors.value = connectors.value.filter((item) => item.id !== id);
    for (const kind of ['runs', 'missing', 'credentials']) {
      const key = `${id}:${kind}`;
      versions.set(key, (versions.get(key) ?? 0) + 1);
      delete detailLoading[key];
      delete detailErrors[key];
    }
    delete runsByConn[id];
    delete missingByConn[id];
    delete credsByConn[id];
    await refresh();
    return true;
  }

  async function saveCredential(id: string, credentialKey: string, secret: string) {
    if (!credentialKey.trim() || !secret) return false;
    const ok = await change(id, 'credential', () => upsertSourceCredential(id, { credential_key: credentialKey.trim(), secret }));
    if (ok) await Promise.all([loadCredentials(id), refresh()]);
    return ok;
  }

  async function removeCredential(id: string, credentialKey: string) {
    const ok = await change(id, 'credential-delete', () => deleteSourceCredential(id, credentialKey));
    if (ok) await loadCredentials(id);
    return ok;
  }

  return {
    connectors, loading, loaded, loadError, actionError, busy,
    runsByConn, missingByConn, credsByConn, detailLoading, detailErrors,
    refresh, loadRuns, loadMissing, loadCredentials, save, trigger, remove, saveCredential, removeCredential,
  };
});
