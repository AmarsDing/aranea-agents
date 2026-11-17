<template>
  <section class="ov-trend">
    <div class="ov-trend__panel">
      <header class="ov-section-head">
        <q-icon name="stacked_bar_chart" size="18px" class="ov-section-head__icon" />
        <h2 class="ov-section-head__title">{{ t('overviewPage.v2.trendCallsTitle') }}</h2>
        <span class="ov-section-head__cap">{{ caption }}</span>
        <span class="ov-trend__legend">
          <i class="ov-trend__swatch ov-trend__swatch--calls" />{{ t('overviewPage.v2.trendCallsLegend') }}
          <i class="ov-trend__swatch ov-trend__swatch--fails" />{{ t('overviewPage.v2.trendFailuresLegend') }}
        </span>
      </header>
      <div v-if="!labels.length" class="ov-trend__empty">{{ t('overviewPage.v2.trendEmpty') }}</div>
      <template v-else>
        <div class="ov-chart__readout" aria-live="polite">
          <template v-if="activeCalls !== null">
            <span class="ov-chart__readout-key">{{ labels[activeCalls] }}</span>
            <span>{{ t('overviewPage.v2.trendCallsLegend') }} {{ formatCount(calls[activeCalls] ?? 0) }}</span>
            <span class="ov-chart__readout-fail"
              >{{ t('overviewPage.v2.trendFailuresLegend') }} {{ formatCount(failures[activeCalls] ?? 0) }}</span
            >
          </template>
          <span v-else class="ov-chart__readout-hint">{{ t('overviewPage.v2.trendReadoutHint') }}</span>
        </div>
        <div
          class="ov-chart"
          tabindex="0"
          role="img"
          :aria-label="t('overviewPage.v2.trendCallsTitle')"
          @pointermove="activeCalls = indexFromPointer($event)"
          @pointerleave="activeCalls = null"
          @focus="activeCalls = labels.length - 1"
          @blur="activeCalls = null"
          @keydown="onKey($event, 'calls')"
        >
          <span class="ov-chart__max">{{ formatCount(maxCalls) }}</span>
          <svg
            class="ov-chart__svg"
            viewBox="0 0 600 150"
            preserveAspectRatio="none"
            aria-hidden="true"
            focusable="false"
          >
            <line v-for="y in GRID" :key="y" class="ov-chart__grid" x1="0" x2="600" :y1="y" :y2="y" />
            <rect
              v-if="activeCalls !== null"
              class="ov-chart__band"
              :x="activeCalls * slot"
              y="4"
              :width="slot"
              height="142"
            />
            <g v-for="(v, i) in calls" :key="i">
              <rect
                class="ov-chart__bar--calls"
                :class="{ 'ov-chart__bar--active': activeCalls === i }"
                :x="barX(i)"
                :y="146 - barH(v, maxCalls)"
                :width="barW"
                :height="barH(v, maxCalls)"
                rx="1.5"
              />
              <rect
                v-if="(failures[i] ?? 0) > 0"
                class="ov-chart__bar--fails"
                :x="barX(i)"
                :y="146 - barH(failures[i] ?? 0, maxCalls)"
                :width="barW"
                :height="barH(failures[i] ?? 0, maxCalls)"
                rx="1.5"
              />
            </g>
          </svg>
        </div>
        <div class="ov-trend__axis">
          <span>{{ labels[0] }}</span>
          <span v-if="labels.length > 2">{{ labels[Math.floor(labels.length / 2)] }}</span>
          <span>{{ labels[labels.length - 1] }}</span>
        </div>
      </template>
    </div>

    <div class="ov-trend__panel">
      <header class="ov-section-head">
        <q-icon name="show_chart" size="18px" class="ov-section-head__icon" />
        <h2 class="ov-section-head__title">{{ t('overviewPage.v2.trendCostTitle') }}</h2>
        <span class="ov-section-head__cap">{{ caption }}</span>
        <span class="ov-trend__legend">
          <i class="ov-trend__swatch ov-trend__swatch--cost" />{{ t('overviewPage.v2.costRange') }}
        </span>
      </header>
      <div v-if="!labels.length" class="ov-trend__empty">{{ t('overviewPage.v2.trendEmpty') }}</div>
      <template v-else>
        <div class="ov-chart__readout" aria-live="polite">
          <template v-if="activeCost !== null">
            <span class="ov-chart__readout-key">{{ labels[activeCost] }}</span>
            <span class="ov-chart__readout-cost">{{ formatUsdCompact(costsUsd[activeCost] ?? 0) }}</span>
          </template>
          <span v-else class="ov-chart__readout-hint">{{ t('overviewPage.v2.trendReadoutHint') }}</span>
        </div>
        <div
          class="ov-chart"
          tabindex="0"
          role="img"
          :aria-label="t('overviewPage.v2.trendCostTitle')"
          @pointermove="activeCost = indexFromPointer($event)"
          @pointerleave="activeCost = null"
          @focus="activeCost = labels.length - 1"
          @blur="activeCost = null"
          @keydown="onKey($event, 'cost')"
        >
          <span class="ov-chart__max">{{ formatUsdCompact(maxCost) }}</span>
          <svg
            class="ov-chart__svg"
            viewBox="0 0 600 150"
            preserveAspectRatio="none"
            aria-hidden="true"
            focusable="false"
          >
            <line v-for="y in GRID" :key="y" class="ov-chart__grid" x1="0" x2="600" :y1="y" :y2="y" />
            <rect
              v-if="activeCost !== null"
              class="ov-chart__band"
              :x="activeCost * slot"
              y="4"
              :width="slot"
              height="142"
            />
            <rect
              v-for="(v, i) in costsUsd"
              :key="i"
              class="ov-chart__bar--cost"
              :class="{ 'ov-chart__bar--active': activeCost === i }"
              :x="barX(i)"
              :y="146 - barH(v, maxCost)"
              :width="barW"
              :height="barH(v, maxCost)"
              rx="1.5"
            />
          </svg>
        </div>
        <div class="ov-trend__axis">
          <span>{{ labels[0] }}</span>
          <span v-if="labels.length > 2">{{ labels[Math.floor(labels.length / 2)] }}</span>
          <span>{{ labels[labels.length - 1] }}</span>
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatCount, formatUsdCompact } from '../../features/usage/moneyFormat';

const props = defineProps<{
  /** x 轴刻度文本（页面预格式化为本地时区短标签；首尾展示）。 */
  labels: string[];
  calls: number[];
  failures: number[];
  costsUsd: number[];
  granularity: 'day' | 'hour';
}>();

const { t } = useI18n();

const GRID = [8, 42, 77, 111, 146];
const PLOT_H = 138;

const caption = computed(() =>
  props.granularity === 'hour' ? t('overviewPage.v2.trendCaptionHour') : t('overviewPage.v2.trendCaptionDay'),
);

const maxCalls = computed(() => Math.max(1, ...props.calls));
const maxCost = computed(() => Math.max(1e-9, ...props.costsUsd));
const slot = computed(() => 600 / Math.max(1, props.labels.length));
const barW = computed(() => Math.min(slot.value * 0.62, 22));

const activeCalls = ref<number | null>(null);
const activeCost = ref<number | null>(null);

function barX(i: number): number {
  return i * slot.value + (slot.value - barW.value) / 2;
}

/** 零值柱高 0；非零保留最低可见高度，避免小值被读成「无」。 */
function barH(v: number, max: number): number {
  if (!Number.isFinite(v) || v <= 0) return 0;
  return Math.max((v / max) * PLOT_H, PLOT_H * 0.02);
}

function indexFromPointer(event: PointerEvent): number {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
  const ratio = (event.clientX - rect.left) / Math.max(1, rect.width);
  return Math.min(props.labels.length - 1, Math.max(0, Math.floor(ratio * props.labels.length)));
}

function onKey(event: KeyboardEvent, series: 'calls' | 'cost') {
  const active = series === 'calls' ? activeCalls : activeCost;
  const last = props.labels.length - 1;
  const current = active.value ?? last;
  let next: number | null;
  if (event.key === 'ArrowLeft') next = Math.max(0, current - 1);
  else if (event.key === 'ArrowRight') next = Math.min(last, current + 1);
  else if (event.key === 'Home') next = 0;
  else if (event.key === 'End') next = last;
  else if (event.key === 'Escape') next = null;
  else return;
  event.preventDefault();
  active.value = next;
}
</script>
