import { onMounted } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useMonitorStore } from '../../stores/monitor/index';
import type { MonitorAlertRule } from './types';

export function useMonitorAlertRules() {
  const $q = useQuasar();
  const { t } = useI18n();
  const monitorStore = useMonitorStore();
  const { alertRules, alertRulesLoading, alertRulesSaving, alertChannelOptions, alertMetrics, alertMetricsLoading } =
    storeToRefs(monitorStore);

  async function load() {
    try {
      await Promise.all([monitorStore.loadAlertRules(), monitorStore.loadAlertMetrics()]);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('monitorPage.alerts.loadFailed') });
    }
  }

  async function save(rules: MonitorAlertRule[]) {
    try {
      await monitorStore.saveAlertRules(rules);
      $q.notify({ type: 'positive', message: t('monitorPage.alerts.saved') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('monitorPage.alerts.saveFailed') });
    }
  }

  /** 删除已持久化规则前的破坏性操作确认（红线 #4：对话框只在 composable/Page 层）。 */
  function confirmRemoveRule(ruleName: string): Promise<boolean> {
    return new Promise<boolean>((resolve) => {
      $q.dialog({
        title: t('monitorPage.alerts.removeConfirmTitle'),
        message: t('monitorPage.alerts.removeConfirmMessage', { name: ruleName }),
        cancel: { label: t('common.cancel'), flat: true, noCaps: true },
        ok: { label: t('monitorPage.alerts.rule.delete'), noCaps: true, color: 'negative' },
        persistent: true,
      })
        .onOk(() => resolve(true))
        .onCancel(() => resolve(false))
        .onDismiss(() => resolve(false));
    });
  }

  onMounted(() => {
    void monitorStore.loadAlertChannelOptions();
    void load();
  });

  return {
    rules: alertRules,
    loading: alertRulesLoading,
    saving: alertRulesSaving,
    channelOptions: alertChannelOptions,
    metrics: alertMetrics,
    metricsLoading: alertMetricsLoading,
    load,
    save,
    confirmRemoveRule,
  };
}
