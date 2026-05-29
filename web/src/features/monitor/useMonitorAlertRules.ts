import { onMounted } from "vue";
import { storeToRefs } from "pinia";
import { useQuasar } from "quasar";
import { useMonitorStore } from "../../stores/monitor/index";
import type { MonitorAlertRule } from "./types";

export function useMonitorAlertRules() {
  const $q = useQuasar();
  const monitorStore = useMonitorStore();
  const { alertRules, alertRulesLoading, alertRulesSaving, alertChannelOptions } = storeToRefs(monitorStore);

  async function load() {
    try {
      await monitorStore.loadAlertRules();
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载失败" });
    }
  }

  async function save(rules: MonitorAlertRule[]) {
    try {
      await monitorStore.saveAlertRules(rules);
      $q.notify({ type: "positive", message: "告警规则已保存" });
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "保存失败" });
    }
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
    load,
    save
  };
}
