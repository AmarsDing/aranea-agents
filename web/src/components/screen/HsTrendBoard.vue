<template>
  <section class="hs-panel hs-trend">
    <header class="hs-head">
      <q-icon name="ssid_chart" size="18px" class="hs-head__icon" />
      <h2 class="hs-head__title">{{ t('harnessScreen.trend.title') }}</h2>
      <span class="hs-head__cap">{{ t(hourly ? 'harnessScreen.trend.hourly' : 'harnessScreen.trend.daily') }}</span>
      <q-btn
        v-if="to"
        class="hs-head__link"
        flat
        dense
        no-caps
        size="sm"
        icon-right="chevron_right"
        :to="to"
        :label="t('harnessScreen.detail')"
      />
    </header>
    <div v-if="!labels.length" class="hs-empty">{{ t('harnessScreen.trend.empty') }}</div>
    <HsChart
      v-else
      class="hs-trend__chart"
      :build="build"
      :deps="[labels, calls, costsUsd]"
      :label="t('harnessScreen.trend.title')"
    />
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import HsChart from './HsChart.vue';
import { trendOption } from './hsCharts';

const props = defineProps<{ labels: string[]; calls: number[]; costsUsd: number[]; hourly: boolean; to?: string }>();
const { t } = useI18n();
const build = () =>
  trendOption(props.labels, props.calls, props.costsUsd, {
    calls: t('harnessScreen.trend.calls'),
    cost: t('harnessScreen.trend.cost'),
  });
</script>
