<template>
  <!--
    Harness › 运行与治理 › 用量成本（2026-11-19 重排）：回答「钱花在哪了」。
    页头放时间范围与导出；四格汇总 → 按日趋势 → 按模型 / 按 Agent 排行 → 明细（筛选 + 表格）。
    汇总和明细用同一组筛选，口径一致。
  -->
  <q-page class="app-standard-page op-page app-registry-page">
    <AppPageHero :title="t('ops.usage.title')" :subtitle="t('ops.usage.hint')">
      <template #actions>
        <q-btn-toggle
          v-model="filters.range"
          dense
          no-caps
          unelevated
          toggle-color="primary"
          :options="rangeOptions"
          :aria-label="t('usageEventsPage.range')"
          @update:model-value="reloadAll"
        />
        <q-btn
          outline
          no-caps
          icon="download"
          :label="t('usageEventsPage.export')"
          :loading="exporting"
          @click="onExportCsv"
        />
        <q-btn flat round dense icon="more_vert" :aria-label="t('ops.common.more')">
          <q-menu anchor="bottom right" self="top right">
            <q-list dense>
              <q-item v-close-popup clickable :disable="purging" @click="onPurgeConfirm">
                <q-item-section avatar><q-icon name="delete_outline" /></q-item-section>
                <q-item-section>{{ t('ops.usage.purge') }}</q-item-section>
              </q-item>
            </q-list>
          </q-menu>
        </q-btn>
      </template>
    </AppPageHero>

    <p v-if="summaryError" class="op-note op-note--danger" role="alert">
      {{ t('ops.common.loadFailed', { msg: summaryError }) }}
    </p>
    <div class="op-kpis" role="list" :aria-busy="summaryLoading ? 'true' : 'false'">
      <div v-for="k in kpis" :key="k.key" class="op-kpi" :class="`op-kpi--${k.tone}`" role="listitem">
        <span class="op-kpi__label">{{ k.label }}</span>
        <span class="op-kpi__value op-num">{{ k.value }}</span>
        <span class="op-kpi__sub">{{ k.sub }}</span>
      </div>
    </div>

    <OpsFailureReasons
      v-if="failures.total.value || failures.error.value"
      :title="t('ops.failures.title')"
      :hint="t('ops.failures.usageHint')"
      :reasons="failures.reasons.value"
      :total="failures.total.value"
      :loading="failures.loading.value"
      :error="failures.error.value"
    />

    <OverviewTrendBoard
      :labels="trend.labels"
      :calls="trend.calls"
      :failures="trend.failures"
      :costs-usd="trend.costs"
      granularity="day"
    />

    <div class="op-grid">
      <section v-for="board in boards" :key="board.key" class="op-panel" :aria-label="board.title">
        <header class="op-panel__head">
          <h2 class="op-panel__title">{{ board.title }}</h2>
          <span class="op-panel__meta">{{ t('ops.usage.byCost') }}</span>
        </header>
        <p v-if="!board.rows.length" class="op-empty">{{ t('ops.usage.noBreakdown') }}</p>
        <ul v-else class="op-list">
          <li v-for="row in board.rows" :key="row.key" class="op-row op-share">
            <span class="op-row__main">
              <span class="op-row__title" :title="row.name">{{ row.name }}</span>
              <span class="op-row__sub op-num">
                {{ t('ops.usage.rowSub', { calls: formatCount(row.calls), tokens: formatCompactCount(row.tokens) }) }}
              </span>
              <span class="op-share__bar" aria-hidden="true"><i :style="{ width: `${row.share}%` }" /></span>
            </span>
            <span class="op-row__num op-num">{{ formatMoney(row.cost) }}</span>
          </li>
        </ul>
      </section>
    </div>

    <section class="op-panel" :aria-label="t('ops.usage.detail')">
      <header class="op-panel__head">
        <h2 class="op-panel__title">
          {{ t('ops.usage.detail') }}<span class="op-panel__count op-num">{{ eventsTotal }}</span>
        </h2>
      </header>
      <div class="op-filters">
        <q-select
          v-for="f in filterFields"
          :key="f.key"
          v-model="filters[f.key]"
          class="op-filters__field"
          dense
          outlined
          clearable
          emit-value
          map-options
          :label="f.label"
          :options="f.options"
          @update:model-value="f.key === 'provider_code' ? onProviderChange() : reloadAll()"
        />
        <q-btn flat dense no-caps icon="restart_alt" :label="t('usageEventsPage.reset')" @click="resetFilters" />
      </div>

      <!-- 五态：已有数据但刷新失败 → 局部失败横幅，表格照常渲染 -->
      <AppStatePanel
        v-if="(error || forbidden) && events.length > 0"
        variant="partial"
        dense
        class="q-mb-md"
        :description="error"
        :action-label="t('usageEventsPage.retry')"
        action-icon="refresh"
        @action="load"
      />

      <!-- 五态：无权限 → 只渲染 forbidden 面板，不提供重试 -->
      <AppStatePanel v-else-if="forbidden" variant="forbidden" class="q-mb-md" />

      <!-- 五态：加载失败且无数据 → 只渲染可重试错误面板，不与空态并存。 -->
      <AppStatePanel
        v-else-if="error"
        variant="error"
        class="q-mb-md"
        :description="error"
        :action-label="t('usageEventsPage.retry')"
        action-icon="refresh"
        @action="load"
      />

      <!-- 五态：首屏加载无数据 → 骨架面板 -->
      <AppStatePanel v-if="loading && events.length === 0 && !error && !forbidden" variant="loading" :rows="5" />

      <!-- 五态：成功但无数据 → 空态 -->
      <AppStatePanel
        v-else-if="!loading && !error && !forbidden && events.length === 0"
        variant="empty"
        :title="t('usageEventsPage.empty')"
        :description="t('usageEventsPage.emptyHint')"
        icon="insights"
      />

      <template v-else-if="events.length > 0">
        <AppRegistryTable
          :rows="events"
          :columns="columns"
          filter=""
          row-key="id"
          :loading="loading"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
          <template #body-cell-model_api_id="props">
            <q-td :props="props">
              <span class="app-registry-cell-primary ellipsis">{{ props.row.model_api_id }}</span>
            </q-td>
          </template>
          <template #body-cell-agent_id="props">
            <q-td :props="props">
              <div class="app-registry-cell-primary ellipsis">
                {{ props.row.agent_name || props.row.agent_key || props.row.agent_id || '—' }}
              </div>
              <div v-if="props.row.agent_name" class="app-registry-cell-sub ellipsis text-mono">
                {{ props.row.agent_id }}
              </div>
            </q-td>
          </template>
          <template #body-cell-session_id="props">
            <q-td :props="props">
              <AppRegistryHoverTip :text="props.row.session_id" :indicator="false">
                <span class="app-registry-cell-primary ellipsis">
                  {{ props.row.session_title || shortenId(props.row.session_id) }}
                </span>
              </AppRegistryHoverTip>
            </q-td>
          </template>
          <template #body-cell-status="props">
            <q-td :props="props">
              <AppStatusChip :status="props.row.status" :tooltip="props.row.error_message" />
            </q-td>
          </template>
          <template #body-cell-total_cost_micro_usd="props">
            <q-td :props="props">{{ formatMoney(props.row.total_cost_micro_usd) }}</q-td>
          </template>
          <template #body-cell-latency_ms="props">
            <q-td :props="props">
              <span :title="`${props.row.latency_ms}ms`">{{ formatLatency(props.row.latency_ms) }}</span>
            </q-td>
          </template>
        </AppRegistryTable>

        <AppRegistryPagination
          :page="page"
          :page-size="pageSize"
          :page-max="pageMax"
          :total="eventsTotal"
          :loading="loading"
          :label="t('usageEventsPage.paginationLabel')"
          @update:page="onPage"
          @update:page-size="onPageSize"
        />
      </template>
    </section>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../components/layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatusChip from '../components/common/AppStatusChip.vue';
import OverviewTrendBoard from '../components/overview/OverviewTrendBoard.vue';
import OpsFailureReasons from '../components/ops/OpsFailureReasons.vue';
import { useUsageEventsPage } from '../features/usage/useUsageEventsPage';
import { formatCompactCount, formatCount } from '../features/usage/moneyFormat';
import type { ModelTokenUsageEvent, ModelUsageBreakdownRow, ModelUsageQuery } from '../features/usage/types';
import { type RegistryTableColumn } from '../features/ui/registryTableColumns';
import { buildUsageEventTableColumns } from '../features/usage/usageTableUi';

const { t } = useI18n();

const {
  events,
  eventsTotal,
  page,
  pageSize,
  pageMax,
  onPage,
  onPageSize,
  loading,
  error,
  forbidden,
  exporting,
  filters,
  rangeOptions,
  statusOptions,
  usageKindOptions,
  effortOptions,
  providerOptions,
  modelOptions,
  agentOptions,
  teamOptions,
  loadFilterOptions,
  onProviderFilterChange,
  purging,
  load,
  reloadAll,
  summary,
  summaryError,
  summaryLoading,
  failures,
  exportCsv,
  onPurgeConfirm,
  resetFilters,
  formatMoney,
  formatLatency,
} = useUsageEventsPage();

const columns = computed(() => buildUsageEventTableColumns(t) satisfies RegistryTableColumn<ModelTokenUsageEvent>[]);

type FilterKey = keyof Pick<
  ModelUsageQuery,
  'provider_code' | 'model_api_id' | 'agent_id' | 'team_id' | 'usage_kind' | 'effort' | 'status'
>;
const filterFields = computed<
  Array<{ key: FilterKey; label: string; options: Array<{ label: string; value: string }> }>
>(() => [
  { key: 'provider_code', label: t('usageEventsPage.provider'), options: providerOptions.value },
  { key: 'model_api_id', label: t('usageEventsPage.model'), options: modelOptions.value },
  { key: 'agent_id', label: t('usageEventsPage.agent'), options: agentOptions.value },
  { key: 'team_id', label: t('usageEventsPage.team'), options: teamOptions.value },
  { key: 'usage_kind', label: t('usageEventsPage.source'), options: usageKindOptions.value },
  { key: 'effort', label: t('usageEventsPage.effortLabel'), options: effortOptions.value },
  { key: 'status', label: t('usageEventsPage.status'), options: statusOptions.value },
]);

const kpis = computed(() => {
  const r = summary.value?.range;
  const none = t('ops.common.none');
  const failed = r ? r.failed_count : 0;
  return [
    {
      key: 'cost',
      label: t('ops.usage.kpi.cost'),
      value: r ? formatMoney(r.total_cost_micro_usd) : none,
      sub: t('ops.usage.kpi.costSub', { today: formatMoney(summary.value?.today.total_cost_micro_usd ?? 0) }),
      tone: 'ok',
    },
    {
      key: 'calls',
      label: t('ops.usage.kpi.calls'),
      value: r ? formatCount(r.call_count) : none,
      sub: t('ops.usage.kpi.callsSub', { failed: formatCount(failed) }),
      tone: failed > 0 ? 'watch' : 'ok',
    },
    {
      key: 'tokens',
      label: t('ops.usage.kpi.tokens'),
      value: r ? formatCompactCount(r.total_tokens) : none,
      sub: t('ops.usage.kpi.tokensSub', {
        input: formatCompactCount(r?.input_tokens ?? 0),
        output: formatCompactCount(r?.output_tokens ?? 0),
      }),
      tone: 'ok',
    },
    {
      key: 'latency',
      label: t('ops.usage.kpi.latency'),
      value: r ? formatLatency(r.avg_latency_ms) : none,
      sub: t('ops.usage.kpi.latencySub', { tps: Math.round(r?.avg_tokens_per_second ?? 0) }),
      tone: 'ok',
    },
  ];
});

const trend = computed(() => {
  const points = summary.value?.trends ?? [];
  return {
    labels: points.map((p) => p.date_key.slice(5)),
    calls: points.map((p) => p.call_count),
    failures: points.map((p) => p.failed_count),
    costs: points.map((p) => p.total_cost_micro_usd / 1_000_000),
  };
});

const agentName = computed(() => new Map(agentOptions.value.map((o) => [o.value, o.label])));

function breakdown(rows: ModelUsageBreakdownRow[], name: (row: ModelUsageBreakdownRow) => string) {
  const top = rows.slice(0, 8);
  // 按费用占比画条；全是 0 元（本地模型、未定价）时退回按调用次数
  const byCost = top.some((r) => r.total_cost_micro_usd > 0);
  const weight = (r: ModelUsageBreakdownRow) => (byCost ? r.total_cost_micro_usd : r.call_count);
  const max = Math.max(1, ...top.map(weight));
  return top.map((r) => ({
    key: `${r.provider_code}/${r.model_api_id}/${r.agent_id}`,
    name: name(r),
    calls: r.call_count,
    tokens: r.total_tokens,
    cost: r.total_cost_micro_usd,
    share: Math.round((weight(r) / max) * 100),
  }));
}

const boards = computed(() => [
  {
    key: 'models',
    title: t('ops.usage.byModel'),
    rows: breakdown(
      summary.value?.top_models ?? [],
      (r) => r.model_display_name || r.model_api_id || t('ops.common.none'),
    ),
  },
  {
    key: 'agents',
    title: t('ops.usage.byAgent'),
    rows: breakdown(
      summary.value?.top_agents ?? [],
      (r) => agentName.value.get(r.agent_id) || r.agent_key || r.agent_id || t('ops.common.none'),
    ),
  },
]);

async function onExportCsv() {
  try {
    await exportCsv();
  } catch {
    // Domain composable owns export failure feedback.
  }
}

/** Provider 变化：级联清理失效的模型筛选后再查询 */
function onProviderChange() {
  onProviderFilterChange();
  reloadAll();
}

/** Session 无标题时截断显示 ID（完整 ID 见悬停 tooltip） */
function shortenId(id?: string) {
  const s = (id ?? '').trim();
  if (!s) return '—';
  return s.length <= 12 ? s : `${s.slice(0, 8)}…`;
}

onMounted(() => {
  void loadFilterOptions();
  reloadAll();
});
</script>
