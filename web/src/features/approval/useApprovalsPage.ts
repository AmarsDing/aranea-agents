import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useAuthStore } from '../../stores/auth';
import { useNavStore } from '../../stores/nav';
import {
  decideApproval,
  getApprovalSummary,
  getRiskMatrix,
  isConflictError,
  listApprovals,
  putRiskMatrix,
} from './api';
import {
  emptyMatrixRow,
  type ApprovalItemRow,
  type ApprovalKindKey,
  type DecideAction,
  type RiskMatrixRowModel,
} from './types';

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
  const summaryCounts = ref<Partial<Record<ApprovalKindKey, number>>>({});
  const summaryTotal = ref(0);

  const selected = ref<ApprovalItemRow | null>(null);
  const note = ref('');
  const deciding = ref(false);

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
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
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

  async function decide(action: DecideAction) {
    const item = selected.value;
    if (!item || deciding.value) return;
    if (action === 'reject' && !note.value.trim()) {
      $q.notify({ type: 'warning', message: t('approvalsPage.noteRequired') });
      return;
    }
    deciding.value = true;
    try {
      await decideApproval({
        item,
        action,
        note: note.value.trim(),
        actor: auth.user ? String(auth.user.id) : '',
      });
      $q.notify({
        type: 'positive',
        message: t(
          `approvalsPage.notify_${action === 'archive' ? 'archived' : action === 'approve' ? 'approved' : 'rejected'}`,
        ),
      });
      selected.value = null;
      note.value = '';
      await Promise.all([loadList(), loadSummary()]);
    } catch (e) {
      if (isConflictError(e)) {
        // 并发决议恰一个成功：本端 409 = 已被他人处理，按提示而非错误呈现。
        $q.notify({ type: 'info', message: t('approvalsPage.notifyConflict') });
        selected.value = null;
        await Promise.all([loadList(), loadSummary()]);
      } else {
        $q.notify({
          type: 'negative',
          message: t('approvalsPage.notifyDecideFailed', { msg: e instanceof Error ? e.message : String(e) }),
        });
      }
    } finally {
      deciding.value = false;
    }
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
    summaryCounts,
    summaryTotal,
    selected,
    note,
    deciding,
    loadList,
    select,
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
