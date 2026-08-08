import { computed, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { storeToRefs } from 'pinia';
import { useSelfImprovementStore } from '../../stores/selfImprovement';
import type { SIRiskRules, SIRun } from './types';
import { canApprove, canClose, canReject, canRollback } from '../../components/self-improvement/selfImprovementUi';

const EMPTY_RULES: SIRiskRules = { lowMaxLines: 0, mediumMaxLines: 0, corePathGlobs: [], dailyAutoQuota: 0 };

// Page orchestration for the self-improvement console (73-self-iteration-v3
// design §八): filters + pagination + detail drawer + governance actions.
// All network access goes through the store; this composable only owns UI
// state (filter refs, drawer open flag, action dialogs).

export function useSelfImprovementPage() {
  const store = useSelfImprovementStore();
  const {
    runs,
    total,
    loading,
    detail,
    detailLoading,
    outcomeStats,
    statsLoading,
    riskRules,
    rulesLoading,
    featureDisabled,
    statusInfo,
    statsFailed,
    errorKind,
  } = storeToRefs(store);
  const $q = useQuasar();
  const { t } = useI18n();

  const status = ref('');
  const riskLevel = ref('');
  const triggerSource = ref('');
  const page = ref(1);
  const pageSize = ref(20);

  const error = computed(() => store.error ?? '');
  // 错误码人性化（P5.5）：按 store 分类映射 i18n 文案，unknown 回退原始 message。
  const errorMessage = computed(() => {
    switch (errorKind.value) {
      case 'forbidden':
        return t('selfImprovementPage.errorForbidden');
      case 'legacy':
        return t('selfImprovementPage.errorLegacy');
      case 'unavailable':
        return t('selfImprovementPage.errorUnavailable');
      default:
        return error.value;
    }
  });
  // 重试只对暂时性故障有意义；forbidden/legacy 需改权限或升级后端。
  const errorRetryable = computed(
    () => errorKind.value === '' || errorKind.value === 'unavailable' || errorKind.value === 'unknown',
  );
  // 前置条件自检（P5.5）：enabled 但 Refine LLM 未配置或沙盒 repo_root 无效时给出警告。
  const prereqLlmMissing = computed(() => statusInfo.value?.enabled === true && !statusInfo.value.refineLlmConfigured);
  const prereqRepoInvalid = computed(() => statusInfo.value?.enabled === true && !statusInfo.value.repoRootValid);
  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

  const drawerOpen = ref(false);
  const actionRunning = ref('');

  const statusOptions = computed(() => [
    { label: t('selfImprovementPage.filterAll'), value: '' },
    { label: t('selfImprovementPage.status.awaiting_governance'), value: 'awaiting_governance' },
    { label: t('selfImprovementPage.status.observing'), value: 'observing' },
    { label: t('selfImprovementPage.status.applying'), value: 'applying' },
    { label: t('selfImprovementPage.status.detected'), value: 'detected' },
    { label: t('selfImprovementPage.status.diagnosing'), value: 'diagnosing' },
    { label: t('selfImprovementPage.status.patching'), value: 'patching' },
    { label: t('selfImprovementPage.status.verifying'), value: 'verifying' },
    { label: t('selfImprovementPage.status.closed'), value: 'closed' },
    { label: t('selfImprovementPage.status.rolled_back'), value: 'rolled_back' },
    { label: t('selfImprovementPage.status.verify_failed'), value: 'verify_failed' },
    { label: t('selfImprovementPage.status.rejected'), value: 'rejected' },
    { label: t('selfImprovementPage.status.failed'), value: 'failed' },
  ]);

  const riskOptions = computed(() => [
    { label: t('selfImprovementPage.filterAll'), value: '' },
    { label: t('selfImprovementPage.risk.low'), value: 'low' },
    { label: t('selfImprovementPage.risk.medium'), value: 'medium' },
    { label: t('selfImprovementPage.risk.high'), value: 'high' },
  ]);

  const triggerOptions = computed(() => [
    { label: t('selfImprovementPage.filterAll'), value: '' },
    { label: t('selfImprovementPage.trigger.error_cluster'), value: 'error_cluster' },
    { label: t('selfImprovementPage.trigger.perf_bottleneck'), value: 'perf_bottleneck' },
    { label: t('selfImprovementPage.trigger.eval_regression'), value: 'eval_regression' },
    { label: t('selfImprovementPage.trigger.test_failure'), value: 'test_failure' },
  ]);

  function currentFilter(): SIRunFilter {
    return {
      status: status.value || undefined,
      riskLevel: riskLevel.value || undefined,
      triggerSource: triggerSource.value || undefined,
      page: page.value,
      pageSize: pageSize.value,
    };
  }

  async function loadRows() {
    await store.loadRuns(currentFilter());
  }

  /** 「重新检测」（disabled 空态）：重新探测 GetStatus，已启用则加载数据。 */
  async function recheckAction() {
    await store.recheck(currentFilter());
  }

  function resetFilters() {
    status.value = '';
    riskLevel.value = '';
    triggerSource.value = '';
    page.value = 1;
  }

  // Merged watcher: one fetch per user gesture. Filter changes reset the page
  // first; the page reset re-triggers this same watcher and performs the load.
  watch([status, riskLevel, triggerSource, page, pageSize], (next, prev) => {
    const filtersChanged = next[0] !== prev[0] || next[1] !== prev[1] || next[2] !== prev[2];
    if (filtersChanged && page.value !== 1) {
      page.value = 1;
      return;
    }
    void loadRows();
  });

  function openDetail(row: SIRun) {
    drawerOpen.value = true;
    void store.loadRun(row.id);
  }

  function notifyError(e: unknown) {
    $q.notify({
      type: 'negative',
      message: e instanceof Error ? e.message : String(e),
    });
  }

  async function runAction(key: string, fn: () => Promise<void>, done: () => Promise<void>) {
    if (actionRunning.value) return;
    actionRunning.value = key;
    try {
      await fn();
      $q.notify({ type: 'positive', message: t('selfImprovementPage.actionDone') });
      await done();
    } catch (e) {
      notifyError(e);
    } finally {
      actionRunning.value = '';
    }
  }

  async function refreshAfterAction(id: string) {
    await loadRows();
    if (drawerOpen.value) {
      await store.loadRun(id);
    }
    void store.loadOutcomeStats();
  }

  function approveRunAction(run: SIRun) {
    if (!canApprove(run.status)) return;
    $q.dialog({
      title: t('selfImprovementPage.approveTitle'),
      message: t('selfImprovementPage.approveConfirm', { id: run.id }),
      prompt: { model: '', type: 'text', label: t('selfImprovementPage.reasonOptional') },
      cancel: true,
      persistent: true,
    }).onOk((reason: string) => {
      void runAction(
        'approve',
        () => store.approve(run.id, reason || undefined),
        () => refreshAfterAction(run.id),
      );
    });
  }

  function rejectRunAction(run: SIRun) {
    if (!canReject(run.status)) return;
    $q.dialog({
      title: t('selfImprovementPage.rejectTitle'),
      message: t('selfImprovementPage.rejectConfirm', { id: run.id }),
      prompt: {
        model: '',
        type: 'textarea',
        label: t('selfImprovementPage.reasonRequired'),
        isValid: (v: string) => v.trim().length > 0,
      },
      cancel: true,
      persistent: true,
    }).onOk((reason: string) => {
      void runAction(
        'reject',
        () => store.reject(run.id, reason.trim()),
        () => refreshAfterAction(run.id),
      );
    });
  }

  function rollbackRunAction(run: SIRun) {
    if (!canRollback(run.status)) return;
    $q.dialog({
      title: t('selfImprovementPage.rollbackTitle'),
      message: t('selfImprovementPage.rollbackConfirm', { id: run.id }),
      prompt: { model: '', type: 'text', label: t('selfImprovementPage.reasonOptional') },
      cancel: true,
      persistent: true,
    }).onOk((reason: string) => {
      void runAction(
        'rollback',
        () => store.rollback(run.id, reason || undefined),
        () => refreshAfterAction(run.id),
      );
    });
  }

  function closeRunAction(run: SIRun) {
    if (!canClose(run.status)) return;
    $q.dialog({
      title: t('selfImprovementPage.closeTitle'),
      message: t('selfImprovementPage.closeConfirm', { id: run.id }),
      cancel: true,
      persistent: true,
    }).onOk(() => {
      void runAction(
        'close',
        () => store.close(run.id),
        () => refreshAfterAction(run.id),
      );
    });
  }

  // Risk-rules dialog: configured view is editable, effective view read-only.
  const rulesDialogOpen = ref(false);
  const rulesSaving = ref(false);
  const configuredRules = computed(() => riskRules.value?.configured ?? EMPTY_RULES);
  const effectiveRules = computed(() => riskRules.value?.effective ?? EMPTY_RULES);

  function openRulesDialog() {
    rulesDialogOpen.value = true;
    void store.loadRiskRules();
  }

  async function saveRiskRulesAction(rules: SIRiskRules) {
    if (rulesSaving.value) return;
    rulesSaving.value = true;
    try {
      await store.saveRiskRules(rules);
      $q.notify({ type: 'positive', message: t('selfImprovementPage.rules.saved') });
      rulesDialogOpen.value = false;
    } catch (e) {
      notifyError(e);
    } finally {
      rulesSaving.value = false;
    }
  }

  onMounted(() => {
    // GetStatus 探测与业务加载并行：disabled 时业务请求返回 503 SELF_IMPROVEMENT
    // 并置位 featureDisabled；statusInfo 用于 enabled 态的前置条件自检。
    void store.loadStatus();
    void loadRows();
    void store.loadOutcomeStats();
  });

  return {
    // list
    status,
    riskLevel,
    triggerSource,
    page,
    pageSize,
    rows: runs,
    total,
    loading,
    error,
    errorMessage,
    errorRetryable,
    pageMax,
    statusOptions,
    riskOptions,
    triggerOptions,
    loadRows,
    resetFilters,
    // feature availability / preflight (P5.5)
    featureDisabled,
    statusInfo,
    prereqLlmMissing,
    prereqRepoInvalid,
    recheckAction,
    // stats
    outcomeStats,
    statsLoading,
    statsFailed,
    // detail drawer
    drawerOpen,
    detail,
    detailLoading,
    openDetail,
    // actions
    actionRunning,
    approveRunAction,
    rejectRunAction,
    rollbackRunAction,
    closeRunAction,
    // risk rules
    rulesDialogOpen,
    rulesSaving,
    rulesLoading,
    configuredRules,
    effectiveRules,
    openRulesDialog,
    saveRiskRulesAction,
  };
}
