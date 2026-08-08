import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  approveRun,
  closeRun,
  getOutcomeStats,
  getRiskRules,
  getRun,
  getStatus,
  listRuns,
  rejectRun,
  rollbackRun,
  updateRiskRules,
} from '../../features/self-improvement/api';
import type {
  SIOutcomeStats,
  SIRiskRules,
  SIRiskRulesView,
  SIRun,
  SIRunDetail,
  SIRunFilter,
  SIStatus,
} from '../../features/self-improvement/types';

/** 错误类别（P5.5 人性化映射；页面据此选 i18n 文案，unknown 时回退原始 message）。 */
export type SIErrorKind = '' | 'forbidden' | 'legacy' | 'unavailable' | 'unknown';

type AxiosLikeError = {
  response?: { status?: number; data?: { reason?: string } };
};

function siErrorInfo(e: unknown): { status?: number; reason?: string } {
  const err = e as AxiosLikeError | undefined;
  return { status: err?.response?.status, reason: err?.response?.data?.reason };
}

/** 503 + reason=SELF_IMPROVEMENT_* = 功能未启用（路由始终注册后的结构化信号；
 *  后端 apierror.ToKratos 将 Domain+Code 拼成 SELF_IMPROVEMENT_UNAVAILABLE）。 */
function isDisabledError(e: unknown): boolean {
  const { status, reason } = siErrorInfo(e);
  return status === 503 && (reason ?? '').startsWith('SELF_IMPROVEMENT');
}

// Self-improvement console store (73-self-iteration-v3, design §八):
// runs list + active detail + Learn-stage outcome stats; mutations reload the
// affected run through the returned promise's caller (page composable).
export const useSelfImprovementStore = defineStore('selfImprovement', () => {
  const runs = ref<SIRun[]>([]);
  const total = ref(0);
  const detail = ref<SIRunDetail | null>(null);
  const outcomeStats = ref<SIOutcomeStats | null>(null);
  const loading = ref(false);
  const detailLoading = ref(false);
  const statsLoading = ref(false);
  const error = ref<string | null>(null);
  const errorKind = ref<SIErrorKind>('');
  /** 业务端点返回 503 SELF_IMPROVEMENT（feature disabled）后置位。 */
  const featureDisabled = ref(false);
  /** GetStatus 探测结果；null = 探测失败（旧后端/网络异常）。 */
  const statusInfo = ref<SIStatus | null>(null);
  /** 统计接口失败（区别于真实的 0 数据）→ 面板显示「不可用」。 */
  const statsFailed = ref(false);

  function captureError(e: unknown) {
    if (isDisabledError(e)) {
      featureDisabled.value = true;
      error.value = null;
      errorKind.value = '';
      return;
    }
    const { status, reason } = siErrorInfo(e);
    if (status === 403) errorKind.value = 'forbidden';
    // 404 双义：路由不存在（旧后端，无域 reason）→ legacy；域内 NOT_FOUND
    // （如 run 已删除）→ unknown 回退原始 message，避免误报「后端版本过旧」。
    else if (status === 404 && !(reason ?? '').startsWith('SELF_IMPROVEMENT')) errorKind.value = 'legacy';
    else if (status !== undefined && status >= 500) errorKind.value = 'unavailable';
    else errorKind.value = 'unknown';
    error.value = e instanceof Error ? e.message : String(e);
  }

  /** GetStatus 探测（disabled 也可调用）；失败仅置 null，不抢主错误条。 */
  async function loadStatus() {
    try {
      statusInfo.value = await getStatus();
      // 探测成功即权威：enabled=false 必须置位 featureDisabled——否则 recheck
      // 后后端仍 disabled 时（不再调业务端点、收不到 503）空态指引会消失。
      featureDisabled.value = !statusInfo.value.enabled;
    } catch {
      statusInfo.value = null;
    }
  }

  async function loadRuns(filter: SIRunFilter) {
    loading.value = true;
    error.value = null;
    errorKind.value = '';
    try {
      const res = await listRuns(filter);
      runs.value = res.items;
      total.value = res.total;
    } catch (e: unknown) {
      captureError(e);
    } finally {
      loading.value = false;
    }
  }

  async function loadRun(id: string): Promise<SIRunDetail | null> {
    detailLoading.value = true;
    error.value = null;
    errorKind.value = '';
    try {
      const res = await getRun(id);
      detail.value = res;
      return res;
    } catch (e: unknown) {
      captureError(e);
      return null;
    } finally {
      detailLoading.value = false;
    }
  }

  async function loadOutcomeStats() {
    statsLoading.value = true;
    statsFailed.value = false;
    try {
      outcomeStats.value = await getOutcomeStats();
    } catch (e: unknown) {
      if (isDisabledError(e)) {
        featureDisabled.value = true;
      } else {
        statsFailed.value = true; // 静默降级为「不可用」，不抢主错误条
      }
      outcomeStats.value = null;
    } finally {
      statsLoading.value = false;
    }
  }

  const riskRules = ref<SIRiskRulesView | null>(null);
  const rulesLoading = ref(false);

  async function loadRiskRules() {
    rulesLoading.value = true;
    error.value = null;
    errorKind.value = '';
    try {
      riskRules.value = await getRiskRules();
    } catch (e: unknown) {
      captureError(e);
    } finally {
      rulesLoading.value = false;
    }
  }

  // saveRiskRules propagates errors to the caller (page composable notifies).
  async function saveRiskRules(rules: SIRiskRules) {
    riskRules.value = await updateRiskRules(rules);
  }

  function patchRow(id: string, status: SIRun['status']) {
    const idx = runs.value.findIndex((r) => r.id === id);
    if (idx >= 0) {
      runs.value[idx] = { ...runs.value[idx], status };
    }
    if (detail.value?.id === id) {
      detail.value = { ...detail.value, status };
    }
  }

  async function approve(id: string, reason?: string) {
    await approveRun(id, reason);
    patchRow(id, 'applying');
  }

  async function reject(id: string, reason: string) {
    await rejectRun(id, reason);
    patchRow(id, 'rejected');
  }

  async function rollback(id: string, reason?: string) {
    await rollbackRun(id, reason);
    patchRow(id, 'rolled_back');
  }

  async function close(id: string, reason?: string) {
    await closeRun(id, reason);
    patchRow(id, 'closed');
  }

  /** 「重新检测」：重置 disabled 标记，重新探测状态并按需加载数据。 */
  async function recheck(filter: SIRunFilter) {
    featureDisabled.value = false;
    await loadStatus();
    if (statusInfo.value?.enabled) {
      await Promise.all([loadRuns(filter), loadOutcomeStats()]);
    }
  }

  return {
    runs,
    total,
    detail,
    outcomeStats,
    loading,
    detailLoading,
    statsLoading,
    error,
    errorKind,
    featureDisabled,
    statusInfo,
    statsFailed,
    loadStatus,
    loadRuns,
    loadRun,
    loadOutcomeStats,
    recheck,
    riskRules,
    rulesLoading,
    loadRiskRules,
    saveRiskRules,
    approve,
    reject,
    rollback,
    close,
  };
});
