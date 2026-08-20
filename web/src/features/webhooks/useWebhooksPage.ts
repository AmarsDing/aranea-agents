import { computed, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { WEBHOOK_SECRET_MASK, type WebhookRow } from './types';
import { testWebhook } from './api';
import { useWebhooksStore } from '../../stores/webhooks';

interface WebhookDialogExposed {
  reset: () => void;
  fill: (row: WebhookRow) => void;
  getPayload: () => Partial<WebhookRow> | null;
}

export function useWebhooksPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const webhooksStore = useWebhooksStore();
  const { webhooks: storeRows, total, loading: storeLoading } = storeToRefs(webhooksStore);
  const loading = storeLoading;
  const saving = ref(false);
  const error = ref('');
  const search = ref('');
  const page = ref(1);
  const pageSize = ref(20);
  const editorOpen = ref(false);
  const editingId = ref('');
  const busyId = ref('');
  const testingId = ref('');
  const dialogRef = ref<WebhookDialogExposed | null>(null);

  const filteredRows = computed(() => storeRows.value);
  const pagedRows = computed(() => storeRows.value);
  const pageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, total.value) / pageSize.value)));

  // 仅做格式预检（http/https + 可解析）；公网/内网可达性由后端 SSRF 守卫裁决，
  // 其报错信息会经 saveWebhook 的 catch 原样提示，避免前后端规则不一致误拦合法地址。
  function isValidWebhookUrl(url: string): boolean {
    try {
      const u = new URL(url);
      return u.protocol === 'https:' || u.protocol === 'http:';
    } catch {
      return false;
    }
  }

  async function loadRows() {
    error.value = '';
    try {
      await webhooksStore.loadWebhooks({
        page: page.value,
        page_size: pageSize.value,
        search: search.value,
      });
      if (page.value > pageMax.value) page.value = pageMax.value;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    }
  }

  let skipNextPageWatch = false;
  watch(search, () => {
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

  function openCreate() {
    editingId.value = '';
    dialogRef.value?.reset();
    editorOpen.value = true;
  }

  function openEdit(row: WebhookRow) {
    editingId.value = row.id;
    dialogRef.value?.fill(row);
    editorOpen.value = true;
  }

  async function saveWebhook() {
    const payload = dialogRef.value?.getPayload();
    if (!payload || !payload.name?.trim() || !payload.url?.trim()) {
      $q.notify({ type: 'warning', message: t('webhooksPage.notifyRequired') });
      return;
    }
    if (!isValidWebhookUrl(payload.url)) {
      $q.notify({ type: 'warning', message: t('webhooksPage.notifyUrlInvalid') });
      return;
    }
    let eventTypes: unknown[] = [];
    try {
      const parsed = JSON.parse(payload.event_types_json ?? '[]');
      if (Array.isArray(parsed)) eventTypes = parsed;
    } catch {
      /* fall through to the empty check below */
    }
    if (eventTypes.length === 0) {
      $q.notify({ type: 'warning', message: t('webhooksPage.notifyEventTypesRequired') });
      return;
    }
    saving.value = true;
    const body = {
      name: payload.name.trim(),
      url: payload.url.trim(),
      event_types_json: payload.event_types_json,
      secret: payload.secret,
      headers: payload.headers,
      enabled: payload.enabled,
    };
    try {
      if (editingId.value) {
        await webhooksStore.saveWebhook(editingId.value, body);
      } else {
        const created = await webhooksStore.addWebhook(body);
        if (created.secret && created.secret !== WEBHOOK_SECRET_MASK) {
          showSecretReveal(created.secret);
        }
      }
      editorOpen.value = false;
      $q.notify({ type: 'positive', message: t('webhooksPage.notifySaved') });
      await loadRows();
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      saving.value = false;
    }
  }

  async function toggleEnabled(row: WebhookRow, enabled: boolean) {
    busyId.value = row.id;
    try {
      await webhooksStore.saveWebhook(row.id, { enabled });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      busyId.value = '';
    }
  }

  async function testRow(row: WebhookRow) {
    testingId.value = row.id;
    try {
      const res = await testWebhook(row.id);
      if (res.success) {
        $q.notify({
          type: 'positive',
          message: t('webhooksPage.notifyTestSuccess', {
            status: res.status_code,
            duration: res.duration_ms,
          }),
        });
      } else {
        $q.notify({
          type: 'negative',
          timeout: 8000,
          message: t('webhooksPage.notifyTestFailed', { error: res.error || `HTTP ${res.status_code}` }),
        });
      }
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      testingId.value = '';
    }
  }

  function confirmDelete(row: WebhookRow) {
    $q.dialog({
      title: t('webhooksPage.confirmDeleteTitle'),
      message: t('webhooksPage.confirmDeleteMessage', { name: row.name }),
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      try {
        await webhooksStore.removeWebhook(row.id);
        $q.notify({ type: 'positive', message: t('webhooksPage.notifyDeleted') });
        await loadRows();
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
      }
    });
  }

  function showSecretReveal(secret: string) {
    $q.dialog({
      title: t('webhooksPage.secretRevealTitle'),
      message: t('webhooksPage.secretRevealMessage') + '\n\n' + secret,
      ok: { label: t('webhooksPage.secretCopy'), color: 'primary' },
      cancel: { label: t('webhooksPage.btnCancel'), flat: true },
      persistent: true,
    }).onOk(() => {
      navigator.clipboard.writeText(secret).then(() => {
        $q.notify({ type: 'positive', message: t('webhooksPage.secretCopied') });
      });
    });
  }

  onMounted(loadRows);

  return {
    t,
    loading,
    saving,
    error,
    search,
    editorOpen,
    editingId,
    busyId,
    testingId,
    dialogRef,
    page,
    pageSize,
    total,
    filteredRows,
    pagedRows,
    pageMax,
    loadRows,
    openCreate,
    openEdit,
    saveWebhook,
    toggleEnabled,
    testRow,
    confirmDelete,
  };
}
