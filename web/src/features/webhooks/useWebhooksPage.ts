import { onMounted, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { WEBHOOK_SECRET_MASK, type WebhookRow } from './types';
import { useWebhooksStore } from '../../stores/webhooks';
import { useLocalPagination } from '../../composables/useLocalPagination';

export function useWebhooksPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const webhooksStore = useWebhooksStore();
  const { webhooks: storeRows, loading: storeLoading } = storeToRefs(webhooksStore);
  const loading = storeLoading;
  const saving = ref(false);
  const error = ref('');
  const search = ref('');
  const editorOpen = ref(false);
  const editingId = ref('');
  const busyId = ref('');
  const dialogRef = ref<InstanceType<import('../../components/webhooks/WebhookDialog.vue').default> | null>(null);

  const {
    page,
    rowsPerPage: pageSize,
    filteredRows,
    pagedRows,
    totalPages: pageMax,
  } = useLocalPagination<WebhookRow>({
    rows: storeRows,
    filter: search,
    filterFn: (r, q) => r.name.toLowerCase().includes(q) || r.url.toLowerCase().includes(q),
  });

  function isValidWebhookUrl(url: string): boolean {
    try {
      const u = new URL(url);
      return u.protocol === 'https:' || u.hostname === 'localhost' || u.hostname === '127.0.0.1';
    } catch {
      return false;
    }
  }

  async function loadRows() {
    error.value = '';
    try {
      await webhooksStore.loadWebhooks();
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    }
  }

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
    saving.value = true;
    try {
      if (editingId.value) {
        await webhooksStore.saveWebhook(editingId.value, payload);
      } else {
        const created = await webhooksStore.addWebhook(payload);
        // Show plaintext secret if present (only returned on create)
        if (created.secret && created.secret !== WEBHOOK_SECRET_MASK) {
          showSecretReveal(created.secret);
        }
      }
      editorOpen.value = false;
      $q.notify({ type: 'positive', message: t('webhooksPage.notifySaved') });
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
    dialogRef,
    page,
    pageSize,
    filteredRows,
    pagedRows,
    pageMax,
    loadRows,
    openCreate,
    openEdit,
    saveWebhook,
    toggleEnabled,
    confirmDelete,
  };
}
