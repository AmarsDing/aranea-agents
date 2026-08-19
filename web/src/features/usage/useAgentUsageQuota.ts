import { computed, ref, watch, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import {
  checkUsageQuota,
  listBudgetAlerts,
  microUsdToUsd,
  setBudgetAlert,
  setUsageQuota,
  type UsageQuotaCheck,
} from './quotaApi';

/** 后端 CheckQuota reason 机器码 → i18n 键（契约见 internal/biz/usage QuotaCheckReason*）。 */
const QUOTA_REASON_KEYS: Record<string, string> = {
  no_quota: 'usageQuota.reasonNoQuota',
  quota_disabled: 'usageQuota.reasonQuotaDisabled',
  quota_exceeded: 'usageQuota.reasonQuotaExceeded',
  within_quota: 'usageQuota.reasonWithinQuota',
};

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

/** YYYY-MM-DD 且为真实日期（拒绝 2026-02-30 这类被 JS 滚动的伪日期）。 */
function isValidDateStr(v: string): boolean {
  if (!DATE_RE.test(v)) return false;
  const d = new Date(`${v}T00:00:00Z`);
  return !Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === v;
}

/** Per-agent monthly USD cap; enforced in Chat before each turn (scope_type=agent). */
export function useAgentUsageQuota(agentId: Ref<string>) {
  const $q = useQuasar();
  const { t } = useI18n();

  /** 周期输入交叉校验：双空合法（后端默认当自然月）；否则须成对、格式合法且 start<=end。返回 '' 表示通过。 */
  function validatePeriodInput(start: string, end: string): string {
    const ps = start.trim();
    const pe = end.trim();
    if (!ps && !pe) return '';
    if (!ps || !pe) return t('usageQuota.periodPairRequired');
    if (!isValidDateStr(ps) || !isValidDateStr(pe)) return t('usageQuota.periodInvalidDate');
    if (ps > pe) return t('usageQuota.periodOrderInvalid');
    return '';
  }

  // v-model.number 解析失败时保留原始字符串（如 "-"/"e"），故类型含 string，守卫须按运行时形态写。
  const monthlyUsd = ref<number | string | null>(null);
  const periodStart = ref('');
  const periodEnd = ref('');
  const saving = ref(false);
  const checking = ref(false);
  const error = ref('');
  const check = ref<UsageQuotaCheck | null>(null);
  const alertRatioPct = ref(80);
  const alertEnabled = ref(true);
  const alertSaving = ref(false);

  const reasonText = computed(() => {
    const c = check.value;
    if (!c) return '';
    return QUOTA_REASON_KEYS[c.reason] ? t(QUOTA_REASON_KEYS[c.reason]) : c.reason;
  });
  /** 有有效限额时才展示 已消耗/剩余/月上限 数值区（否则后端不统计，数值恒 0 无意义）。 */
  const hasActiveQuota = computed(() => (check.value?.quota?.monthly_micro_usd ?? 0) > 0);

  // 输入规则（q-input :rules，true=通过）；保存前仍会整体复核，双保险。
  const monthlyUsdRules = [
    (v: number | null | string) =>
      v === null || v === '' || (typeof v === 'number' && !Number.isNaN(v) && v >= 0) || t('usageQuota.monthlyUsdInvalid'),
  ];
  // 交叉规则挂到开始/结束两个字段：Quasar 只在本字段变化/失焦时重验，单边挂载会导致常见填写顺序下无字段级提示。
  const periodCrossRule = () =>
    !periodStart.value.trim() ||
    !periodEnd.value.trim() ||
    validatePeriodInput(periodStart.value, periodEnd.value) === '' ||
    t('usageQuota.periodOrderInvalid');
  const periodFormatRule = (v: string) => !v?.trim() || isValidDateStr(v.trim()) || t('usageQuota.periodFormatInvalid');
  const periodStartRules = [periodFormatRule, periodCrossRule];
  const periodEndRules = [periodFormatRule, periodCrossRule];
  const alertRatioRules = [
    (v: number | null | string) =>
      (typeof v === 'number' && !Number.isNaN(v) && v >= 1 && v <= 100) || t('usageQuota.alertRatioInvalid'),
  ];

  async function loadQuota() {
    const id = agentId.value.trim();
    if (!id) {
      check.value = null;
      return;
    }
    error.value = '';
    // check 响应的 quota 已含惰性滚动后的最新周期；未配置时为零值 quota。
    // 直接用它填充表单，避免额外 GET 在未配置时产生 404 噪音。
    await runCheck();
    if (!check.value) return; // 检查失败保留现状（error 已由 runCheck 记录）
    const q = check.value.quota;
    monthlyUsd.value = q && q.monthly_micro_usd > 0 ? q.monthly_micro_usd / 1_000_000 : null;
    periodStart.value = q?.period_start || '';
    periodEnd.value = q?.period_end || '';
  }

  async function runCheck() {
    const id = agentId.value.trim();
    if (!id) {
      check.value = null;
      return;
    }
    checking.value = true;
    try {
      check.value = await checkUsageQuota('agent', id);
    } catch (e) {
      check.value = null;
      error.value = e instanceof Error ? e.message : t('usageQuota.checkFailed');
    } finally {
      checking.value = false;
    }
  }

  async function saveQuota() {
    const id = agentId.value.trim();
    if (!id) {
      $q.notify({ type: 'warning', message: t('usageQuota.agentNotLoaded') });
      return;
    }
    // 守卫与 monthlyUsdRules 对齐：非数字/NaN/负数一律拦截，避免垃圾输入被静默折算为 0（禁用配额）。
    const usd = monthlyUsd.value;
    if (usd !== null && usd !== '' && (typeof usd !== 'number' || Number.isNaN(usd) || usd < 0)) {
      $q.notify({ type: 'warning', message: t('usageQuota.monthlyUsdInvalid') });
      return;
    }
    const periodErr = validatePeriodInput(periodStart.value, periodEnd.value);
    if (periodErr) {
      $q.notify({ type: 'warning', message: periodErr });
      return;
    }
    saving.value = true;
    error.value = '';
    try {
      const micro = typeof usd === 'number' && usd > 0 ? Math.round(usd * 1_000_000) : 0;
      await setUsageQuota('agent', id, {
        monthly_micro_usd: micro,
        period_start: periodStart.value.trim(),
        period_end: periodEnd.value.trim(),
      });
      $q.notify({ type: 'positive', message: t('usageQuota.quotaSaved') });
      await loadQuota();
    } catch (e) {
      error.value = e instanceof Error ? e.message : t('usageQuota.saveFailed');
      $q.notify({ type: 'negative', message: error.value });
    } finally {
      saving.value = false;
    }
  }

  async function loadAlert() {
    const id = agentId.value.trim();
    if (!id) return;
    try {
      const items = await listBudgetAlerts('agent', id);
      const primary = items.find((a) => a.enabled) ?? items[0];
      if (primary) {
        alertRatioPct.value = Math.round((primary.alert_ratio || 0.8) * 100);
        alertEnabled.value = primary.enabled;
      }
    } catch {
      // keep defaults
    }
  }

  async function saveAlert() {
    const id = agentId.value.trim();
    if (!id) {
      $q.notify({ type: 'warning', message: t('usageQuota.agentNotLoaded') });
      return;
    }
    const pct = alertRatioPct.value;
    if (typeof pct !== 'number' || Number.isNaN(pct) || pct < 1 || pct > 100) {
      $q.notify({ type: 'warning', message: t('usageQuota.alertRatioInvalid') });
      return;
    }
    alertSaving.value = true;
    error.value = '';
    try {
      await setBudgetAlert('agent', id, {
        alert_ratio: pct / 100,
        enabled: alertEnabled.value,
      });
      $q.notify({ type: 'positive', message: t('usageQuota.thresholdSaved') });
      await loadAlert();
    } catch (e) {
      error.value = e instanceof Error ? e.message : t('usageQuota.saveThresholdFailed');
      $q.notify({ type: 'negative', message: error.value });
    } finally {
      alertSaving.value = false;
    }
  }

  watch(
    agentId,
    (id) => {
      if (id.trim()) {
        void loadQuota();
        void loadAlert();
      } else {
        check.value = null;
        monthlyUsd.value = null;
        periodStart.value = '';
        periodEnd.value = '';
      }
    },
    { immediate: true },
  );

  return {
    monthlyUsd,
    periodStart,
    periodEnd,
    saving,
    checking,
    error,
    check,
    reasonText,
    hasActiveQuota,
    alertRatioPct,
    alertEnabled,
    alertSaving,
    monthlyUsdRules,
    periodStartRules,
    periodEndRules,
    alertRatioRules,
    microUsdToUsd,
    loadQuota,
    saveQuota,
    saveAlert,
  };
}
