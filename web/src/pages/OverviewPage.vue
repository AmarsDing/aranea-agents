<template>
  <q-page class="app-standard-page ov-page">
    <div class="ov-page__shell">
      <q-banner v-if="error" class="ov-error" role="alert">
        <template #avatar>
          <q-icon name="warning" size="18px" />
        </template>
        {{ t('overviewPage.v2.loadFailed') }}: {{ error }}
        <template #action>
          <q-btn flat dense :label="t('overviewPage.v2.refresh')" @click="onRefresh" />
        </template>
      </q-banner>

      <OverviewVerdictBanner
        class="ov-page__sticky"
        :verdict="verdict"
        :headline="headline"
        :subline="subline"
        :chips="chips"
        :updated-at="updatedAtClock"
        :stale="stale"
        :loading="loading"
        @refresh="onRefresh"
      >
        <template #filters>
          <q-select
            :model-value="filters.range"
            dense
            outlined
            emit-value
            map-options
            :label="t('overviewPage.filterRange')"
            :options="rangeOptions"
            class="ov-filter"
            @update:model-value="(v) => setRange(String(v))"
          />
          <q-select
            :model-value="filters.provider_code || undefined"
            dense
            outlined
            emit-value
            map-options
            clearable
            :label="t('overviewPage.filterProvider')"
            :options="providerOptions"
            class="ov-filter ov-filter--wide"
            @update:model-value="(v) => setProvider(v ? String(v) : '')"
          />
        </template>
      </OverviewVerdictBanner>

      <OverviewAttentionQueue :lanes="board.lanes" @drill="onDrillItem" />

      <OverviewRedStrip
        :model-tiles="modelTiles"
        :run-tiles="runTiles"
        :run-window="runnerWindowMinutes"
        :run-window-options="runnerWindowOptions"
        @update:run-window="onRunWindow"
        @drill="onTileDrill"
      />

      <OverviewCostBoard
        :cost-value-usd="costValueUsd"
        :cost-delta="costDelta"
        :budget="budget"
        :unpriced-count="unpricedRows.length"
        :top-bars="topModelBars"
        :cache="cacheView"
        @drill="onTileDrill"
      />

      <OverviewTrendBoard
        :labels="trendLabels"
        :calls="trendCalls"
        :failures="trendFailures"
        :costs-usd="trendCosts"
        :granularity="trendGranularity"
      />

      <OverviewOrgBoard
        :orgs="inventory.orgs"
        :teams="inventory.teams"
        :agents-total="inventory.agentsTotal"
        :agents-enabled="inventory.agentsEnabled"
        :worst-agents="worstAgentRows"
        @drill="router.push('/agents')"
      />

      <OverviewFreshnessFooter :sources="footSources" :notes="footNotes" />
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { useOverviewPage } from '../features/usage/useOverviewPage';
import { useOverviewView } from '../features/usage/useOverviewView';
import { useAttentionBoard } from '../features/usage/useAttentionBoard';
import type { AttentionItem } from '../features/usage/types';
import OverviewVerdictBanner from '../components/overview/OverviewVerdictBanner.vue';
import OverviewAttentionQueue from '../components/overview/OverviewAttentionQueue.vue';
import OverviewRedStrip from '../components/overview/OverviewRedStrip.vue';
import OverviewCostBoard from '../components/overview/OverviewCostBoard.vue';
import OverviewTrendBoard from '../components/overview/OverviewTrendBoard.vue';
import OverviewOrgBoard from '../components/overview/OverviewOrgBoard.vue';
import OverviewFreshnessFooter from '../components/overview/OverviewFreshnessFooter.vue';

const router = useRouter();
const page = useOverviewPage();
const {
  t,
  error,
  loading,
  filters,
  rangeOptions,
  providerOptions,
  setRange,
  setProvider,
  trendGranularity,
  costDelta,
  unpricedRows,
  topModelBars,
  runnerWindowMinutes,
  runnerWindowOptions,
  loadRunnerMetrics,
  inventory,
  refreshAll,
} = page;

const anomalies = computed(() => page.overview.value?.anomalies ?? []);
const { board } = useAttentionBoard(anomalies);
const {
  verdict,
  headline,
  subline,
  chips,
  updatedAtClock,
  stale,
  modelTiles,
  runTiles,
  costValueUsd,
  budget,
  cacheView,
  trendLabels,
  trendCalls,
  trendFailures,
  trendCosts,
  worstAgentRows,
  footSources,
  footNotes,
} = useOverviewView(page, board);

// —— 交互（路由下钻全部收在 Page 层） ——
function onRefresh() {
  void refreshAll();
}
function onRunWindow(v: number) {
  runnerWindowMinutes.value = v;
  void loadRunnerMetrics();
}
function onDrillItem(item: AttentionItem) {
  if (item.to) void router.push(item.to);
}
function onTileDrill(key: string) {
  if (key === 'unpriced') {
    void router.push('/models');
    return;
  }
  if (key === 'calls' || key === 'callSuccessRate' || key === 'burnRate' || key === 'detail') {
    void router.push('/usage/events');
  }
}
</script>
