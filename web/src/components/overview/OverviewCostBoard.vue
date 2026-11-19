<template>
  <section class="ov-cost">
    <header class="ov-section-head">
      <q-icon name="payments" size="18px" class="ov-section-head__icon" />
      <h2 class="ov-section-head__title">{{ t('overviewPage.v2.costTitle') }}</h2>
      <span class="ov-section-head__cap">{{ t('overviewPage.v2.costCap') }}</span>
    </header>
    <div class="ov-cost__grid">
      <OverviewStatTile
        class="ov-cost__cell"
        :label="t('overviewPage.v2.costRange')"
        :value="costValueUsd"
        :delta="costDelta"
        delta-good-when-negative
        :caption="t('overviewPage.v2.deltaVsYesterday')"
        tone="info"
        clickable
        @drill="$emit('drill', 'detail')"
      />

      <div class="ov-budget ov-cost__cell">
        <div class="ov-tile__top">
          <span class="ov-tile__label">{{ t('overviewPage.v2.budgetBurnTitle') }}</span>
          <span class="ov-tile__dot" :class="`ov-tile__dot--${budget.tone}`" aria-hidden="true" />
        </div>
        <template v-if="available && budget.configured">
          <div class="ov-budget__pct">{{ (budget.pct * 100).toFixed(1) }}%</div>
          <div
            class="ov-budget__bar"
            role="progressbar"
            :aria-valuenow="budget.pct * 100"
            aria-valuemin="0"
            aria-valuemax="100"
          >
            <div
              class="ov-budget__fill"
              :class="`ov-budget__fill--${budget.tone}`"
              :style="{ width: `${Math.min(budget.pct * 100, 100)}%` }"
            />
          </div>
          <div class="ov-budget__meta">
            {{ t('overviewPage.v2.budgetUsed', { spent: budget.spentUsd, cap: budget.capUsd }) }}
          </div>
          <div v-if="budget.forecastUsd" class="ov-budget__meta">
            {{ t('overviewPage.v2.budgetForecast', { value: budget.forecastUsd }) }}
            <span v-if="budget.lowConfidence" class="ov-budget__lowconf">{{
              t('overviewPage.v2.budgetForecastLowConf')
            }}</span>
          </div>
        </template>
        <div v-else class="ov-budget__empty">
          {{ t(available ? 'overviewPage.v2.budgetNoConfig' : 'uiSystem.overview.dataUnavailable') }}
        </div>
      </div>

      <div
        class="ov-cost__chipcell ov-cost__cell"
        :class="[
          !available ? '' : unpricedCount > 0 ? 'ov-cost__chipcell--warn' : 'ov-cost__chipcell--ok',
          { 'ov-cost__chipcell--clickable': unpricedCount > 0 },
        ]"
        :role="unpricedCount > 0 ? 'button' : undefined"
        :tabindex="unpricedCount > 0 ? 0 : undefined"
        @click="unpricedCount > 0 && $emit('drill', 'unpriced')"
        @keydown.enter="unpricedCount > 0 && $emit('drill', 'unpriced')"
      >
        <div class="ov-cost__chiprow">
          <q-icon :name="unpricedCount > 0 ? 'price_change' : 'sell'" size="15px" />
          <span class="ov-cost__chipvalue">{{
            !available
              ? t('uiSystem.overview.pricingUnknown')
              : unpricedCount > 0
                ? t('overviewPage.v2.unpricedChip', { count: unpricedCount })
                : t('overviewPage.v2.unpricedNone')
          }}</span>
        </div>
        <div class="ov-tile__caption">{{ t('overviewPage.v2.unpricedCap') }}</div>
      </div>

      <div
        v-if="!cache.denied"
        class="ov-cost__chipcell ov-cost__cell"
        :class="cache.low ? 'ov-cost__chipcell--warn' : 'ov-cost__chipcell--ok'"
      >
        <div class="ov-cost__chiprow">
          <q-icon name="cached" size="15px" />
          <span class="ov-cost__chipvalue">
            {{
              cache.samples
                ? t('overviewPage.v2.cacheHitChip', { pct: (cache.p50 * 100).toFixed(1) })
                : t('overviewPage.v2.cacheHitEmpty')
            }}
          </span>
        </div>
        <div class="ov-tile__caption">
          {{ cache.low ? t('overviewPage.v2.cacheLow') : t('overviewPage.v2.cacheCap') }}
        </div>
      </div>
    </div>

    <div v-if="topBars.length || agentBars.length" class="ov-topbars">
      <div class="ov-topbars__head">
        <span class="ov-tile__label">{{ t('overviewPage.v2.topCostTitle') }}</span>
        <span class="ov-tile__caption">{{ t('overviewPage.v2.topCostCap') }}</span>
        <q-btn-toggle
          v-model="by"
          dense
          outline
          no-caps
          class="ov-topbars__by"
          :options="[
            { label: t('overviewPage.v2.spendByModel'), value: 'model' },
            { label: t('overviewPage.v2.spendByAgent'), value: 'agent' },
          ]"
        />
      </div>
      <div v-if="!bars.length" class="ov-tile__caption">{{ t('overviewPage.v2.spendEmpty') }}</div>
      <div v-for="(bar, i) in bars" :key="bar.key" class="ov-topbars__row">
        <span class="ov-topbars__rank" aria-hidden="true">{{ String(i + 1).padStart(2, '0') }}</span>
        <span class="ov-topbars__label" :title="bar.key">{{ bar.label }}</span>
        <div class="ov-topbars__track">
          <div
            class="ov-topbars__fill"
            :class="{ 'ov-topbars__fill--unpriced': bar.unpriced }"
            :style="{ width: `${(bar.ratio * 100).toFixed(1)}%` }"
          />
        </div>
        <span class="ov-topbars__value">
          {{ bar.unpriced ? t('overviewPage.v2.unpricedMark') : formatUsdCompactFromUsd(bar.costUsd) }}
        </span>
      </div>
    </div>

    <!-- 编码终端：terminals 为 undefined 时还没取回，不占位；null 是取不到，如实写明 -->
    <div v-if="terminals !== undefined" class="ov-topbars">
      <div class="ov-topbars__head">
        <span class="ov-tile__label">{{ t('overviewPage.v2.termTitle') }}</span>
        <span class="ov-tile__caption">{{ t('overviewPage.v2.termCap') }}</span>
      </div>
      <div v-if="!terminals" class="ov-tile__caption">{{ t('overviewPage.v2.termUnavailable') }}</div>
      <div v-else-if="!terminals.length" class="ov-tile__caption">{{ t('overviewPage.v2.termEmpty') }}</div>
      <div v-for="term in terminals ?? []" :key="term.agent" class="ov-terms__row">
        <span class="ov-topbars__label" :title="term.source">{{ term.agent }}</span>
        <div class="ov-terms__wins">
          <div v-for="w in term.windows" :key="w.label" class="ov-terms__win" :title="windowTip(w)">
            <span class="ov-tile__caption">{{ w.label }}</span>
            <div
              v-if="w.percent >= 0"
              class="ov-topbars__track"
              role="progressbar"
              :aria-label="`${term.agent} ${w.label}`"
              :aria-valuenow="Math.round(w.percent)"
              aria-valuemin="0"
              aria-valuemax="100"
            >
              <div
                class="ov-topbars__fill ov-terms__fill"
                :class="{
                  'ov-terms__fill--warn': w.percent >= 70 && w.percent < 90,
                  'ov-terms__fill--danger': w.percent >= 90,
                }"
                :style="{ width: `${Math.min(w.percent, 100)}%` }"
              />
            </div>
            <span v-else class="ov-tile__caption">{{ t('overviewPage.v2.termQuotaUnknown') }}</span>
            <span class="ov-topbars__value">{{
              w.percent >= 0 ? `${Math.round(w.percent)}%` : formatCompactCount(w.tokens)
            }}</span>
          </div>
        </div>
        <span class="ov-tile__caption">{{ t('overviewPage.v2.termSessions', { count: term.sessions }) }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import OverviewStatTile from './OverviewStatTile.vue';
import { formatCompactCount, formatCount, formatUsdCompactFromUsd } from '../../features/usage/moneyFormat';
import type { OvBudget, OvCacheHit, OvTerminal, OvTopBar } from './ovTypes';

const props = withDefaults(
  defineProps<{
    available?: boolean;
    costValueUsd: string;
    costDelta: number | null;
    budget: OvBudget;
    unpricedCount: number;
    topBars: OvTopBar[];
    agentBars?: OvTopBar[];
    cache: OvCacheHit;
    terminals?: OvTerminal[] | null;
  }>(),
  { available: true, agentBars: () => [], terminals: undefined },
);

defineEmits<{ drill: [key: string] }>();

const { t } = useI18n();

const by = ref<'model' | 'agent'>('model');
const bars = computed(() => (by.value === 'model' ? props.topBars : props.agentBars));

function windowTip(w: OvTerminal['windows'][number]) {
  const tip = t('overviewPage.v2.termWindowTip', { label: w.label, tokens: formatCount(w.tokens) });
  if (!w.resetsAt) return tip;
  const at = new Date(w.resetsAt);
  const time = Number.isNaN(at.getTime()) ? w.resetsAt : at.toLocaleString();
  return `${tip} · ${t('overviewPage.v2.termResets', { time })}`;
}
</script>
