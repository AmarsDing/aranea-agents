<template>
  <q-page class="app-standard-page overview-page">
    <div class="overview-page__shell">
      <CommandCenterHero
        :username="username"
        :active-agent-count="agentStats.active"
        :provider-count="providerCount"
        :category-count="categoryCount"
        :team-count="teamCount"
        :today-session-count="sessionActiveCount"
        :today-token-count="overview?.today?.total_tokens ?? 0"
        @open-token-trend="openTokenTrendDialog"
      >
        <template #actions>
          <OverviewMonitorQuickLinks />
          <q-btn
            outline
            no-caps
            class="overview-primary-btn"
            icon="warning_amber"
            :label="t('overviewPage.btnAlerts')"
            @click="scrollToAlerts"
          />
          <q-btn
            unelevated
            no-caps
            class="overview-primary-btn"
            icon="receipt_long"
            :label="t('overviewPage.btnDetails')"
            :to="eventsPageQuery"
          />
        </template>
      </CommandCenterHero>

      <CommandCenterQuickActions />

      <CommandCenterStatusPanels
        :agent-stats="agentStats"
        :session-active-count="sessionActiveCount"
        :session-sparkline="sessionSparkline"
        :provider-health="providerHealthSummary"
        :runner-stats="runnerStats"
        :loading="loading"
      />

      <div class="overview-filter-bar">
        <div class="overview-filter-bar__inner">
          <q-select v-model="filters.range" dense outlined emit-value map-options :label="t('overviewPage.filterRange')" :options="rangeOptions" class="overview-filter-field" @update:model-value="loadOverview" />
          <q-select v-model="filters.provider_code" dense outlined clearable emit-value map-options :label="t('overviewPage.filterProvider')" :options="providerOptions" class="overview-filter-field" @update:model-value="onProviderChange" />
          <q-select v-model="filters.model_api_id" dense outlined clearable emit-value map-options :label="t('overviewPage.filterModel')" :options="modelOptions" class="overview-filter-field" @update:model-value="loadOverview" />
          <q-select v-model="filters.status" dense outlined clearable emit-value map-options :label="t('overviewPage.filterStatus')" :options="statusOptions" class="overview-filter-field" @update:model-value="loadOverview" />
          <q-select v-model="trendGranularity" dense outlined emit-value map-options :label="t('overviewPage.filterGranularity')" :options="granularityOptions" class="overview-filter-field" @update:model-value="loadOverview" />
        </div>
      </div>

      <div class="overview-content" :class="{ 'overview-content--loading': loading }">
        <q-inner-loading :showing="loading" color="primary">
          <q-spinner size="42px" />
        </q-inner-loading>

        <q-banner v-if="error" class="bg-negative text-white overview-error" rounded>
          <template #avatar><q-icon name="error" /></template>
          {{ t('overviewPage.errorBanner') }}：{{ error }}
          <template #action><q-btn flat :label="t('overviewPage.btnRetry')" @click="loadOverview" /></template>
        </q-banner>

        <UsageMetricCards :overview="overview" />

        <div class="overview-chart-row">
          <div class="overview-chart-row__main">
            <UsageTrendChart :points="overview?.trends ?? []" :hourly="trendGranularity === 'hour'" />
          </div>
          <div class="overview-chart-row__side">
            <q-card flat class="overview-panel overview-summary-panel">
              <q-card-section>
                <div class="overview-section-title">{{ t('overviewPage.sectionSummary') }}</div>
                <div class="overview-section-caption">{{ t('overviewPage.summaryCaption') }}</div>
              </q-card-section>
              <q-separator class="overview-separator" />
              <q-card-section class="q-py-sm">
                <div class="overview-summary-grid">
                  <div class="overview-summary-item">
                    <div class="overview-summary-item__label">{{ t('overviewPage.totalCalls') }}</div>
                    <div class="overview-summary-item__value">{{ formatCount(overview?.range.call_count) }}</div>
                  </div>
                  <div class="overview-summary-item">
                    <div class="overview-summary-item__label">{{ t('overviewPage.totalTokens') }}</div>
                    <div class="overview-summary-item__value">{{ formatCount(overview?.range.total_tokens) }}</div>
                  </div>
                  <div class="overview-summary-item">
                    <div class="overview-summary-item__label">{{ t('overviewPage.totalCost') }}</div>
                    <div class="overview-summary-item__value overview-summary-item__value--accent">{{ formatMoney(overview?.range.total_cost_micro_usd) }}</div>
                  </div>
                  <div class="overview-summary-item">
                    <div class="overview-summary-item__label">{{ t('overviewPage.successRate') }}</div>
                    <div class="overview-summary-item__value">{{ formatPercent(overview?.range.success_rate) }}</div>
                  </div>
                </div>
              </q-card-section>
              <q-separator class="overview-separator" />
              <q-card-section>
                <div class="overview-section-title" style="font-size:0.85rem">{{ t('overviewPage.tokenComposition') }}</div>
                <UsageTokenComposition :summary="overview?.range" />
              </q-card-section>
            </q-card>
          </div>
        </div>

        <div class="overview-mid-row">
          <UsageModelCostPie :top-models="overview?.top_models ?? []" />
          <UsageProviderCostPie :top-models="overview?.top_models ?? []" />
          <OverviewProviderHealth :models="providerModels" :loading="providerHealthLoading" />
        </div>

        <div class="overview-ranks">
          <UsageTopModels :rows="overview?.top_models ?? []" />
          <UsageTopAgents :rows="overview?.top_agents ?? []" />
        </div>

        <div class="overview-section">
          <OverviewRunnerMetrics
            :metrics="runnerMetrics"
            :loading="runnerLoading"
            :window-minutes="runnerWindowMinutes"
            @update:window-minutes="runnerWindowMinutes = $event; reloadRunnerMetrics()"
            @refresh="reloadRunnerMetrics()"
            @drill="openRunsTab({ tab: 'traces' })"
          />
        </div>

        <div class="overview-alert-stack" ref="alertStackRef">
          <UsageInefficientModels v-if="(overview?.inefficient_models?.length ?? 0) > 0" :rows="overview?.inefficient_models ?? []" />
          <div class="overview-alert-stack__row">
            <UsageAnomalyList :rows="overview?.anomalies ?? []" />
            <UsageFallbackEvents :anomalies="overview?.anomalies ?? []" />
          </div>
        </div>
      </div>
    </div>

    <TokenTrendDialog
      v-model:open="tokenTrendDialogOpen"
      :trend-points="overview?.trends ?? []"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, ref } from "vue";
import { useOverviewPage } from "../features/usage/useOverviewPage";
import CommandCenterHero from "../components/usage/CommandCenterHero.vue";
import CommandCenterStatusPanels from "../components/usage/CommandCenterStatusPanels.vue";
import CommandCenterQuickActions from "../components/usage/CommandCenterQuickActions.vue";
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
import UsageModelCostPie from "../components/usage/UsageModelCostPie.vue";
import UsageProviderCostPie from "../components/usage/UsageProviderCostPie.vue";
import TokenTrendDialog from "../components/usage/TokenTrendDialog.vue";

const UsageTrendChart = defineAsyncComponent(() => import("../components/usage/UsageTrendChart.vue"));

const {
  t,
  overview, loading, error,
  trendGranularity, filters,
  rangeOptions, statusOptions, granularityOptions,
  providerOptions, modelOptions, onProviderChange,
  loadOverview, formatCount, formatMoney, formatPercent,
  providerModels, providerHealthLoading,
  runnerMetrics, runnerLoading, runnerWindowMinutes,
  reloadRunnerMetrics, openRunsTab,
  agentStats, providerCount, categoryCount, teamCount,
  tokenTrendDialogOpen, openTokenTrendDialog,
  username, providerHealthSummary,
  sessionActiveCount, sessionSparkline, runnerStats
} = useOverviewPage();

const alertStackRef = ref<HTMLElement | null>(null);

function scrollToAlerts() {
  alertStackRef.value?.scrollIntoView({ behavior: "smooth" });
}

const eventsPageQuery = computed(() => {
  const query: Record<string, string> = { range: filters.range || "30d" };
  if (filters.provider_code) query.provider_code = filters.provider_code;
  if (filters.model_api_id) query.model_api_id = filters.model_api_id;
  if (filters.status) query.status = filters.status;
  return { path: "/usage/events", query };
});

onMounted(() => void loadOverview());
</script>
