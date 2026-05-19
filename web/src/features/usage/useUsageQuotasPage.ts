import { computed, onMounted, ref } from "vue";
import { useQuasar } from "quasar";
import { listAgents } from "../agents/api";
import type { Agent } from "../agents/types";
import { checkUsageQuota, getUsageQuota, microUsdToUsd, setUsageQuota, type UsageQuotaCheck } from "./quotaApi";

export function useUsageQuotasPage() {
  const $q = useQuasar();
  const agents = ref<Agent[]>([]);
  const loadingAgents = ref(false);
  const scopeType = ref("agent");
  const scopeId = ref("");
  const monthlyUsd = ref<number | null>(null);
  const periodStart = ref("");
  const periodEnd = ref("");
  const saving = ref(false);
  const checking = ref(false);
  const error = ref("");
  const check = ref<UsageQuotaCheck | null>(null);

  const selectedAgent = computed(() => agents.value.find((a) => a.id === scopeId.value) ?? null);

  async function loadAgents() {
    loadingAgents.value = true;
    error.value = "";
    try {
      agents.value = await listAgents({ limit: 200, offset: 0 });
    } catch (e) {
      error.value = e instanceof Error ? e.message : "加载 Agent 失败";
    } finally {
      loadingAgents.value = false;
    }
  }

  async function loadQuota() {
    if (!scopeId.value) return;
    error.value = "";
    try {
      const q = await getUsageQuota(scopeType.value, scopeId.value);
      monthlyUsd.value = q.monthly_micro_usd > 0 ? q.monthly_micro_usd / 1_000_000 : null;
      periodStart.value = q.period_start || "";
      periodEnd.value = q.period_end || "";
    } catch {
      monthlyUsd.value = null;
      periodStart.value = "";
      periodEnd.value = "";
    }
    await runCheck();
  }

  async function runCheck() {
    if (!scopeId.value) {
      check.value = null;
      return;
    }
    checking.value = true;
    try {
      check.value = await checkUsageQuota(scopeType.value, scopeId.value);
    } catch (e) {
      check.value = null;
      error.value = e instanceof Error ? e.message : "检查配额失败";
    } finally {
      checking.value = false;
    }
  }

  async function saveQuota() {
    if (!scopeId.value) {
      $q.notify({ type: "warning", message: "请选择 Agent" });
      return;
    }
    saving.value = true;
    error.value = "";
    try {
      const micro = monthlyUsd.value != null && monthlyUsd.value > 0 ? Math.round(monthlyUsd.value * 1_000_000) : 0;
      await setUsageQuota(scopeType.value, scopeId.value, {
        monthly_micro_usd: micro,
        period_start: periodStart.value.trim(),
        period_end: periodEnd.value.trim()
      });
      $q.notify({ type: "positive", message: "配额已保存" });
      await loadQuota();
    } catch (e) {
      error.value = e instanceof Error ? e.message : "保存失败";
      $q.notify({ type: "negative", message: error.value });
    } finally {
      saving.value = false;
    }
  }

  function onAgentChange(id: string) {
    scopeId.value = id;
    void loadQuota();
  }

  onMounted(() => {
    void loadAgents();
  });

  return {
    agents,
    loadingAgents,
    scopeType,
    scopeId,
    monthlyUsd,
    periodStart,
    periodEnd,
    saving,
    checking,
    error,
    check,
    selectedAgent,
    microUsdToUsd,
    loadAgents,
    loadQuota,
    runCheck,
    saveQuota,
    onAgentChange
  };
}
