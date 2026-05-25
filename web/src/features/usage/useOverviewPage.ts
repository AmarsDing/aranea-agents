import { reactive, ref } from "vue";
import { useRoute } from "vue-router";
import { storeToRefs } from "pinia";
import { useUsageStore } from "../../stores/usage";
import type { ModelUsageQuery } from "./types";
import { formatUsdFromMicro } from "./moneyFormat";

const VALID_RANGES = new Set(["today", "7d", "30d", "month"]);

export function useOverviewPage() {
  const route = useRoute();
  const usageStore = useUsageStore();
  const { overview, loading } = storeToRefs(usageStore);
  const trendGranularity = ref<"day" | "hour">("day");
  const initialRange = String(route.query.range || "30d");
  const filters = reactive<ModelUsageQuery>({
    range: VALID_RANGES.has(initialRange) ? initialRange : "30d",
    provider_code: "",
    model_api_id: "",
    status: ""
  });

  const rangeOptions = [
    { label: "今日", value: "today" },
    { label: "7 天", value: "7d" },
    { label: "30 天", value: "30d" },
    { label: "本月", value: "month" }
  ];

  const statusOptions = [
    { label: "成功", value: "success" },
    { label: "失败", value: "failed" },
    { label: "取消", value: "cancelled" },
    { label: "超时", value: "timeout" }
  ];

  const granularityOptions = [
    { label: "按天", value: "day" },
    { label: "按小时", value: "hour" }
  ];

  async function loadOverview() {
    await usageStore.loadOverview({ ...filters }, trendGranularity.value);
  }

  function formatCount(value?: number) {
    return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value ?? 0);
  }

  function formatMoney(value?: number) {
    return formatUsdFromMicro(value);
  }

  function formatPercent(value?: number) {
    return `${Math.round((value ?? 0) * 100)}%`;
  }

  return {
    overview,
    loading,
    trendGranularity,
    filters,
    rangeOptions,
    statusOptions,
    granularityOptions,
    loadOverview,
    formatCount,
    formatMoney,
    formatPercent
  };
}
