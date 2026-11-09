import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import {
  createCredential,
  deleteCredential,
  importBitwarden,
  listCredentialExecutions,
  listCredentials,
  updateCredential,
} from './api';
import {
  emptyCredentialForm,
  type CredentialExecutionRow,
  type CredentialFormModel,
  type CredentialRow,
} from './types';

export function useCredentialsPage() {
  const { t } = useI18n();
  const $q = useQuasar();

  const tab = ref<'credentials' | 'executions'>('credentials');

  // ---- 凭据列表 ----
  const rows = ref<CredentialRow[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');
  const siteFilter = ref('');
  const page = ref(1);
  const pageSize = ref(20);
  const pageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, total.value) / pageSize.value)));

  async function loadRows() {
    error.value = '';
    loading.value = true;
    try {
      const res = await listCredentials({ site: siteFilter.value, page: page.value, pageSize: pageSize.value });
      rows.value = res.items;
      total.value = res.total;
      if (page.value > pageMax.value) page.value = pageMax.value;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  let skipNextPageWatch = false;
  watch(siteFilter, () => {
    if (page.value !== 1) {
      skipNextPageWatch = true;
      page.value = 1;
    }
    void loadRows();
  });
  watch([page, pageSize], () => {
    if (skipNextPageWatch) {
      skipNextPageWatch = false;
      return;
    }
    void loadRows();
  });

  // ---- 新建 / 编辑 ----
  const editorOpen = ref(false);
  const editingId = ref('');
  const saving = ref(false);
  const form = ref<CredentialFormModel>(emptyCredentialForm());

  function openCreate() {
    editingId.value = '';
    form.value = emptyCredentialForm();
    editorOpen.value = true;
  }

  function openEdit(row: CredentialRow) {
    editingId.value = row.id;
    form.value = {
      name: row.name,
      site: row.site,
      url: row.url,
      username: row.username,
      secret_kind: row.secret_kind,
      sensitivity: row.sensitivity,
      note: row.note,
      secret: '',
    };
    editorOpen.value = true;
  }

  async function saveCredential() {
    const f = form.value;
    if (!f.name.trim() || !f.site.trim()) {
      $q.notify({ type: 'warning', message: t('credentialsPage.notifyRequired') });
      return;
    }
    if (!editingId.value && !f.secret.trim()) {
      $q.notify({ type: 'warning', message: t('credentialsPage.notifySecretRequired') });
      return;
    }
    saving.value = true;
    try {
      if (editingId.value) {
        await updateCredential(editingId.value, {
          name: f.name.trim(),
          url: f.url.trim(),
          username: f.username.trim(),
          sensitivity: f.sensitivity,
          note: f.note.trim(),
          secret: f.secret,
        });
      } else {
        await createCredential({
          name: f.name.trim(),
          site: f.site.trim(),
          url: f.url.trim(),
          username: f.username.trim(),
          secret_kind: f.secret_kind,
          sensitivity: f.sensitivity,
          note: f.note.trim(),
          secret: f.secret,
        });
      }
      editorOpen.value = false;
      $q.notify({ type: 'positive', message: t('credentialsPage.notifySaved') });
      await loadRows();
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      saving.value = false;
    }
  }

  function confirmDelete(row: CredentialRow) {
    $q.dialog({
      title: t('credentialsPage.confirmDeleteTitle'),
      message: t('credentialsPage.confirmDeleteMessage', { name: row.name, site: row.site }),
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      try {
        await deleteCredential(row.id);
        $q.notify({ type: 'positive', message: t('credentialsPage.notifyDeleted') });
        await loadRows();
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
      }
    });
  }

  // ---- Bitwarden 导入 ----
  const importOpen = ref(false);
  const importJson = ref('');
  const importing = ref(false);

  function openImport() {
    importJson.value = '';
    importOpen.value = true;
  }

  async function runImport() {
    if (!importJson.value.trim()) {
      $q.notify({ type: 'warning', message: t('credentialsPage.notifyImportRequired') });
      return;
    }
    importing.value = true;
    try {
      const res = await importBitwarden(importJson.value);
      importOpen.value = false;
      $q.notify({
        type: 'positive',
        message: t('credentialsPage.notifyImported', { imported: res.imported, skipped: res.skipped }),
      });
      await loadRows();
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      importing.value = false;
    }
  }

  // ---- 执行审计 ----
  const execRows = ref<CredentialExecutionRow[]>([]);
  const execTotal = ref(0);
  const execLoading = ref(false);
  const execError = ref('');
  const execSiteFilter = ref('');
  const execPage = ref(1);
  const execPageSize = ref(20);
  const execPageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, execTotal.value) / execPageSize.value)));

  async function loadExecutions() {
    execError.value = '';
    execLoading.value = true;
    try {
      const res = await listCredentialExecutions({
        site: execSiteFilter.value,
        page: execPage.value,
        pageSize: execPageSize.value,
      });
      execRows.value = res.items;
      execTotal.value = res.total;
      if (execPage.value > execPageMax.value) execPage.value = execPageMax.value;
    } catch (e) {
      execError.value = e instanceof Error ? e.message : String(e);
    } finally {
      execLoading.value = false;
    }
  }

  let skipNextExecPageWatch = false;
  watch(execSiteFilter, () => {
    if (execPage.value !== 1) {
      skipNextExecPageWatch = true;
      execPage.value = 1;
    }
    void loadExecutions();
  });
  watch([execPage, execPageSize], () => {
    if (skipNextExecPageWatch) {
      skipNextExecPageWatch = false;
      return;
    }
    void loadExecutions();
  });
  watch(tab, (v) => {
    if (v === 'executions') void loadExecutions();
  });

  onMounted(loadRows);

  return {
    t,
    tab,
    rows,
    total,
    loading,
    error,
    siteFilter,
    page,
    pageSize,
    pageMax,
    loadRows,
    editorOpen,
    editingId,
    saving,
    form,
    openCreate,
    openEdit,
    saveCredential,
    confirmDelete,
    importOpen,
    importJson,
    importing,
    openImport,
    runImport,
    execRows,
    execTotal,
    execLoading,
    execError,
    execSiteFilter,
    execPage,
    execPageSize,
    execPageMax,
    loadExecutions,
  };
}
