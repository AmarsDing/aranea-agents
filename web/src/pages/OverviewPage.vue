<template>
  <q-page class="app-standard-page overview-page">
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

      <div class="overview-filter-bar">
        <div class="overview-filter-bar__inner">
          <q-select v-model="filters.range" dense outlined emit-value map-options label="时间范围" :options="rangeOptions" class="overview-filter-field" @update:model-value="loadOverview" />
          <q-input v-model="filters.provider_code" dense outlined clearable label="Provider" debounce="300" class="overview-filter-field" @update:model-value="loadOverview" />
          <q-input v-model="filters.model_api_id" dense outlined clearable label="模型" debounce="300" class="overview-filter-field" @update:model-value="loadOverview" />
          <q-select v-model="filters.status" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" class="overview-filter-field" @update:model-value="loadOverview" />
          <q-select v-model="trendGranularity" dense outlined emit-value map-options label="趋势粒度" :options="granularityOptions" class="overview-filter-field" @update:model-value="loadOverview" />
        </div>
      </div>

      <div class="overview-content" :class="{ 'overview-content--loading': loading }">
        <q-inner-loading :showing="loading" color="primary">
          <q-spinner size="42px" />
        </q-inner-loading>

        <q-banner v-if="error" class="bg-negative text-white overview-error" rounded>
          <template #avatar><q-icon name="error" /></template>
          加载概览数据失败：{{ error }}
          <template #action><q-btn flat label="重试" @click="loadOverview" /></template>
        </q-banner>

        <UsageMetricCards :overview="overview" />

        <div class="overview-chart-row">
          <div class="overview-chart-row__main">
            <UsageTrendChart :points="overview?.trends ?? []" :hourly="trendGranularity === 'hour'" />
          </div>
          <div class="overview-chart-row__side">
            <q-card flat class="overview-panel overview-summary-panel">
              <q-card-section>
                <div class="overview-section-title">区间摘要</div>
                <div class="overview-section-caption">当前筛选总量</div>
              </q-card-section>
              <q-separator class="overview-separator" />
              <q-card-section class="q-py-sm">
                <div class="overview-summary-grid">
                  <div class="overview-summary-item">
                    <div class="overview-summary-item__label">总调用</div>
                    <div class="overview-summary-item__value">{{ formatCount(overview?.range.call_count) }}</div>
                  </div>
                  <div class="overview-summary-item">
                    <div class="overview-summary-item__label">总 Token</div>
                    <div class="overview-summary-item__value">{{ formatCount(overview?.range.total_tokens) }}</div>
                  </div>
                  <div class="overview-summary-item">
                    <div class="overview-summary-item__label">总费用</div>
                    <div class="overview-summary-item__value overview-summary-item__value--accent">{{ formatMoney(overview?.range.total_cost_micro_usd) }}</div>
                  </div>
                  <div class="overview-summary-item">
                    <div class="overview-summary-item__label">成功率</div>
                    <div class="overview-summary-item__value">{{ formatPercent(overview?.range.success_rate) }}</div>
                  </div>
                </div>
              </q-card-section>
              <q-separator class="overview-separator" />
              <q-card-section>
                <div class="overview-section-title" style="font-size:0.85rem">Token 构成</div>
                <UsageTokenComposition :summary="overview?.range" />
              </q-card-section>
            </q-card>
          </div>
        </div>

        <div class="overview-mid-row">
          <div class="overview-mid-row__left">
            <UsageBreakdownCharts :top-models="overview?.top_models ?? []" />
          </div>
          <div class="overview-mid-row__right">
            <OverviewProviderHealth />
          </div>
        </div>

        <div class="overview-ranks">
          <UsageTopModels :rows="overview?.top_models ?? []" />
          <UsageTopAgents :rows="overview?.top_agents ?? []" />
        </div>

        <div class="overview-section">
          <OverviewRunnerMetrics />
        </div>

        <div class="overview-alert-stack">
          <UsageInefficientModels v-if="(overview?.inefficient_models?.length ?? 0) > 0" :rows="overview?.inefficient_models ?? []" />
          <UsageAnomalyList :rows="overview?.anomalies ?? []" />
          <UsageFallbackEvents :anomalies="overview?.anomalies ?? []" />
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
import OverviewProviderHealth from "../components/usage/OverviewProviderHealth.vue";
import UsageAnomalyList from "../components/usage/UsageAnomalyList.vue";
import UsageFallbackEvents from "../components/usage/UsageFallbackEvents.vue";
import UsageInefficientModels from "../components/usage/UsageInefficientModels.vue";
import UsageMetricCards from "../components/usage/UsageMetricCards.vue";
import UsageTokenComposition from "../components/usage/UsageTokenComposition.vue";
import UsageTopAgents from "../components/usage/UsageTopAgents.vue";
import UsageTopModels from "../components/usage/UsageTopModels.vue";

const UsageTrendChart = defineAsyncComponent(() => import("../components/usage/UsageTrendChart.vue"));
const UsageBreakdownCharts = defineAsyncComponent(() => import("../components/usage/UsageBreakdownCharts.vue"));

const {
  overview, loading, error,
  trendGranularity, filters,
  rangeOptions, statusOptions, granularityOptions,
  loadOverview, formatCount, formatMoney, formatPercent
} = useOverviewPage();

onMounted(() => void loadOverview());
</script>