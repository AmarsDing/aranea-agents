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
      <div v-else class="ov-trend__bars" role="img" :aria-label="t('overviewPage.v2.trendCallsTitle')">
        <div
          v-for="(v, i) in calls"
          :key="i"
          class="ov-trend__col"
          :title="`${labels[i]} · ${t('overviewPage.v2.trendCallsLegend')} ${v} · ${t('overviewPage.v2.trendFailuresLegend')} ${failures[i] ?? 0}`"
        >
          <div class="ov-trend__bar ov-trend__bar--fails" :style="{ height: pct(failures[i] ?? 0, maxCalls) }" />
          <div class="ov-trend__bar ov-trend__bar--calls" :style="{ height: pct(v, maxCalls) }" />
        </div>
      </div>
      <div v-if="labels.length" class="ov-trend__axis">
        <span>{{ labels[0] }}</span>
        <span>{{ labels[labels.length - 1] }}</span>
      </div>
    </div>

    <div class="ov-trend__panel">
      <header class="ov-section-head">
        <q-icon name="show_chart" size="18px" class="ov-section-head__icon" />
        <h2 class="ov-section-head__title">{{ t('overviewPage.v2.trendCostTitle') }}</h2>
        <span class="ov-section-head__cap">{{ caption }}</span>
      </header>
      <div v-if="!labels.length" class="ov-trend__empty">{{ t('overviewPage.v2.trendEmpty') }}</div>
      <div v-else class="ov-trend__bars" role="img" :aria-label="t('overviewPage.v2.trendCostTitle')">
        <div v-for="(v, i) in costsUsd" :key="i" class="ov-trend__col" :title="`${labels[i]} · ${formatUsdCompact(v)}`">
          <div class="ov-trend__bar ov-trend__bar--cost" :style="{ height: pct(v, maxCost) }" />
        </div>
      </div>
      <div v-if="labels.length" class="ov-trend__axis">
        <span>{{ labels[0] }}</span>
        <span>{{ labels[labels.length - 1] }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatUsdCompact } from '../../features/usage/moneyFormat';

const props = defineProps<{
  /** x 轴刻度文本（页面预格式化为本地时区短标签；首尾展示）。 */
  labels: string[];
  calls: number[];
  failures: number[];
  costsUsd: number[];
  granularity: 'day' | 'hour';
}>();

const { t } = useI18n();

const caption = computed(() =>
  props.granularity === 'hour' ? t('overviewPage.v2.trendCaptionHour') : t('overviewPage.v2.trendCaptionDay'),
);

const maxCalls = computed(() => Math.max(1, ...props.calls));
const maxCost = computed(() => Math.max(1e-9, ...props.costsUsd));

function pct(v: number, max: number): string {
  if (!Number.isFinite(v) || v <= 0) return '0%';
  return `${Math.max((v / max) * 100, 2).toFixed(1)}%`;
}
</script>
