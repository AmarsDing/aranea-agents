import { computed, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { WEBHOOK_EVENT_TYPES, WEBHOOK_SECRET_MASK, type WebhookRow } from './types';
import { testWebhook, type WebhookTestResult } from './api';
import { useWebhooksStore } from '../../stores/webhooks';
import { isForbiddenApiError } from '../../utils/kratosError';
import { confirm } from '../../composables/useAppConfirm';
import type { CapStat } from '../../components/capability/capTypes';

interface WebhookDialogExposed {
  reset: () => void;
  fill: (row: WebhookRow) => void;
  getPayload: () => Partial<WebhookRow> | null;
}

export function parseWebhookEvents(json: string): string[] {
  if (!json?.trim()) return [];
  try {
    const parsed = JSON.parse(json);
    return Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === 'string') : [];
  } catch {
    return [];
  }
}

export function useWebhooksPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const webhooksStore = useWebhooksStore();
  const { webhooks: storeRows, loading } = storeToRefs(webhooksStore);
  const saving = ref(false);
  const error = ref('');
  /** 列表接口 403：页面据此渲染 forbidden 态（无重试）。 */
  const forbidden = ref(false);
  const loaded = ref(false);
  const search = ref('');
  const stateFilter = ref('');
  const page = ref(1);
  const pageSize = ref(24);
  const editorOpen = ref(false);
  const editingId = ref('');
  const busyId = ref('');
  const testingId = ref('');
  const dialogRef = ref<WebhookDialogExposed | null>(null);
  const inspectedId = ref('');
  /** 本次打开页面以来每条推送最近一次测试的结果（后端不存投递记录，只能看这一次的） */
  const lastTest = ref<Record<string, WebhookTestResult>>({});

  // shortcut: 出站推送一般只有个位数，整表读回在前端筛选/分页，状态条的数才准；上百条时改回服务端分页 + 计数接口
  const filteredRows = computed(() => {
    const q = search.value.trim().toLowerCase();
    return storeRows.value.filter((row) => {
      if (stateFilter.value === 'enabled' && !row.enabled) return false;
      if (stateFilter.value === 'disabled' && row.enabled) return false;
      return !q || row.name.toLowerCase().includes(q) || row.url.toLowerCase().includes(q);
    });
  });
  const total = computed(() => filteredRows.value.length);
  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const pagedRows = computed(() =>
    filteredRows.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value),
  );

  const stats = computed<Array<CapStat & { clickable: boolean }>>(() => {
    const all = storeRows.value.length;
    const on = storeRows.value.filter((row) => row.enabled).length;
    return [
      { key: '', label: t('connect.common.all'), value: all, clickable: true },
      { key: 'enabled', label: t('connect.common.enabled'), value: on, tone: 'ok', clickable: true },
      { key: 'disabled', label: t('connect.common.disabled'), value: all - on, tone: 'off', clickable: true },
    ];
  });

  const inspected = computed(() => storeRows.value.find((row) => row.id === inspectedId.value) ?? null);
  const inspectorOpen = computed({
    get: () => inspected.value !== null,
    set: (open: boolean) => {
      if (!open) inspectedId.value = '';
    },
  });

  watch([search, stateFilter, pageSize], () => {
    page.value = 1;
  });
  watch(pageMax, (max) => {
    if (page.value > max) page.value = max;
  });

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
    forbidden.value = false;
    try {
      await webhooksStore.loadWebhooks();
      loaded.value = true;
    } catch (e) {
      forbidden.value = isForbiddenApiError(e);
      error.value = e instanceof Error ? e.message : String(e);
    }
  }

  function eventLabel(evt: string): string {
    const found = WEBHOOK_EVENT_TYPES.find((et) => et.value === evt);
    return found ? t(found.labelKey) : evt;
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

  function inspect(row: WebhookRow) {
    inspectedId.value = row.id;
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
    if (parseWebhookEvents(payload.event_types_json ?? '[]').length === 0) {
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
        inspectedId.value = created.id;
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
      lastTest.value = { ...lastTest.value, [row.id]: res };
      $q.notify({
        type: res.success ? 'positive' : 'negative',
        timeout: res.success ? 3000 : 8000,
        message: res.success
          ? t('webhooksPage.notifyTestSuccess', { status: res.status_code, duration: res.duration_ms })
          : t('webhooksPage.notifyTestFailed', { error: res.error || `HTTP ${res.status_code}` }),
      });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      testingId.value = '';
    }
  }

  async function confirmDelete(row: WebhookRow) {
    const ok = await confirm({
      title: t('webhooksPage.confirmDeleteTitle'),
      message: t('connect.webhooks.deleteMessage', {
        name: row.name,
        url: row.url,
        events: parseWebhookEvents(row.event_types_json).map(eventLabel).join('、') || t('connect.common.none'),
      }),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await webhooksStore.removeWebhook(row.id);
      if (inspectedId.value === row.id) inspectedId.value = '';
      $q.notify({ type: 'positive', message: t('webhooksPage.notifyDeleted') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    }
  }

  function showSecretReveal(secret: string) {
    void (async () => {
      const copy = await confirm({
        title: t('webhooksPage.secretRevealTitle'),
        message: `${t('webhooksPage.secretRevealMessage')}\n\n${secret}`,
        confirmLabel: t('webhooksPage.secretCopy'),
        cancelLabel: t('webhooksPage.btnCancel'),
        tone: 'primary',
      });
      if (!copy) return;
      await navigator.clipboard.writeText(secret).then(() => {
        $q.notify({ type: 'positive', message: t('webhooksPage.secretCopied') });
      });
    })();
  }

  onMounted(loadRows);

  return {
    t,
    rows: storeRows,
    loaded,
    loading,
    saving,
    error,
    forbidden,
    search,
    stateFilter,
    stats,
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
    inspected,
    inspectorOpen,
    lastTest,
    eventLabel,
    loadRows,
    openCreate,
    openEdit,
    inspect,
    saveWebhook,
    toggleEnabled,
    testRow,
    confirmDelete,
  };
}
