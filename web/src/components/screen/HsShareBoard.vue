<template>
  <section class="hs-panel hs-share">
    <header class="hs-head">
      <q-icon name="donut_large" size="18px" class="hs-head__icon" />
      <h2 class="hs-head__title">{{ t('harnessScreen.share.title') }}</h2>
      <span v-if="rows.length" class="hs-head__cap">{{
        t(byCost ? 'harnessScreen.share.byCost' : 'harnessScreen.share.byTokens')
      }}</span>
      <q-btn-toggle
        class="hs-head__tail"
        :model-value="mode"
        dense
        no-caps
        unelevated
        toggle-color="primary"
        :options="[
          { label: t('harnessScreen.share.model'), value: 'model' },
          { label: t('harnessScreen.share.agent'), value: 'agent' },
        ]"
        :aria-label="t('harnessScreen.share.title')"
        @update:model-value="(v) => $emit('update:mode', v)"
      />
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
    <div v-if="!rows.length" class="hs-empty">{{ t('harnessScreen.share.empty') }}</div>
    <div v-else class="hs-share__body">
      <div class="hs-share__donut">
        <HsChart :build="build" :deps="[rows]" :label="t('harnessScreen.share.title')" />
        <div class="hs-share__center" aria-hidden="true">
          <span class="hs-share__total hs-num">{{ total }}</span>
          <span class="hs-share__total-cap">{{ t('harnessScreen.share.total') }}</span>
        </div>
      </div>
      <ol class="hs-share__list">
        <li v-for="(r, i) in rows" :key="r.key" class="hs-share__row">
          <i class="hs-dot" :class="`hs-dot--${(i % 6) + 1}`" aria-hidden="true" />
          <span class="hs-share__name" :title="r.label">{{ r.label }}</span>
          <span class="hs-share__pct hs-num">{{ pct(r.share) }}</span>
          <span class="hs-share__cost hs-num" :class="{ 'hs-share__cost--unpriced': r.unpriced }">{{
            r.unpriced ? t('harnessScreen.share.unpriced') : r.cost
          }}</span>
          <span class="hs-share__meta hs-num">{{
            t('harnessScreen.share.meta', { tokens: r.tokens, calls: r.calls })
          }}</span>
          <span class="hs-share__bar" aria-hidden="true">
            <i :class="`hs-fill hs-fill--${(i % 6) + 1}`" :style="{ width: `${Math.max(r.share * 100, 1)}%` }" />
          </span>
        </li>
      </ol>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import HsChart from './HsChart.vue';
import { donutOption } from './hsCharts';
import type { HsShareRow } from './hsTypes';

const props = defineProps<{
  rows: HsShareRow[];
  byCost: boolean;
  total: string;
  mode: 'model' | 'agent';
  to?: string;
}>();
defineEmits<{ 'update:mode': [value: 'model' | 'agent'] }>();
const { t } = useI18n();

const pct = (share: number) => `${(share * 100).toFixed(share >= 0.1 ? 0 : 1)}%`;
const build = () =>
  donutOption(
    props.rows.map((r) => ({
      label: r.label,
      share: r.share,
      tip: `${r.label}<br/>${pct(r.share)} · ${r.cost} · ${t('harnessScreen.share.meta', { tokens: r.tokens, calls: r.calls })}`,
    })),
  );
</script>
