<template>
  <q-page class="app-page-cream q-pa-sm q-pa-md-md">
    <div class="dashboard-glass-shell q-pa-sm q-pa-md-md">
      <q-card flat class="dashboard-glass-card q-mb-md">
        <q-card-section class="row items-center q-col-gutter-md">
          <div class="col-12 col-md">
            <div class="text-h5 text-weight-bold text-cream-text">模型消耗概览</div>
            <div class="text-caption text-cream-muted">Token、费用、调用次数与异常请求运营看板</div>
          </div>
          <div class="col-12 col-sm-6 col-md-2">
            <q-select v-model="filters.range" dense outlined emit-value map-options label="时间范围" :options="rangeOptions" @update:model-value="loadOverview" />
          </div>
          <div class="col-12 col-sm-6 col-md-2">
            <q-input v-model="filters.provider_code" dense outlined clearable label="Provider" debounce="300" @update:model-value="loadOverview" />
          </div>
          <div class="col-12 col-sm-6 col-md-2">
            <q-input v-model="filters.model_api_id" dense outlined clearable label="模型" debounce="300" @update:model-value="loadOverview" />
          </div>
          <div class="col-12 col-sm-6 col-md-2">
            <q-select v-model="filters.status" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" @update:model-value="loadOverview" />
          </div>
        </q-card-section>
      </q-card>

      <q-inner-loading :showing="loading">
        <q-spinner color="primary" size="42px" />
      </q-inner-loading>

      <UsageMetricCards :overview="overview" />

      <div class="row q-col-gutter-md q-mt-md">
        <div class="col-12 col-lg-8">
          <UsageTrendPanel :points="overview?.trends ?? []" />
        </div>
        <div class="col-12 col-lg-4">
          <q-card flat bordered class="usage-summary-card">
            <q-card-section>
              <div class="text-h6">区间摘要</div>
              <div class="text-caption text-grey-7">当前筛选范围内的总量</div>
            </q-card-section>
            <q-list dense>
              <q-item>
                <q-item-section>总调用</q-item-section>
                <q-item-section side>{{ formatCount(overview?.range.call_count) }}</q-item-section>
              </q-item>
              <q-item>
                <q-item-section>总 Token</q-item-section>
                <q-item-section side>{{ formatCount(overview?.range.total_tokens) }}</q-item-section>
              </q-item>
              <q-item>
                <q-item-section>总费用</q-item-section>
                <q-item-section side>{{ formatMoney(overview?.range.total_cost_micro_usd) }}</q-item-section>
              </q-item>
              <q-item>
                <q-item-section>成功率</q-item-section>
                <q-item-section side>{{ formatPercent(overview?.range.success_rate) }}</q-item-section>
              </q-item>
            </q-list>
          </q-card>
        </div>
      </div>

      <div class="row q-col-gutter-md q-mt-md">
        <div class="col-12 col-lg-6">
          <UsageTopModels :rows="overview?.top_models ?? []" />
        </div>
        <div class="col-12 col-lg-6">
          <UsageTopAgents :rows="overview?.top_agents ?? []" />
        </div>
      </div>

      <div class="q-mt-md">
        <UsageAnomalyList :rows="overview?.anomalies ?? []" />
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { getModelUsageOverview, type ModelUsageOverview, type ModelUsageQuery } from "../features/usage/api";
import UsageAnomalyList from "../components/usage/UsageAnomalyList.vue";
import UsageMetricCards from "../components/usage/UsageMetricCards.vue";
import UsageTopAgents from "../components/usage/UsageTopAgents.vue";
import UsageTopModels from "../components/usage/UsageTopModels.vue";
import UsageTrendPanel from "../components/usage/UsageTrendPanel.vue";

const overview = ref<ModelUsageOverview | null>(null);
const loading = ref(false);
const filters = reactive<ModelUsageQuery>({
  range: "30d",
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

onMounted(loadOverview);

async function loadOverview() {
  loading.value = true;
  try {
    overview.value = await getModelUsageOverview(filters);
  } finally {
    loading.value = false;
  }
}

function formatCount(value?: number) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value ?? 0);
}

function formatMoney(value?: number) {
  return `$${((value ?? 0) / 1_000_000).toFixed(4)}`;
}

function formatPercent(value?: number) {
  return `${Math.round((value ?? 0) * 100)}%`;
}
</script>

<style scoped>
.usage-summary-card {
  border-radius: 18px;
  min-height: 100%;
}
</style>
