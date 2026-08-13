import { computed, ref, watch, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import {
  checkUsageQuota,
  getUsageQuota,
  listBudgetAlerts,
  microUsdToUsd,
  setBudgetAlert,
  setUsageQuota,
  type UsageQuotaCheck,
} from './quotaApi';

/** 后端 CheckQuota reason 机器码 → 中文文案（契约见 internal/biz/usage QuotaCheckReason*）。 */
const QUOTA_REASON_TEXT: Record<string, string> = {
  no_quota: '未配置配额，对话不受限制',
  quota_disabled: '未启用限额（月预算为 0），对话不受限制',
  quota_exceeded: '本月消耗已达配额上限，新对话已被拦截',
  within_quota: '本月消耗在配额范围内',
};

/** Per-agent monthly USD cap; enforced in Chat before each turn (scope_type=agent). */
export function useAgentUsageQuota(agentId: Ref<string>) {
  const $q = useQuasar();
  const { t } = useI18n();
  const monthlyUsd = ref<number | null>(null);
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
    return QUOTA_REASON_TEXT[c.reason] ?? c.reason;
  });
  /** 有有效限额时才展示 已消耗/剩余/月上限 数值区（否则后端不统计，数值恒 0 无意义）。 */
  const hasActiveQuota = computed(() => (check.value?.quota?.monthly_micro_usd ?? 0) > 0);

  async function loadQuota() {
    const id = agentId.value.trim();
    if (!id) {
      check.value = null;
      return;
    }
    error.value = '';
    // 先校验：后端会将过期周期惰性滚动为当自然月；随后读取拿到滚动后的最新周期。
    await runCheck();
    try {
      const q = await getUsageQuota('agent', id);
      monthlyUsd.value = q.monthly_micro_usd > 0 ? q.monthly_micro_usd / 1_000_000 : null;
      periodStart.value = q.period_start || '';
      periodEnd.value = q.period_end || '';
    } catch {
      monthlyUsd.value = null;
      periodStart.value = '';
      periodEnd.value = '';
    }
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
      error.value = e instanceof Error ? e.message : '检查配额失败';
    } finally {
      checking.value = false;
    }
  }

  async function saveQuota() {
    const id = agentId.value.trim();
    if (!id) {
      $q.notify({ type: 'warning', message: 'Agent 未加载' });
      return;
    }
    saving.value = true;
    error.value = '';
    try {
      const micro = monthlyUsd.value != null && monthlyUsd.value > 0 ? Math.round(monthlyUsd.value * 1_000_000) : 0;
      await setUsageQuota('agent', id, {
        monthly_micro_usd: micro,
        period_start: periodStart.value.trim(),
        period_end: periodEnd.value.trim(),
      });
      $q.notify({ type: 'positive', message: '配额已保存' });
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
    alertSaving.value = true;
    error.value = '';
    try {
      await setBudgetAlert('agent', id, {
        alert_ratio: Math.min(1, Math.max(0.01, alertRatioPct.value / 100)),
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
    microUsdToUsd,
    loadQuota,
    runCheck,
    saveQuota,
    saveAlert,
  };
}
