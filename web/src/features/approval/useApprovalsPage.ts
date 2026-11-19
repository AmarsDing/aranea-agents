import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useAuthStore } from '../../stores/auth';
import { useNavStore } from '../../stores/nav';
import { isForbiddenApiError } from '../../utils/kratosError';
import {
  decideApproval,
  getApprovalSummary,
  getRiskMatrix,
  isConflictError,
  listApprovals,
  putRiskMatrix,
} from './api';
import {
  approvalItemKey,
  canDecideApproval,
  emptyMatrixRow,
  type ApprovalItemRow,
  type ApprovalKindKey,
  type DecideAction,
  type RiskMatrixRowModel,
} from './types';

export type ApprovalShortcut =
  | 'next'
  | 'previous'
  | 'approve'
  | 'reject'
  | 'toggleSelect'
  | 'selectAll'
  | 'clearSelection';

export type DecisionStatus = 'pending' | 'processing' | 'done' | 'conflict' | 'failed';

export type DecisionOutcome = {
  key: string;
  title: string;
  status: DecisionStatus;
  message?: string;
};

export function isApprovalTypingTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName);
}

export function resolveApprovalShortcut(
  event: KeyboardEvent,
  target: EventTarget | null = event.target,
): ApprovalShortcut | null {
  if (event.isComposing || event.altKey || event.metaKey) return null;
  if (event.ctrlKey && event.key === 'Enter') return 'reject';
  if (event.ctrlKey) {
    // Ctrl/Cmd+A 全选可见待办；输入框内保留原生全选。
    if (event.key.toLowerCase() === 'a' && !isApprovalTypingTarget(target)) return 'selectAll';
    return null;
  }
  if (isApprovalTypingTarget(target)) return null;
  const key = event.key.toLowerCase();
  if (key === 'j' || key === 'n' || key === 'arrowdown') return 'next';
  if (key === 'k' || key === 'arrowup') return 'previous';
  if (key === 'a') return 'approve';
  if (key === 'r') return 'reject';
  if (key === 'x') return 'toggleSelect';
  if (key === 'escape') return 'clearSelection';
  return null;
}

export function useApprovalsPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const auth = useAuthStore();
  const nav = useNavStore();

  const tab = ref<'queue' | 'matrix'>('queue');

  /** 矩阵写路径硬判 approval.policy（superadmin 经 * 覆盖）；展示层仅控显隐。 */
  const canPolicy = computed(() => nav.permissions.includes('*') || nav.permissions.includes('approval.policy'));

  // ---- 待办队列 ----
  const kindFilter = ref<ApprovalKindKey | ''>('');
  const items = ref<ApprovalItemRow[]>([]);
  const total = ref(0);
  const page = ref(1);
  const pageSize = ref(20);
  const pageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, total.value) / pageSize.value)));
  const loading = ref(false);
  const error = ref('');
  /** 403 / PERMISSION_DENIED：页面渲染 forbidden 态，不提供重试。 */
  const forbidden = ref(false);
  const summaryCounts = ref<Partial<Record<ApprovalKindKey, number>>>({});
  const summaryTotal = ref(0);

  const selected = ref<ApprovalItemRow | null>(null);
  const note = ref('');
  const deciding = ref(false);

  // ---- 批量裁决（DecisionDock 批量模式） ----
  const selectedKeys = ref<string[]>([]);
  const outcomes = ref<DecisionOutcome[]>([]);
  const batchRunning = ref(false);
  const batchDone = ref(0);
  const batchTotal = ref(0);
  const lastBatchAction = ref<DecideAction>('approve');
  const lastBatchNote = ref('');

  const selectedItems = computed(() =>
    items.value.filter((item) => selectedKeys.value.includes(approvalItemKey(item))),
  );
  const failedOutcomes = computed(() => outcomes.value.filter((item) => item.status === 'failed'));
  const checkableItems = computed(() => items.value.filter((item) => canDecideApproval(item.kind)));
  const allVisibleChecked = computed(
    () => checkableItems.value.length > 0 && checkableItems.value.every((item) => isChecked(item)),
  );
  const someVisibleChecked = computed(() => selectedKeys.value.length > 0 && !allVisibleChecked.value);

  function isChecked(item: ApprovalItemRow): boolean {
    return selectedKeys.value.includes(approvalItemKey(item));
  }

  function toggleSelect(item: ApprovalItemRow) {
    if (!canDecideApproval(item.kind)) return;
    const key = approvalItemKey(item);
    selectedKeys.value = selectedKeys.value.includes(key)
      ? selectedKeys.value.filter((candidate) => candidate !== key)
      : [...selectedKeys.value, key];
  }

  function selectAllVisible() {
    selectedKeys.value = checkableItems.value.map(approvalItemKey);
  }

  function clearSelection() {
    selectedKeys.value = [];
  }

  function dismissOutcomes() {
    outcomes.value = [];
  }

  async function loadSummary() {
    try {
      const res = await getApprovalSummary();
      summaryCounts.value = res.counts;
      summaryTotal.value = res.total;
    } catch {
      // 计数失败不阻断列表（徽章退化为不显示），列表错误走 error 横幅。
    }
  }

  async function loadList() {
    error.value = '';
    forbidden.value = false;
    loading.value = true;
    try {
      const res = await listApprovals({ kind: kindFilter.value, page: page.value, pageSize: pageSize.value });
      items.value = res.items;
      total.value = res.total;
      if (page.value > pageMax.value) page.value = pageMax.value;
      // 选中项消失（被他人决议/翻页出窗）时清空详情，防对旧条目重复决议。
      if (selected.value && !res.items.some((it) => it.kind === selected.value!.kind && it.id === selected.value!.id)) {
        selected.value = null;
      }
      // 勾选只作用于当前可见页：翻页/筛选后丢弃不可见的键，避免批量误伤。
      const visibleKeys = new Set(res.items.map(approvalItemKey));
      selectedKeys.value = selectedKeys.value.filter((key) => visibleKeys.has(key));
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      forbidden.value = isForbiddenApiError(e);
    } finally {
      loading.value = false;
    }
  }

  let skipNextPageWatch = false;
  watch(kindFilter, () => {
    if (page.value !== 1) {
      skipNextPageWatch = true;
      page.value = 1;
    }
    void loadList();
  });
  watch([page, pageSize], () => {
    if (skipNextPageWatch) {
      skipNextPageWatch = false;
      return;
    }
    void loadList();
  });

  function select(item: ApprovalItemRow) {
    selected.value = item;
    note.value = '';
  }

  function selectNext() {
    if (!items.value.length) return;
    const index = selected.value ? items.value.findIndex((item) => item.id === selected.value!.id && item.kind === selected.value!.kind) : -1;
    select(items.value[Math.min(index + 1, items.value.length - 1)]);
  }

  function selectPrevious() {
    if (!items.value.length) return;
    const index = selected.value ? items.value.findIndex((item) => item.id === selected.value!.id && item.kind === selected.value!.kind) : 0;
    select(items.value[Math.max(index - 1, 0)]);
  }

  async function refreshAfterDecision(index: number) {
    await Promise.all([loadList(), loadSummary()]);
    const next = items.value[Math.min(index, items.value.length - 1)];
    if (next) select(next);
  }

  type DecisionResult = Pick<DecisionOutcome, 'status' | 'message'>;

  /** 单条裁决的唯一出口：单条与批量共用，保证 409/失败语义一致。 */
  async function decideItem(item: ApprovalItemRow, action: DecideAction, reason: string): Promise<DecisionResult> {
    if (!canDecideApproval(item.kind, action)) {
      return { status: 'failed', message: t('shell.approvalDock.unsupported') };
    }
    try {
      await decideApproval({
        item,
        action,
        note: reason,
        actor: auth.user ? String(auth.user.id) : '',
      });
      return { status: 'done' };
    } catch (e) {
      if (isConflictError(e)) return { status: 'conflict' };
      return { status: 'failed', message: e instanceof Error ? e.message : String(e) };
    }
  }

  async function decide(action: DecideAction) {
    const item = selected.value;
    if (!item || deciding.value) return;
    const selectedIndex = items.value.findIndex((candidate) => candidate.id === item.id && candidate.kind === item.kind);
    if (action === 'reject' && !note.value.trim()) {
      $q.notify({ type: 'warning', message: t('approvalsPage.noteRequired') });
      return;
    }
    deciding.value = true;
    try {
      const result = await decideItem(item, action, note.value.trim());
      if (result.status === 'done') {
        $q.notify({
          type: 'positive',
          message: t(
            `approvalsPage.notify_${action === 'archive' ? 'archived' : action === 'approve' ? 'approved' : 'rejected'}`,
          ),
        });
        selected.value = null;
        note.value = '';
        selectedKeys.value = selectedKeys.value.filter((key) => key !== approvalItemKey(item));
        await refreshAfterDecision(selectedIndex);
        return;
      }
      if (result.status === 'conflict') {
        // 并发决议恰一个成功：本端 409 = 已被他人处理，按提示而非错误呈现。
        $q.notify({ type: 'info', message: t('approvalsPage.notifyConflict') });
        selected.value = null;
        selectedKeys.value = selectedKeys.value.filter((key) => key !== approvalItemKey(item));
        await refreshAfterDecision(selectedIndex);
        return;
      }
      // 失败保留选中与理由，用户可直接重试，不必重新找到这一条。
      $q.notify({
        type: 'negative',
        message: t('approvalsPage.notifyDecideFailed', { msg: result.message ?? '' }),
      });
    } finally {
      deciding.value = false;
    }
  }

  function patchOutcome(key: string, patch: Partial<DecisionOutcome>) {
    outcomes.value = outcomes.value.map((entry) => (entry.key === key ? { ...entry, ...patch } : entry));
  }

  async function runBatch(targets: ApprovalItemRow[], action: DecideAction, reason: string) {
    if (!targets.length || batchRunning.value) return;
    batchRunning.value = true;
    batchTotal.value = targets.length;
    batchDone.value = 0;
    lastBatchAction.value = action;
    lastBatchNote.value = reason;
    outcomes.value = targets.map((item) => ({
      key: approvalItemKey(item),
      title: item.title || item.id,
      status: 'pending' as DecisionStatus,
    }));
    let ok = 0;
    let conflict = 0;
    let failed = 0;
    for (const item of targets) {
      const key = approvalItemKey(item);
      patchOutcome(key, { status: 'processing' });
      const result = await decideItem(item, action, reason);
      patchOutcome(key, result);
      if (result.status === 'done') {
        ok += 1;
        selectedKeys.value = selectedKeys.value.filter((candidate) => candidate !== key);
      } else if (result.status === 'conflict') {
        conflict += 1;
        selectedKeys.value = selectedKeys.value.filter((candidate) => candidate !== key);
      } else {
        failed += 1;
      }
      batchDone.value += 1;
    }
    batchRunning.value = false;
    await Promise.all([loadList(), loadSummary()]);
    $q.notify({
      type: failed > 0 ? 'warning' : 'positive',
      message: t('shell.approvalDock.batchDone', { ok, conflict, failed }),
    });
  }

  /** 批量裁决：只处理当前动作受支持的已选项；驳回沿用同一理由。 */
  function batchDecide(action: DecideAction): Promise<void> | undefined {
    if (batchRunning.value) return undefined;
    const targets = selectedItems.value.filter((item) => canDecideApproval(item.kind, action));
    if (!targets.length) {
      $q.notify({ type: 'warning', message: t('shell.approvalDock.unsupported') });
      return undefined;
    }
    if (action === 'reject' && !note.value.trim()) {
      $q.notify({ type: 'warning', message: t('approvalsPage.noteRequired') });
      return undefined;
    }
    const reason = note.value.trim();
    if (reason) note.value = '';
    return runBatch(targets, action, reason);
  }

  /** 重试失败项：不重新提交已成功/已被处理的条目，避免重复裁决。 */
  function retryFailed(): Promise<void> | undefined {
    if (batchRunning.value) return undefined;
    const keys = new Set(failedOutcomes.value.map((entry) => entry.key));
    const targets = items.value.filter((item) => keys.has(approvalItemKey(item)));
    if (!targets.length) {
      $q.notify({ type: 'info', message: t('shell.approvalDock.retryUnavailable') });
      return undefined;
    }
    return runBatch(targets, lastBatchAction.value, lastBatchNote.value);
  }

  // ---- 风险矩阵（A-4） ----
  const matrixRows = ref<RiskMatrixRowModel[]>([]);
  const matrixLoading = ref(false);
  const matrixSaving = ref(false);
  const matrixError = ref('');
  const matrixDirty = ref(false);

  async function loadMatrix() {
    matrixError.value = '';
    matrixLoading.value = true;
    try {
      matrixRows.value = await getRiskMatrix();
      matrixDirty.value = false;
    } catch (e) {
      matrixError.value = e instanceof Error ? e.message : String(e);
    } finally {
      matrixLoading.value = false;
    }
  }

  function addMatrixRow() {
    matrixRows.value = [...matrixRows.value, emptyMatrixRow()];
    matrixDirty.value = true;
  }

  function removeMatrixRow(idx: number) {
    matrixRows.value = matrixRows.value.filter((_, i) => i !== idx);
    matrixDirty.value = true;
  }

  function markMatrixDirty() {
    matrixDirty.value = true;
  }

  async function saveMatrix() {
    if (matrixSaving.value) return;
    matrixSaving.value = true;
    try {
      matrixRows.value = await putRiskMatrix(matrixRows.value);
      matrixDirty.value = false;
      $q.notify({ type: 'positive', message: t('approvalsPage.matrixSaved') });
    } catch (e) {
      $q.notify({
        type: 'negative',
        message: t('approvalsPage.matrixSaveFailed', { msg: e instanceof Error ? e.message : String(e) }),
      });
    } finally {
      matrixSaving.value = false;
    }
  }

  watch(tab, (v) => {
    if (v === 'matrix' && canPolicy.value && matrixRows.value.length === 0 && !matrixError.value) {
      void loadMatrix();
    }
  });

  onMounted(() => {
    void nav.ensureMenus();
    void loadList();
    void loadSummary();
  });

  return {
    tab,
    canPolicy,
    kindFilter,
    items,
    total,
    page,
    pageSize,
    pageMax,
    loading,
    error,
    forbidden,
    summaryCounts,
    summaryTotal,
    selected,
    note,
    deciding,
    selectedKeys,
    selectedItems,
    outcomes,
    failedOutcomes,
    batchRunning,
    batchDone,
    batchTotal,
    allVisibleChecked,
    someVisibleChecked,
    isChecked,
    toggleSelect,
    selectAllVisible,
    clearSelection,
    dismissOutcomes,
    batchDecide,
    retryFailed,
    loadList,
    select,
    selectNext,
    selectPrevious,
    decide,
    matrixRows,
    matrixLoading,
    matrixSaving,
    matrixError,
    matrixDirty,
    loadMatrix,
    addMatrixRow,
    removeMatrixRow,
    markMatrixDirty,
    saveMatrix,
  };
}
