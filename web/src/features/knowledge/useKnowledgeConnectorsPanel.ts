import { computed, onScopeDispose, reactive, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useSourceSyncStore } from '../../stores/knowledge/sourceSync';
import type { SourceConnector } from './types';
import { emptySourceConnectorForm } from './sourceSyncUi';
import type { SourceConnectorForm, SourceCredentialForm, SourceSyncPanelView } from './sourceSyncUi';
import { useSourceSyncWs } from './useSourceSyncWs';

export function useKnowledgeConnectorsPanel(active: () => boolean) {
  const store = useSourceSyncStore();
  const { t } = useI18n();
  const $q = useQuasar();
  const dialogOpen = ref(false);
  const editing = ref<SourceConnector | null>(null);
  const form = reactive(emptySourceConnectorForm());
  const credForm = reactive<Record<string, SourceCredentialForm>>({});

  // Initialise before the child renders, independently of network success.
  watch(() => store.connectors, (rows) => {
    for (const row of rows) credForm[row.id] ??= { key: 'default', secret: '' };
    for (const id of Object.keys(credForm)) if (!rows.some((row) => row.id === id)) delete credForm[id];
  }, { immediate: true, flush: 'sync' });

  const ws = useSourceSyncWs(() => '', () => { if (active()) void store.refresh(); });
  ws.disconnect();
  watch(active, (visible) => {
    if (visible) { void store.refresh(); ws.reconnect(); }
    else {
      ws.disconnect();
      dialogOpen.value = false;
      for (const entry of Object.values(credForm)) entry.secret = '';
    }
  }, { immediate: true });
  onScopeDispose(() => { for (const entry of Object.values(credForm)) entry.secret = ''; });

  function openCreate() {
    editing.value = null;
    Object.assign(form, emptySourceConnectorForm());
    dialogOpen.value = true;
  }

  function openEdit(connector: SourceConnector) {
    if (store.busy[connector.id]) return;
    let config: Record<string, unknown> = {};
    try {
      const parsed: unknown = JSON.parse(connector.config_json || '{}');
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('Invalid connector configuration');
      config = parsed as Record<string, unknown>;
    } catch {
      store.actionError = t('sourceSync.configInvalid');
      return;
    }
    editing.value = connector;
    Object.assign(form, emptySourceConnectorForm(), {
      kind: connector.kind, name: connector.name, enabled: connector.enabled,
      interval_seconds: connector.interval_seconds,
      max_items_per_run: connector.max_items_per_run,
      max_deletes_per_run: connector.max_deletes_per_run,
      max_fetch_bytes_per_run: connector.max_fetch_bytes_per_run,
      root: typeof config.root === 'string' ? config.root : '',
      credential_key: typeof config.credential_key === 'string' ? config.credential_key : 'default',
      query: typeof config.query === 'string' ? config.query : '',
    });
    dialogOpen.value = true;
  }

  async function save() {
    if (store.busy[editing.value?.id || '$create']) return;
    const config = form.kind === 'shared_dir' ? { root: form.root.trim() }
      : { credential_key: form.credential_key.trim(), ...(form.query.trim() ? { query: form.query.trim() } : {}) };
    if (!(form.kind === 'shared_dir' ? form.root.trim() : form.credential_key.trim()) ||
      ![form.interval_seconds, form.max_items_per_run, form.max_deletes_per_run, form.max_fetch_bytes_per_run].every((value) => Number.isSafeInteger(value) && value > 0)) {
      store.actionError = t('sourceSync.invalidForm');
      return;
    }
    const ok = await store.save(editing.value?.id ?? null, {
      ...(editing.value ? {} : { kind: form.kind }), name: form.name.trim(), config_json: JSON.stringify(config),
      interval_seconds: Math.max(900, form.interval_seconds), enabled: form.enabled,
      max_items_per_run: form.max_items_per_run, max_deletes_per_run: form.max_deletes_per_run,
      max_fetch_bytes_per_run: form.max_fetch_bytes_per_run,
    });
    if (ok) dialogOpen.value = false;
  }

  function remove(connector: SourceConnector) {
    if (store.busy[connector.id]) return;
    $q.dialog({ title: t('sourceSync.remove'), message: t('sourceSync.removeConfirm'), cancel: true, persistent: true })
      .onOk(() => { void store.remove(connector.id); });
  }

  function removeCredential(id: string, key: string) {
    if (store.busy[id]) return;
    $q.dialog({ title: t('sourceSync.credDelete'), message: t('sourceSync.credDeleteConfirm'), cancel: true, persistent: true })
      .onOk(() => { void store.removeCredential(id, key); });
  }

  async function saveCredential(id: string) {
    const entry = credForm[id];
    if (!entry || store.busy[id]) return;
    const submitted = entry.secret;
    if (await store.saveCredential(id, entry.key, submitted)) {
      // A newer draft typed during a request belongs to the user.
      if (entry.secret === submitted) entry.secret = '';
    }
  }

  const view = computed<SourceSyncPanelView>(() => ({
    connectors: store.connectors, loading: store.loading, loaded: store.loaded,
    loadError: store.loadError, actionError: store.actionError, busy: store.busy,
    runsByConn: store.runsByConn, missingByConn: store.missingByConn, credsByConn: store.credsByConn,
    detailLoading: store.detailLoading, detailErrors: store.detailErrors,
    dialogOpen: dialogOpen.value, editing: editing.value, form, credForm,
  }));
  const events = {
    refresh: store.refresh, openCreate, openEdit, save, remove,
    trigger: (connector: SourceConnector) => store.trigger(connector.id),
    loadRuns: store.loadRuns, loadMissing: store.loadMissing, loadCredentials: store.loadCredentials,
    saveCredential, removeCredential,
    close: () => { if (!store.busy[editing.value?.id || '$create']) dialogOpen.value = false; },
    updateForm: (patch: Partial<SourceConnectorForm>) => { Object.assign(form, patch); },
    updateCredential: (id: string, patch: Partial<SourceCredentialForm>) => { if (credForm[id]) Object.assign(credForm[id], patch); },
  };
  return { view, events };
}
