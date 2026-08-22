<!--
  Pure presentation: Runs 列表（APM 风）。
  筛选/分页状态由父级 useMonitorTraces 持有（v-model 回写），变更后由 composable watcher 统一发起服务端查询。
  详情弹窗见 TraceDetailDialog.vue（由 MonitorPage 直接接线）。
-->
<template>
  <q-card flat bordered class="monitor-card trace-list">
    <q-card-section class="trace-list__head row items-center justify-between no-wrap">
      <div>
        <div class="text-h6 text-weight-bold">{{ t('monitorPage.traces.title') }}</div>
        <div class="text-caption trace-list__subtitle">{{ t('monitorPage.traces.subtitle') }}</div>
      </div>
      <div class="row items-center q-gutter-sm no-wrap">
        <q-input
          :model-value="keyword"
          class="trace-list__search"
          dense
          outlined
          clearable
          debounce="300"
          :placeholder="t('monitorPage.traces.searchPlaceholder')"
          @update:model-value="emit('update:keyword', String($event ?? ''))"
        >
          <template #prepend><q-icon name="search" size="16px" /></template>
        </q-input>
        <span class="trace-live-pill" :class="`trace-live-pill--${liveTone}`">
          <span class="trace-live-pill__dot" />
          {{ livePill.label }}
        </span>
        <q-btn flat round dense icon="restart_alt" @click="emit('reset')">
          <q-tooltip>{{ t('monitorPage.traces.reset') }}</q-tooltip>
        </q-btn>
        <q-btn flat round dense icon="refresh" :loading="loading" @click="emit('refresh')">
          <q-tooltip>{{ t('monitorPage.traces.refresh') }}</q-tooltip>
        </q-btn>
      </div>
    </q-card-section>

    <!-- 筛选胶囊组：类型（默认排除内部域）+ 状态；计数来自服务端聚合（忽略自身维度） -->
    <div class="trace-filters">
      <div class="trace-filters__row row items-center q-gutter-sm">
        <span class="trace-filters__label">{{ t('monitorPage.traces.filterDomain') }}</span>
        <div class="trace-filters__group">
          <button
            v-for="opt in domainOptions"
            :key="opt.value || 'all'"
            class="trace-filter-pill"
            :class="{ 'trace-filter-pill--active': domain === opt.value }"
            type="button"
            @click="emit('update:domain', opt.value)"
          >
            {{ opt.label }}
            <span class="trace-filter-pill__count">{{ formatCompactInt(opt.count) }}</span>
          </button>
        </div>
      </div>
      <div class="trace-filters__row row items-center q-gutter-sm">
        <span class="trace-filters__label">{{ t('monitorPage.traces.filterStatus') }}</span>
        <div class="trace-filters__group">
          <button
            v-for="opt in statusOptions"
            :key="opt.value || 'all'"
            class="trace-filter-pill"
            :class="{ 'trace-filter-pill--active': status === opt.value }"
            type="button"
            @click="emit('update:status', opt.value)"
          >
            {{ opt.label }}
            <span class="trace-filter-pill__count">{{ formatCompactInt(opt.count) }}</span>
          </button>
        </div>
      </div>
    </div>

    <div class="app-registry-table-shell">
      <AppRegistryTable
        :shell="false"
        table-class="monitor-traces-table"
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
        @row-click="onRowClick"
      >
        <template #body-cell-name="slotProps">
          <q-td :props="slotProps">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis trace-name-cell">
                {{ slotProps.row.name || slotProps.row.key || 'unknown' }}
                <q-tooltip max-width="400px" class="text-body2">{{
                  slotProps.row.name || slotProps.row.key || 'unknown'
                }}</q-tooltip>
              </div>
              <div class="app-registry-cell-sub ellipsis row items-center q-gutter-xs">
                <q-badge v-if="domainLabel(slotProps.row)" dense outline :color="domainColor(slotProps.row)">
                  {{ domainLabel(slotProps.row) }}
                </q-badge>
                <span class="text-mono">{{ shortId(slotProps.row.key || slotProps.row.id) }}</span>
              </div>
            </div>
          </q-td>
        </template>
        <template #body-cell-agent="slotProps">
          <q-td :props="slotProps">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">
                {{ slotProps.row.agent_name || '-' }}
                <q-tooltip v-if="slotProps.row.agent_name" max-width="300px" class="text-body2">{{
                  slotProps.row.agent_name
                }}</q-tooltip>
              </div>
              <div
                v-if="slotProps.row.agent_name && slotProps.row.agent_id"
                class="app-registry-cell-sub ellipsis text-mono"
              >
                {{ slotProps.row.agent_id }}
              </div>
            </div>
          </q-td>
        </template>
        <template #body-cell-model="slotProps">
          <q-td :props="slotProps">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">{{ slotProps.row.model || '-' }}</div>
              <div class="app-registry-cell-sub ellipsis">{{ slotProps.row.provider || '-' }}</div>
            </div>
          </q-td>
        </template>
        <template #body-cell-tokens="slotProps">
          <q-td :props="slotProps">
            <span class="trace-num">{{ formatTokens(slotProps.row) }}</span>
          </q-td>
        </template>
        <template #body-cell-latency="slotProps">
          <q-td :props="slotProps">
            <span class="trace-num">{{ formatLatencyRow(slotProps.row) }}</span>
          </q-td>
        </template>
        <template #body-cell-cost="slotProps">
          <q-td :props="slotProps">
            <span class="trace-num">{{ formatCost(slotProps.row) }}</span>
          </q-td>
        </template>
        <template #body-cell-status="slotProps">
          <q-td :props="slotProps">
            <span class="trace-status" :class="`trace-status--${statusTone(slotProps.row.status)}`">
              <span class="trace-status__dot" />
              {{ statusLabel(slotProps.row.status) }}
            </span>
          </q-td>
        </template>
        <template #body-cell-time="slotProps">
          <q-td :props="slotProps">
            <span class="trace-num trace-time">{{ formatDate(slotProps.row.created_at) }}</span>
          </q-td>
        </template>
        <template #no-data>
          <div class="trace-list__empty column items-center q-pa-xl">
            <q-icon name="manage_search" size="40px" class="q-mb-sm" />
            <div class="text-subtitle2">{{ t('monitorPage.traces.emptyTitle') }}</div>
            <div class="text-caption q-mb-md">{{ t('monitorPage.traces.emptyHint') }}</div>
            <q-btn
              v-if="filtersActive"
              flat
              no-caps
              color="accent"
              icon="restart_alt"
              :label="t('monitorPage.traces.reset')"
              @click="emit('reset')"
            />
          </div>
        </template>
      </AppRegistryTable>

      <AppRegistryPagination
        :page="page"
        :page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="t('monitorPage.traces.paginationLabel')"
        :page-size-options="[12, 24, 48]"
        @update:page="emit('update:page', $event)"
        @update:page-size="emit('update:pageSize', $event)"
      />
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MonitorTrace, StreamState } from '../../features/monitor/types';
import { formatDate } from '../../features/monitor/utils';
import { formatCompactInt, formatCostUsd } from '../../features/monitor/runFormat';
import { TRACE_DOMAIN_FILTERS, TRACE_STATUS_FILTERS } from '../../features/monitor/tracesQuery';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import {
  createMonitorTraceColumns,
  traceDomainColor,
  traceDomainLabel,
  traceRunMetrics,
  traceStatusLabel,
} from './monitorTableUi';

const { t } = useI18n();

const columns = computed(() => createMonitorTraceColumns(t));

function statusLabel(status?: string): string {
  return traceStatusLabel(t, status);
}

/** 状态 → 展示色调（状态点+文字用，与 Span 树/瀑布一致语义） */
function statusTone(status?: string): string {
  const s = String(status ?? '');
  if (s === 'running') return 'running';
  if (s === 'ok' || s === 'success') return 'ok';
  if (s === 'error') return 'error';
  if (s === 'timeout' || s === 'interrupted') return 'warn';
  return 'idle';
}

function formatTokens(row: MonitorTrace): string {
  const n = traceRunMetrics(row).total_tokens;
  return n > 0 ? formatCompactInt(n) : '-';
}

function formatLatency(ms: number): string {
  if (ms <= 0) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function formatLatencyRow(row: MonitorTrace): string {
  return formatLatency(traceRunMetrics(row).duration_ms);
}

function formatCost(row: MonitorTrace): string {
  const usd = traceRunMetrics(row).total_cost_usd;
  return usd > 0 ? formatCostUsd(usd) : '-';
}

function shortId(id: string): string {
  const s = String(id || '');
  return s.length > 13 ? `${s.slice(0, 8)}…${s.slice(-4)}` : s;
}

function domainLabel(row: MonitorTrace): string {
  return traceDomainLabel(t, row.domain);
}

function domainColor(row: MonitorTrace): string {
  return traceDomainColor(row.domain);
}

const props = defineProps<{
  rows: MonitorTrace[];
  /** 服务端总条数（分页用，与 AuditTable 同模式） */
  total: number;
  loading: boolean;
  /** 筛选/分页状态（父级 useMonitorTraces 持有，v-model 回写） */
  keyword: string;
  status: string;
  domain: string;
  page: number;
  pageSize: number;
  /** 筛选 chips 计数（服务端聚合，各自忽略自身维度） */
  statusCounts: Record<string, number>;
  domainCounts: Record<string, number>;
  /** WS 实时订阅状态（pill 展示） */
  liveState: StreamState;
  highlightUsageEventId?: string;
}>();

const emit = defineEmits<{
  'update:keyword': [value: string];
  'update:status': [value: string];
  'update:domain': [value: string];
  'update:page': [value: number];
  'update:pageSize': [value: number];
  refresh: [];
  reset: [];
  openTrace: [row: MonitorTrace];
}>();

const pageMax = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)));

/** 有任一筛选条件激活（空态展示「重置」引导用） */
const filtersActive = computed(() => props.keyword.trim() !== '' || props.status !== '' || props.domain !== '');

/** 类型 pills：'' = 默认视图（排除内部域）；计数 = 非内部域合计 */
const domainOptions = computed(() => {
  const counts = props.domainCounts;
  const allCount = Object.entries(counts).reduce(
    (sum, [key, value]) => (key === 'system' || key === 'skill' ? sum : sum + value),
    0,
  );
  return TRACE_DOMAIN_FILTERS.map((value) => ({
    value,
    label: value === '' ? t('monitorPage.traces.filterAll') : traceDomainLabel(t, value) || value,
    count: value === '' ? allCount : (counts[value] ?? 0),
  }));
});

/** 状态 pills：'' = 全部；计数 = 各状态合计 */
const statusOptions = computed(() => {
  const counts = props.statusCounts;
  const allCount = Object.values(counts).reduce((sum, value) => sum + value, 0);
  return TRACE_STATUS_FILTERS.map((value) => ({
    value,
    label: value === '' ? t('monitorPage.traces.filterAll') : traceStatusLabel(t, value),
    count: value === '' ? allCount : (counts[value] ?? 0),
  }));
});

/** 实时状态 pill：connected（已连接监听中）与 live（事件流动中）都视为实时可用 */
const livePill = computed(() => {
  switch (props.liveState) {
    case 'live':
    case 'connected':
      return { label: t('monitorPage.traces.live.live') };
    case 'error':
      return { label: t('monitorPage.traces.live.error') };
    default:
      return { label: t('monitorPage.traces.live.connecting') };
  }
});

const liveTone = computed(() => {
  switch (props.liveState) {
    case 'live':
    case 'connected':
      return 'live';
    case 'error':
      return 'error';
    default:
      return 'idle';
  }
});

function onRowClick(_evt: Event, row: MonitorTrace) {
  emit('openTrace', row);
}

function tryOpenHighlightedRun() {
  const hit = (props.highlightUsageEventId || '').trim();
  if (!hit || props.rows.length === 0) return;
  const row = props.rows.find((r) => String(r.id || '').trim() === hit);
  if (row) emit('openTrace', row);
}

watch(() => props.highlightUsageEventId, tryOpenHighlightedRun);
watch(() => props.rows.length, tryOpenHighlightedRun);
</script>

<style scoped>
.trace-list__head {
  padding-bottom: 12px;
}

.trace-list__subtitle {
  color: var(--color-text-secondary);
}

.trace-list__search {
  width: 300px;
}

/* ── 实时状态 pill ── */
.trace-live-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 11px;
  border: 1px solid var(--glass-border);
  color: var(--color-text-secondary);
  background: color-mix(in srgb, var(--glass-surface) 60%, transparent);
}

.trace-live-pill__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-text-icon-muted);
}

.trace-live-pill--live {
  color: var(--color-success);
  border-color: color-mix(in srgb, var(--color-success) 40%, transparent);
}

.trace-live-pill--live .trace-live-pill__dot {
  background: var(--color-success);
  animation: trace-live-pulse 1.6s ease-in-out infinite;
}

.trace-live-pill--error {
  color: var(--color-danger);
  border-color: color-mix(in srgb, var(--color-danger) 40%, transparent);
}

.trace-live-pill--error .trace-live-pill__dot {
  background: var(--color-danger);
}

@keyframes trace-live-pulse {
  0%,
  100% {
    opacity: 100%;
  }

  50% {
    opacity: 30%;
  }
}

/* ── 筛选胶囊组 ── */
.trace-filters {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 0 16px 12px;
}

.trace-filters__label {
  min-width: 44px;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.trace-filters__group {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 3px;
  border-radius: 12px;
  border: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent);
}

.trace-filter-pill {
  appearance: none;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 12px;
  line-height: 1;
  padding: 6px 10px;
  border-radius: 9px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition:
    background 0.15s ease,
    color 0.15s ease;
}

.trace-filter-pill:hover {
  color: var(--color-text-primary);
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
}

.trace-filter-pill--active,
.trace-filter-pill--active:hover {
  background: var(--color-accent);
  color: var(--color-on-accent);
}

.trace-filter-pill__count {
  font-size: 10px;
  font-variant-numeric: tabular-nums;
  opacity: 65%;
}

/* ── 表格单元格 ── */
.trace-num {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-secondary);
}

.trace-time {
  white-space: nowrap;
}

.trace-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  white-space: nowrap;
}

.trace-status__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-text-icon-muted);
}

.trace-status--ok {
  color: var(--color-success);
}

.trace-status--ok .trace-status__dot {
  background: var(--color-success);
}

.trace-status--error {
  color: var(--color-danger);
}

.trace-status--error .trace-status__dot {
  background: var(--color-danger);
}

.trace-status--warn {
  color: var(--color-warning);
}

.trace-status--warn .trace-status__dot {
  background: var(--color-warning);
}

.trace-status--running {
  color: var(--color-accent);
}

.trace-status--running .trace-status__dot {
  background: var(--color-accent);
  animation: trace-live-pulse 1.4s ease-in-out infinite;
}

.monitor-traces-table :deep(tbody tr) {
  cursor: pointer;
  transition: background 0.12s ease;
}

.monitor-traces-table :deep(tbody tr:hover td:first-child) {
  box-shadow: inset 2px 0 0 var(--color-accent);
}

.trace-name-cell {
  max-width: 24ch;
}

.text-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
}

.trace-list__empty {
  color: var(--color-text-secondary);
}
</style>
