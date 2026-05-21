<template>
  <q-page class="overview-page">
    <div class="overview-page__shell">
      <OverviewPageHero>
        <template #actions>
          <OverviewMonitorQuickLinks />
          <q-btn
            unelevated
            no-caps
            class="overview-primary-btn"
            icon="receipt_long"
            label="查看明细"
            :to="{ path: '/usage/events', query: { range: filters.range || '30d' } }"
          />
        </template>
      </OverviewPageHero>

      <OverviewRunnerMetrics />

      <q-card flat class="overview-filter-card q-mb-md">
        <q-card-section class="q-pb-sm">
          <div class="text-subtitle2 text-weight-medium overview-section-label">筛选</div>
        </q-card-section>
        <q-card-section class="q-pt-none app-form-field-grid items-end">
          <q-select
            v-model="filters.range"
            dense
            outlined
            emit-value
            map-options
            label="时间范围"
            :options="rangeOptions"
            @update:model-value="loadOverview"
          />
          <q-input
            v-model="filters.provider_code"
            dense
            outlined
            clearable
            label="Provider"
            debounce="300"
            @update:model-value="loadOverview"
          />
          <q-input
            v-model="filters.model_api_id"
            dense
            outlined
            clearable
            label="模型"
            debounce="300"
            @update:model-value="loadOverview"
          />
          <q-select
            v-model="filters.status"
            dense
            outlined
            clearable
            emit-value
            map-options
            label="状态"
            :options="statusOptions"
            @update:model-value="loadOverview"
          />
          <q-select
            v-model="trendGranularity"
            dense
            outlined
            emit-value
            map-options
            label="趋势粒度"
            :options="granularityOptions"
            @update:model-value="loadOverview"
          />
        </q-card-section>
      </q-card>

      <div class="overview-content" :class="{ 'overview-content--loading': loading }">
        <q-inner-loading :showing="loading" color="primary">
          <q-spinner size="42px" />
        </q-inner-loading>

        <UsageMetricCards :overview="overview" />

        <div class="row q-col-gutter-md overview-section">
          <div class="col-12 col-lg-8">
            <UsageTrendChart :points="overview?.trends ?? []" :hourly="trendGranularity === 'hour'" />
          </div>
          <div class="col-12 col-lg-4">
            <q-card flat class="overview-panel overview-summary-panel">
              <q-card-section>
                <div class="text-h6 overview-section-title">区间摘要</div>
                <div class="text-caption overview-section-caption">当前筛选范围内的总量</div>
              </q-card-section>
              <q-list dense class="overview-summary-list">
                <q-item>
                  <q-item-section class="overview-summary-label">总调用</q-item-section>
                  <q-item-section side class="overview-summary-value">{{ formatCount(overview?.range.call_count) }}</q-item-section>
                </q-item>
                <q-item>
                  <q-item-section class="overview-summary-label">总 Token</q-item-section>
                  <q-item-section side class="overview-summary-value">{{ formatCount(overview?.range.total_tokens) }}</q-item-section>
                </q-item>
                <q-item>
                  <q-item-section class="overview-summary-label">总费用</q-item-section>
                  <q-item-section side class="overview-summary-value">{{ formatMoney(overview?.range.total_cost_micro_usd) }}</q-item-section>
                </q-item>
                <q-item>
                  <q-item-section class="overview-summary-label">成功率</q-item-section>
                  <q-item-section side class="overview-summary-value">{{ formatPercent(overview?.range.success_rate) }}</q-item-section>
                </q-item>
              </q-list>
            </q-card>
          </div>
        </div>

        <UsageBreakdownCharts :top-models="overview?.top_models ?? []" />

        <div class="row q-col-gutter-md overview-section">
          <div class="col-12 col-lg-6">
            <UsageTopModels :rows="overview?.top_models ?? []" />
          </div>
          <div class="col-12 col-lg-6">
            <UsageTopAgents :rows="overview?.top_agents ?? []" />
          </div>
        </div>

        <div v-if="(overview?.inefficient_models?.length ?? 0) > 0" class="overview-section">
          <UsageInefficientModels :rows="overview?.inefficient_models ?? []" />
        </div>

        <div class="overview-section">
          <UsageAnomalyList :rows="overview?.anomalies ?? []" />
        </div>
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { defineAsyncComponent, onMounted } from "vue";
import { useOverviewPage } from "../features/usage/useOverviewPage";
import OverviewPageHero from "../components/usage/OverviewPageHero.vue";
import OverviewMonitorQuickLinks from "../components/usage/OverviewMonitorQuickLinks.vue";
import OverviewRunnerMetrics from "../components/usage/OverviewRunnerMetrics.vue";
import UsageAnomalyList from "../components/usage/UsageAnomalyList.vue";
import UsageInefficientModels from "../components/usage/UsageInefficientModels.vue";
import UsageMetricCards from "../components/usage/UsageMetricCards.vue";
import UsageTopAgents from "../components/usage/UsageTopAgents.vue";
import UsageTopModels from "../components/usage/UsageTopModels.vue";

const UsageTrendChart = defineAsyncComponent(() => import("../components/usage/UsageTrendChart.vue"));
const UsageBreakdownCharts = defineAsyncComponent(() => import("../components/usage/UsageBreakdownCharts.vue"));

const {
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
} = useOverviewPage();

onMounted(() => void loadOverview());
</script>
