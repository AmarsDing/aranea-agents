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
        :loading="loading || attentionLoading"
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

      <OverviewDomainCards :cards="domainCards" />

      <!-- 设计稿：费用消耗是主体，需要处理的事在右边 -->
      <div class="ov-mainrow">
        <OverviewCostBoard
          :available="!!page.overview.value"
          :cost-value-usd="costValueUsd"
          :cost-delta="costDelta"
          :budget="budget"
          :unpriced-count="unpricedRows.length"
          :top-bars="topModelBars"
          :agent-bars="topAgentBars"
          :cache="cacheView"
          :terminals="terminals"
          @drill="onTileDrill"
        />
        <OverviewAttentionQueue
          :lanes="board.lanes"
          :loading="loading || attentionLoading"
          @drill="onDrillItem"
          @refresh="onRefresh"
        />
      </div>

      <div class="ov-workrow">
        <OverviewTaskBoard
          :items="taskSituations.items.value"
          :total="taskSituations.total.value"
          :limit="taskSituations.limit.value"
          :offset="taskSituations.offset.value"
          :loading="taskSituations.loading.value"
          :error="taskSituations.error.value"
          :query="taskSituations.query.value"
          @refresh="taskSituations.refresh"
          @filter="taskSituations.setQuery"
          @page="(offset) => taskSituations.setQuery({ offset })"
          @page-size="(limit) => taskSituations.setQuery({ limit, offset: 0 })"
          @drill="onTaskDrill"
          @create="onTaskCreate"
        />
        <div class="ov-workrow__rail">
          <AppWorkspaceLaunchpad :label="t('uiSystem.launchpad.label')" :items="launchpadItems" />
        </div>
      </div>

      <OverviewRedStrip
        :model-tiles="modelTiles"
        :run-tiles="runTiles"
        :run-window="runnerWindowMinutes"
        :run-window-options="runnerWindowOptions"
        @update:run-window="onRunWindow"
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
        :inventory-available="inventoryAvailable"
        :usage-available="!!page.overview.value"
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
import { useTerminalUsage } from '../features/usage/useTerminalUsage';
import { useHarnessCounts } from '../features/usage/useHarnessCounts';
import { useInboxBadge } from '../features/today/useInboxBadge';
import { buildNavDomains } from '../config/navDomains';
import { useNavStore } from '../stores/nav';
import { useTaskSituations } from '../features/session/useTaskSituations';
import { workspaceLocationQuery } from '../features/chat/taskWorkspaceNavigation';
import type { TaskSituationItem } from '../features/session/types';
import type { AttentionItem } from '../features/usage/types';
import OverviewVerdictBanner from '../components/overview/OverviewVerdictBanner.vue';
import OverviewTaskBoard from '../components/overview/OverviewTaskBoard.vue';
import OverviewAttentionQueue from '../components/overview/OverviewAttentionQueue.vue';
import OverviewRedStrip from '../components/overview/OverviewRedStrip.vue';
import OverviewCostBoard from '../components/overview/OverviewCostBoard.vue';
import OverviewDomainCards from '../components/overview/OverviewDomainCards.vue';
import type { OvDomainCard } from '../components/overview/ovTypes';
import OverviewTrendBoard from '../components/overview/OverviewTrendBoard.vue';
import OverviewOrgBoard from '../components/overview/OverviewOrgBoard.vue';
import OverviewFreshnessFooter from '../components/overview/OverviewFreshnessFooter.vue';
import AppWorkspaceLaunchpad from '../components/layout/AppWorkspaceLaunchpad.vue';
import { workspaceShortcuts } from '../config/workspaceShortcuts';

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
  topAgentBars,
  runnerWindowMinutes,
  runnerWindowOptions,
  loadRunnerMetrics,
  inventory,
  inventoryAvailable,
  refreshAll,
} = page;

const launchpadItems = computed(() =>
  workspaceShortcuts.map((item) => ({
    ...item,
    title: t(`uiSystem.launchpad.${item.key}.title`),
    description: t(`uiSystem.launchpad.${item.key}.description`),
    action: t('uiSystem.launchpad.start'),
  })),
);

const { board, loading: attentionLoading, refresh: refreshAttention } = useAttentionBoard();
// 没有运行台权限就不发请求，免得弹一条 403 提示
const nav = useNavStore();
const { terminals, refresh: refreshTerminals } = useTerminalUsage(() => nav.isPathVisible('/fleet'));
const { counts, refresh: refreshCounts } = useHarnessCounts(nav.isPathVisible);
const { count: inboxCount } = useInboxBadge();

// 五张领域卡：导航里 Harness 模块的五个领域（看不到的领域不出卡），数字来自各自的接口
const domainCards = computed<OvDomainCard[]>(() => {
  const c = counts.value;
  const inv = inventoryAvailable.value ? inventory.value : null;
  const summary = page.rangeSummary.value;
  const facts: Record<string, OvDomainCard['facts']> = {
    agents: [
      { label: 'Agent', value: inv ? `${inv.agentsEnabled} / ${inv.agentsTotal}` : undefined },
      { label: t('overviewPage.v2.fact.teams'), value: inv?.teams },
      { label: t('overviewPage.v2.fact.orgs'), value: inv?.orgs },
    ],
    orchestrate: [
      { label: t('overviewPage.v2.fact.graphs'), value: c.graphs },
      { label: t('overviewPage.v2.fact.cron'), value: c.cron },
    ],
    capability: [
      { label: t('overviewPage.v2.fact.tools'), value: c.tools },
      { label: t('overviewPage.v2.fact.skills'), value: c.skills },
      { label: 'MCP', value: c.mcp },
    ],
    connect: [
      { label: t('overviewPage.v2.fact.channels'), value: c.channels },
      { label: t('overviewPage.v2.fact.credentials'), value: c.credentials },
    ],
    ops: [
      { label: t('overviewPage.v2.fact.calls'), value: summary?.call_count },
      {
        label: t('overviewPage.v2.fact.successRate'),
        value: summary && summary.call_count > 0 ? `${Math.round(summary.success_rate * 100)}%` : undefined,
      },
      { label: t('overviewPage.v2.fact.waiting'), value: inboxCount.value },
    ],
  };
  return buildNavDomains(nav.isPathVisible)
    .filter((domain) => domain.module === 'harness' && facts[domain.key])
    .map((domain) => ({
      key: domain.key,
      icon: domain.icon,
      title: t(domain.labelKey),
      question: t(`overviewPage.v2.domainQ.${domain.key}`),
      to: domain.primaries[0].to,
      facts: facts[domain.key],
    }));
});
const taskSituations = useTaskSituations();
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
  void refreshAttention();
  void refreshTerminals();
  void refreshCounts();
  void taskSituations.refresh();
}
function onTaskDrill(item: TaskSituationItem) {
  const query = workspaceLocationQuery({
    sessionId: item.session_id,
    taskId: item.task_id,
    stepId: item.current_step_id,
  });
  query.from = router.currentRoute.value.fullPath;
  void router.push({ path: '/task-workspace', query });
}
function onTaskCreate() {
  void router.push('/chat');
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
