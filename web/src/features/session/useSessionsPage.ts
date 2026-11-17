import { computed, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useQuasar } from 'quasar';
import { formatBatchNotifyMessage } from './batchNotify';
import type { BatchPreviewResult, BulkProgress, RetentionDialogMode, SessionBatchScope } from './types';
import type { Session } from './types';
import { useSessionsBulk } from './useSessionsBulk';
import { useSessionStore } from '../../stores/session/index';
import { sortSessionsForDisplay } from './sessionSort';
import { downloadTextFile } from './downloadExport';
import { isForbiddenApiError } from '../../utils/kratosError';

export function useSessionsPage() {
  const $q = useQuasar();
  const route = useRoute();
  const sessionStore = useSessionStore();

  const rows = ref<Session[]>([]);
  const selected = ref<Session | null>(null);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');
  /** 列表接口 403：页面据此渲染 forbidden 态（无重试）。 */
  const forbidden = ref(false);

  const keyword = ref('');
  const ownerType = ref<string | null>(null);
  const status = ref<string | null>(null);
  const contextStatus = ref<string | null>(null);
  const page = ref(1);
  const pageSize = ref(20);

  // 选择列默认展示：批量操作条依赖每行可选；工具栏按钮可临时收起以收窄表格。
  const selectionMode = ref(true);
  /** 选择状态与逐项批量执行（归档/取消归档/删除）内聚在 useSessionsBulk。 */
  const bulk = useSessionsBulk(rows, loadRows);

  const bulkProgress = ref<BulkProgress>({ active: false, label: '', indeterminate: true });

  const deleteDialogOpen = ref(false);
  const deleteTargetIds = ref<string[]>([]);
  const deleteLoading = ref(false);

  const retentionOpen = ref(false);
  const retentionMode = ref<RetentionDialogMode>('archive');
  const retentionPreview = ref<BatchPreviewResult | null>(null);
  const retentionPreviewLoading = ref(false);
  const retentionLoading = ref(false);

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const selectedCount = bulk.count;

  const batchScope = computed<SessionBatchScope>(() => ({
    owner_type: ownerType.value || undefined,
    status: status.value || undefined,
    context_status: contextStatus.value || undefined,
    keyword: keyword.value || undefined,
  }));

  function clearSelection() {
    bulk.clear();
  }

  // 筛选条件变化必须清空选择（并回到第 1 页）：否则批量动作会作用到用户已看不见的行。
  watch([keyword, ownerType, status, contextStatus], () => {
    page.value = 1;
    clearSelection();
    void loadRows();
  });

  // 分页/页大小变化同样清空选择：「本页全选」只在当前页生效，跨页残留会造成误操作。
  watch([page, pageSize], () => {
    clearSelection();
    void loadRows();
  });

  watch(
    () => route.params.sessionId,
    () => {
      void loadSelected();
    },
  );

  function onKeywordUpdate(value: string | number | null) {
    keyword.value = value == null || value === '' ? '' : String(value);
  }

  function resetFilters() {
    keyword.value = '';
    ownerType.value = null;
    status.value = null;
    contextStatus.value = null;
    page.value = 1;
    clearSelection();
  }

  function toggleSelectionMode() {
    selectionMode.value = !selectionMode.value;
    if (!selectionMode.value) {
      clearSelection();
    }
  }

  const toggleRowSelection = bulk.toggleRow;
  const togglePageSelection = bulk.toggleAll;
  const isRowSelected = bulk.isSelected;

  function isPageFullySelected() {
    return bulk.pageAllSelected.value;
  }

  async function loadRows() {
    loading.value = true;
    error.value = '';
    forbidden.value = false;
    try {
      const result = await sessionStore.searchPage({
        keyword: keyword.value || undefined,
        owner_type: ownerType.value || undefined,
        status: status.value || undefined,
        context_status: contextStatus.value || undefined,
        // 管理列表默认只列根会话，团队成员会话等子会话不混入；
        // 显式筛选 owner_type=team 时除外（团队会话均有 parent）。
        root_only: ownerType.value !== 'team',
        limit: pageSize.value,
        offset: (page.value - 1) * pageSize.value,
      });
      rows.value = sortSessionsForDisplay(result.items);
      total.value = result.total;
      await loadSelected();
    } catch (err) {
      forbidden.value = isForbiddenApiError(err);
      error.value = err instanceof Error ? err.message : '加载 Session 失败';
    } finally {
      loading.value = false;
    }
  }

  function selectSession(session: Session) {
    selected.value = session;
  }

  async function loadSelected() {
    const id = String(route.params.sessionId || '');
    if (!id) {
      const selectedId = selected.value?.id;
      selected.value = selectedId ? (rows.value.find((item) => item.id === selectedId) ?? null) : null;
      return;
    }
    selected.value = rows.value.find((item) => item.id === id) ?? (await sessionStore.fetchSession(id));
  }

  function startBulk(label: string) {
    bulkProgress.value = { active: true, label, indeterminate: true };
  }

  function endBulk() {
    bulkProgress.value = { active: false, label: '', indeterminate: true };
  }

  function notifyBatchResult(
    action: 'archive' | 'delete',
    result: Awaited<ReturnType<typeof sessionStore.batchArchive>>,
    requested?: number,
  ) {
    const type = result.failed_ids.length > 0 || result.truncated ? 'warning' : 'positive';
    $q.notify({ type, message: formatBatchNotifyMessage(action, result, requested) });
  }

  async function runBatchDelete(ids: string[]) {
    if (!ids.length) return;
    startBulk('正在删除…');
    try {
      const result = await sessionStore.batchDelete({ ids });
      notifyBatchResult('delete', result, ids.length);
      clearSelection();
      await loadRows();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
    } finally {
      endBulk();
    }
  }

  function promptDelete(ids: string[]) {
    deleteTargetIds.value = ids;
    deleteDialogOpen.value = true;
  }

  async function confirmDeleteDialog() {
    const ids = [...deleteTargetIds.value];
    if (!ids.length) return;
    deleteLoading.value = true;
    try {
      if (ids.length === 1) {
        await sessionStore.removeSession(ids[0]);
        $q.notify({ type: 'positive', message: '会话已删除' });
        deleteDialogOpen.value = false;
        deleteTargetIds.value = [];
        await loadRows();
      } else {
        deleteDialogOpen.value = false;
        deleteTargetIds.value = [];
        deleteLoading.value = false;
        await runBatchDelete(ids);
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
    } finally {
      deleteLoading.value = false;
    }
  }

  function openRetention(mode: RetentionDialogMode) {
    retentionMode.value = mode;
    retentionPreview.value = null;
    retentionOpen.value = true;
  }

  async function previewRetention(payload: { days: number; includeArchived: boolean }) {
    retentionPreviewLoading.value = true;
    try {
      retentionPreview.value = await sessionStore.previewBatch({
        mode: retentionMode.value,
        older_than_days: payload.days,
        scope: batchScope.value,
        include_archived: payload.includeArchived,
      });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '预览失败' });
    } finally {
      retentionPreviewLoading.value = false;
    }
  }

  async function confirmRetention(payload: { days: number; includeArchived: boolean }) {
    retentionLoading.value = true;
    const action = retentionMode.value;
    startBulk(action === 'archive' ? '正在批量归档…' : '正在批量删除…');
    try {
      const result =
        action === 'archive'
          ? await sessionStore.batchArchive({ older_than_days: payload.days, scope: batchScope.value })
          : await sessionStore.batchDelete({
              older_than_days: payload.days,
              scope: batchScope.value,
              include_archived: payload.includeArchived,
            });
      notifyBatchResult(action, result);
      retentionOpen.value = false;
      retentionPreview.value = null;
      await loadRows();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '操作失败' });
    } finally {
      retentionLoading.value = false;
      endBulk();
    }
  }

  async function archiveRow(id: string) {
    try {
      await sessionStore.archive(id);
      if (selected.value?.id === id) {
        selected.value = { ...selected.value, archived_at: new Date().toISOString() };
      }
      await loadRows();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '归档失败' });
    }
  }

  async function togglePinRow(id: string, pinned: boolean) {
    try {
      const updated = await sessionStore.setPinned(id, pinned);
      rows.value = sortSessionsForDisplay(rows.value.map((row) => (row.id === id ? updated : row)));
      if (selected.value?.id === id) {
        selected.value = updated;
      }
      $q.notify({ type: 'positive', message: pinned ? '已置顶' : '已取消置顶' });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : pinned ? '置顶失败' : '取消置顶失败',
      });
    }
  }

  async function exportSelectedDetail(format: 'markdown' | 'json') {
    if (!selected.value) return;
    try {
      const payload = await sessionStore.exportSession(selected.value.id, format);
      downloadTextFile(payload.content, payload.filename, payload.content_type);
      $q.notify({ type: 'positive', message: '导出成功' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '导出失败' });
    }
  }

  return {
    rows,
    selected,
    total,
    loading,
    error,
    forbidden,
    keyword,
    ownerType,
    status,
    contextStatus,
    page,
    pageSize,
    pageMax,
    selectionMode,
    selectedCount,
    bulkProgress,
    bulkActions: bulk.actions,
    bulkBusy: bulk.busy,
    bulkError: bulk.error,
    runBulkAction: bulk.run,
    clearSelection,
    deleteDialogOpen,
    deleteTargetIds,
    deleteLoading,
    retentionOpen,
    retentionMode,
    retentionPreview,
    retentionPreviewLoading,
    retentionLoading,
    batchScope,
    onKeywordUpdate,
    resetFilters,
    loadRows,
    selectSession,
    toggleSelectionMode,
    toggleRowSelection,
    togglePageSelection,
    isRowSelected,
    isPageFullySelected,
    promptDelete,
    confirmDeleteDialog,
    openRetention,
    previewRetention,
    confirmRetention,
    archiveRow,
    togglePinRow,
    exportSession: sessionStore.exportSession,
    exportSelectedDetail,
  };
}
