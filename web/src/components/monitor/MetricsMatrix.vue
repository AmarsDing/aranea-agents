<!--
  MetricsMatrix：指标 × 时间窗的同屏矩阵（纯展示）。
  单个 stat 卡只能说明「现在多少」，矩阵要说的是「哪个窗口在恶化」：
  列 = 15m/1h/6h/24h，行 = 运行数/错误数/失败率/成功率/P95，
  以最长窗口为基准算差值，最差单元格徽标 + 页面置顶提示，点单元格下钻 Runs。
-->
<template>
  <section
    class="metrics-matrix"
    data-test="metrics-matrix"
    :aria-busy="loadingAll ? 'true' : 'false'"
    role="group"
    :aria-label="t('monitorPage.runner.matrix.title')"
  >
    <header class="metrics-matrix__head">
      <div>
        <h3 class="metrics-matrix__title">{{ t('monitorPage.runner.matrix.title') }}</h3>
        <p class="metrics-matrix__subtitle">{{ t('monitorPage.runner.matrix.subtitle') }}</p>
      </div>
      <p v-if="matrix.worstCell" class="metrics-matrix__alert" role="status" data-test="matrix-worst-hint">
        {{
          t('monitorPage.runner.matrix.worstHint', {
            window: windowLabel(matrix.worstCell.windowMinutes),
            metric: metricLabel(matrix.worstCell.key),
            value: matrix.worstCell.formatted,
          })
        }}
      </p>
    </header>

    <AppStatePanel
      v-if="matrix.hasData === false && loadingAll"
      variant="loading"
      dense
      :rows="4"
    />
    <AppStatePanel
      v-else-if="matrix.hasData === false"
      variant="empty"
      dense
      icon="grid_on"
      :title="t('monitorPage.runner.matrix.emptyTitle')"
      :description="t('monitorPage.runner.matrix.emptyDesc')"
    />

    <div v-else class="metrics-matrix__scroll">
      <table class="metrics-matrix__table" data-test="matrix-table">
        <caption class="metrics-matrix__caption">
          {{ t('monitorPage.runner.matrix.baselineHint', { window: baselineLabel }) }}
        </caption>
        <thead>
          <tr>
            <th scope="col" class="metrics-matrix__corner">{{ t('monitorPage.runner.matrix.metricColumn') }}</th>
            <th v-for="col in matrix.columns" :key="col.windowMinutes" scope="col" class="metrics-matrix__colhead">
              <button
                type="button"
                class="metrics-matrix__window"
                :class="{ 'metrics-matrix__window--active': col.windowMinutes === activeWindow }"
                data-test="matrix-window"
                :data-window="col.windowMinutes"
                :aria-pressed="col.windowMinutes === activeWindow ? 'true' : 'false'"
                @click="emit('select-window', col.windowMinutes)"
              >
                {{ windowLabel(col.windowMinutes) }}
              </button>
              <span
                v-if="col.error"
                class="metrics-matrix__col-error"
                data-test="matrix-column-error"
                :data-window="col.windowMinutes"
                role="alert"
              >
                <q-icon name="error_outline" size="14px" aria-hidden="true" />
                {{ t('monitorPage.runner.matrix.columnError', { window: windowLabel(col.windowMinutes) }) }}
                <q-btn
                  flat
                  dense
                  no-caps
                  size="sm"
                  data-test="matrix-retry"
                  :data-window="col.windowMinutes"
                  :label="t('monitorPage.runner.matrix.reloadColumn')"
                  @click="emit('retry', col.windowMinutes)"
                />
              </span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in matrix.rows" :key="row.key" data-test="matrix-row" :data-metric="row.key">
            <th scope="row" class="metrics-matrix__metric">{{ metricLabel(row.key) }}</th>
            <td v-for="cell in row.cells" :key="`${cell.key}-${cell.windowMinutes}`" class="metrics-matrix__cellwrap">
              <button
                type="button"
                class="metrics-matrix__cell"
                :class="[
                  cell.tone ? `metrics-matrix__cell--${cell.tone}` : '',
                  { 'metrics-matrix__cell--worst': cell.worst },
                ]"
                data-test="matrix-cell"
                :data-window="cell.windowMinutes"
                :data-metric="cell.key"
                :data-tone="cell.tone || 'none'"
                :data-worst="cell.worst ? 'true' : 'false'"
                :aria-label="cellLabel(cell)"
                @click="emit('drill', { windowMinutes: cell.windowMinutes, metric: cell.key })"
              >
                <span class="metrics-matrix__value">{{ cell.formatted }}</span>
                <span v-if="deltaText(cell)" class="metrics-matrix__delta">{{ deltaText(cell) }}</span>
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppStatePanel from '../common/AppStatePanel.vue';
import {
  buildMetricsMatrix,
  formatDelta,
  type MatrixCell,
  type MatrixMetricKey,
  type MatrixWindowSlice,
} from '../../features/monitor/metricsMatrix';

const props = withDefaults(
  defineProps<{
    slices: MatrixWindowSlice[];
    /** 当前驱动下方明细卡片的窗口（与 Runner 指标面板的窗口选择同源） */
    activeWindow?: number;
  }>(),
  { activeWindow: 60 },
);

const emit = defineEmits<{
  'select-window': [windowMinutes: number];
  drill: [payload: { windowMinutes: number; metric: MatrixMetricKey }];
  retry: [windowMinutes: number];
}>();

const { t } = useI18n();

const matrix = computed(() => buildMetricsMatrix(props.slices));
const loadingAll = computed(
  () => props.slices.length > 0 && props.slices.some((s) => s.loading) && props.slices.every((s) => !s.metrics),
);
const baselineLabel = computed(() => {
  const withData = matrix.value.columns.filter((c) => !c.error);
  const longest = withData[withData.length - 1]?.windowMinutes ?? props.activeWindow;
  return windowLabel(longest);
});

function windowLabel(minutes: number): string {
  const known: Record<number, string> = {
    15: 'monitorPage.runner.windowOption.m15',
    60: 'monitorPage.runner.windowOption.h1',
    360: 'monitorPage.runner.windowOption.h6',
    1440: 'monitorPage.runner.windowOption.h24',
  };
  const key = known[minutes];
  return key ? t(key) : `${minutes}m`;
}

function metricLabel(key: MatrixMetricKey): string {
  const map: Record<MatrixMetricKey, string> = {
    total_runs: 'monitorPage.runner.totalRuns',
    error_runs: 'monitorPage.runner.errorRuns',
    error_rate: 'monitorPage.runner.errorRate',
    success_rate: 'monitorPage.runner.successRate',
    p95_duration_ms: 'monitorPage.runner.p95Latency',
  };
  return t(map[key]);
}

function deltaText(cell: MatrixCell): string {
  return formatDelta(cell.key, cell.delta);
}

function cellLabel(cell: MatrixCell): string {
  const parts = [
    windowLabel(cell.windowMinutes),
    metricLabel(cell.key),
    cell.formatted,
    t('monitorPage.runner.matrix.drillHint'),
  ];
  const delta = formatDelta(cell.key, cell.delta);
  if (delta) parts.push(delta);
  if (cell.worst) parts.push(t('monitorPage.runner.matrix.worst'));
  return parts.join(' · ');
}
</script>

<style scoped lang="sass">
.metrics-matrix
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
  padding: var(--nt-space-4)
  margin-bottom: var(--nt-space-4)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-l)
  background: var(--nt-data-bg)

  &__head
    display: flex
    align-items: flex-start
    justify-content: space-between
    gap: var(--nt-space-3)
    flex-wrap: wrap

  &__title
    margin: 0
    font-size: var(--nt-fs-title)
    font-weight: 650
    color: var(--nt-text-0)

  &__subtitle
    margin: var(--nt-space-1) 0 0
    font-size: var(--nt-fs-caption)
    color: var(--nt-text-2)

  &__alert
    margin: 0
    padding: var(--nt-space-1) var(--nt-space-3)
    border: 1px solid var(--nt-danger)
    border-radius: var(--nt-radius-pill)
    color: var(--nt-danger)
    font-size: var(--nt-fs-caption)
    background: var(--nt-data-inset)

  &__scroll
    overflow-x: auto

  &__table
    width: 100%
    border-collapse: collapse
    font-variant-numeric: tabular-nums

  &__caption
    caption-side: bottom
    padding-top: var(--nt-space-2)
    text-align: left
    color: var(--nt-text-3)
    font-size: var(--nt-fs-caption)

  &__corner,
  &__metric
    text-align: left
    font-weight: 600
    color: var(--nt-text-2)
    font-size: var(--nt-fs-caption)
    padding: var(--nt-space-1) var(--nt-space-2)
    white-space: nowrap

  &__colhead
    padding: var(--nt-space-1) var(--nt-space-2)
    text-align: center
    vertical-align: top

  &__window
    border: 1px solid var(--nt-border-1)
    border-radius: var(--nt-radius-pill)
    background: transparent
    color: var(--nt-text-2)
    font-size: var(--nt-fs-caption)
    padding: var(--nt-space-1) var(--nt-space-3)
    cursor: pointer

    &:hover
      border-color: var(--nt-border-2)

    &:focus-visible
      outline: 2px solid var(--nt-accent)
      outline-offset: 2px

    &--active
      border-color: var(--nt-accent)
      color: var(--nt-accent)
      background: var(--nt-data-raised)

  &__col-error
    display: flex
    flex-direction: column
    align-items: center
    gap: var(--nt-space-1)
    margin-top: var(--nt-space-1)
    color: var(--nt-danger)
    font-size: var(--nt-fs-micro)

  &__cellwrap
    padding: 1px

  &__cell
    width: 100%
    display: flex
    flex-direction: column
    align-items: center
    gap: 1px
    padding: var(--nt-space-1) var(--nt-space-2)
    border: 1px solid transparent
    border-radius: var(--nt-radius-m)
    background: var(--nt-data-raised)
    color: var(--nt-text-1)
    cursor: pointer

    &:hover
      border-color: var(--nt-border-2)

    &:focus-visible
      outline: 2px solid var(--nt-accent)
      outline-offset: 2px

    &--warn
      border-color: var(--nt-warn)
      color: var(--nt-warn)

    &--danger
      border-color: var(--nt-danger)
      color: var(--nt-danger)
      box-shadow: var(--nt-glow-danger)

    &--worst
      background: var(--nt-data-inset)

  &__value
    font-family: var(--nt-font-mono)
    font-size: var(--nt-fs-panel)
    font-weight: 650

  &__delta
    font-size: var(--nt-fs-micro)
    color: var(--nt-text-3)
</style>
