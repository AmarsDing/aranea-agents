import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { confirm } from '../../composables/useAppConfirm';
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
import { isForbiddenApiError } from '../../utils/kratosError';

export function useCredentialsPage() {
  const { t } = useI18n();
  const $q = useQuasar();

  const tab = ref<'credentials' | 'executions'>('credentials');

  // ---- 凭据列表 ----
  const rows = ref<CredentialRow[]>([]);
  const loading = ref(false);
  const loaded = ref(false);
  const error = ref('');
  /** 列表接口 403：页面据此渲染 forbidden 态（无重试）。 */
  const forbidden = ref(false);
  const siteFilter = ref('');
  const sensitivityFilter = ref('');
  const page = ref(1);
  const pageSize = ref(24);

  // shortcut: 一次读回至多 200 条（接口上限），在前端筛选/分页，关键数才准；超过 200 条时改回服务端分页 + 计数
  const filteredRows = computed(() => {
    const q = siteFilter.value.trim().toLowerCase();
    return rows.value.filter((row) => {
      if (sensitivityFilter.value && row.sensitivity !== sensitivityFilter.value) return false;
      return !q || [row.site, row.name, row.username].some((v) => v.toLowerCase().includes(q));
    });
  });
  const total = computed(() => filteredRows.value.length);
  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const pagedRows = computed(() =>
    filteredRows.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value),
  );
  const highCount = computed(() => rows.value.filter((row) => row.sensitivity === 'high').length);

  async function loadRows() {
    error.value = '';
    forbidden.value = false;
    loading.value = true;
    try {
      const res = await listCredentials({ page: 1, pageSize: 200 });
      rows.value = res.items;
      loaded.value = true;
    } catch (e) {
      forbidden.value = isForbiddenApiError(e);
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  watch([siteFilter, sensitivityFilter, pageSize], () => {
    page.value = 1;
  });
  watch(pageMax, (max) => {
    if (page.value > max) page.value = max;
  });

  // ---- 检视面板：选中凭据的元数据 + 它最近的执行 ----
  const inspectedId = ref('');
  const inspected = computed(() => rows.value.find((row) => row.id === inspectedId.value) ?? null);
  const inspectorOpen = computed({
    get: () => inspected.value !== null,
    set: (open: boolean) => {
      if (!open) inspectedId.value = '';
    },
  });
  const inspectedExecs = ref<CredentialExecutionRow[]>([]);
  const inspectedExecsLoading = ref(false);

  async function inspect(row: CredentialRow) {
    inspectedId.value = row.id;
    inspectedExecs.value = [];
    inspectedExecsLoading.value = true;
    try {
      const res = await listCredentialExecutions({ credential_id: row.id, page: 1, pageSize: 8 });
      if (inspectedId.value === row.id) inspectedExecs.value = res.items;
    } catch {
      // 审计要 audit.read；没有权限或读失败时这一块显示为空，不影响凭据本身
    } finally {
      inspectedExecsLoading.value = false;
    }
  }

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

  async function confirmDelete(row: CredentialRow) {
    const ok = await confirm({
      title: t('credentialsPage.confirmDeleteTitle'),
      message: t('connect.credentials.deleteMessage', { name: row.name, site: row.site }),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await deleteCredential(row.id);
      if (inspectedId.value === row.id) inspectedId.value = '';
      $q.notify({ type: 'positive', message: t('credentialsPage.notifyDeleted') });
      await loadRows();
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    }
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
  /** 执行审计接口 403：页面据此渲染 forbidden 态（无重试）。 */
  const execForbidden = ref(false);
  const execSiteFilter = ref('');
  const execPage = ref(1);
  const execPageSize = ref(20);
  const execPageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, execTotal.value) / execPageSize.value)));

  async function loadExecutions() {
    execError.value = '';
    execForbidden.value = false;
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
      execForbidden.value = isForbiddenApiError(e);
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
    loaded,
    error,
    forbidden,
    siteFilter,
    sensitivityFilter,
    highCount,
    page,
    pageSize,
    pageMax,
    pagedRows,
    loadRows,
    inspected,
    inspectorOpen,
    inspectedExecs,
    inspectedExecsLoading,
    inspect,
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
    execForbidden,
    execSiteFilter,
    execPage,
    execPageSize,
    execPageMax,
    loadExecutions,
  };
}
