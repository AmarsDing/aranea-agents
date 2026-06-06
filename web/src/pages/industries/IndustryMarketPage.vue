<template>
  <q-page class="industry-market-page">
    <AppPageHero
      :kicker="t('industries.market.kicker')"
      :title="t('industries.market.title')"
      :subtitle="t('industries.market.subtitle')"
    >
      <template #actions>
        <q-btn flat rounded no-caps :label="`↻ ${t('industries.market.actionRefresh')}`" @click="refresh" />
        <q-btn flat rounded no-caps :label="`↥ ${t('industries.market.actionExport')}`" />
        <q-btn unelevated rounded no-caps color="primary" :label="`+ ${t('industries.market.actionRequestNew')}`" />
      </template>
    </AppPageHero>

    <!-- Metrics strip -->
    <IndustryMetricStrip :summary="summary" />

    <!-- Toolbar -->
    <IndustryMarketToolbar v-model="filters" :counts="summary" />

    <!-- Content: empty / grid / table -->
    <div v-if="filtered.length === 0" class="industry-market-page__empty">
      <h4>{{ t('industries.market.emptyTitle') }}</h4>
      <p>{{ t('industries.market.emptyHint') }}</p>
    </div>

    <div v-else-if="filters.view === 'grid'" class="industry-market-page__grid">
      <IndustryCard
        v-for="ind in filtered"
        :key="ind.key"
        :industry="ind"
        :is-open="openKey === ind.key"
        @select="openKey = openKey === ind.key ? null : ind.key"
      />
      <div class="industry-market-page__cta">
        <div class="industry-market-page__cta-plus">+</div>
        <div class="industry-market-page__cta-title">{{ t('industries.market.ctaTitle') }}</div>
        <small class="industry-market-page__cta-sub" v-html="t('industries.market.ctaSubtitle')" />
      </div>
    </div>

    <div v-else class="industry-market-table">
      <table>
        <thead>
          <tr>
            <th>{{ t('industries.market.title') }}</th>
            <th>{{ t('industries.market.tableDesc') }}</th>
            <th class="num">{{ t('industries.market.metricDept') }}</th>
            <th class="num">{{ t('industries.market.metricPos') }}</th>
            <th class="num">{{ t('industries.market.metricAgent') }}</th>
            <th class="num">{{ t('industries.market.metricInstalled') }}</th>
            <th>{{ t('industries.market.tableStatus') }}</th>
            <th>{{ t('industries.market.tableSource') }}</th>
          </tr>
        </thead>
        <tbody>
          <IndustryTableRow v-for="ind in filtered" :key="ind.key" :industry="ind" @select="openKey = ind.key" />
        </tbody>
      </table>
    </div>

    <!-- Signature quote (the 120% detail) -->
    <div class="industry-market-page__signature">
      <span class="industry-market-page__signature-rule" />
      <span class="industry-market-page__signature-quote industry-market-page__signature-quote--em">
        {{ t('industries.market.signatureQuote') }}
      </span>
      <span class="industry-market-page__signature-caption">
        {{ t('industries.market.signatureCaption') }}
      </span>
    </div>

    <!-- Drawer -->
    <IndustryDrawer
      v-model="drawerOpen"
      :industry="activeIndustry"
      :detail="industryDetail"
      :detail-loading="detailLoading"
      @install="onInstall"
      @view-prompts="onViewPrompts"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../../components/layout/AppPageHero.vue';
import IndustryMetricStrip from '../../components/industries/IndustryMetricStrip.vue';
import IndustryMarketToolbar from '../../components/industries/IndustryMarketToolbar.vue';
import IndustryCard from '../../components/industries/IndustryCard.vue';
import IndustryTableRow from '../../components/industries/IndustryTableRow.vue';
import IndustryDrawer from '../../components/industries/IndustryDrawer.vue';
import {
  useIndustryMarket,
  type IndustryStatusFilter,
  type IndustrySourceFilter,
} from '../../features/industries/useIndustryMarket';
import type { Industry } from '../../features/industries/types';

const { t } = useI18n();
const {
  industries,
  loading,
  error,
  summary,
  fetchIndustries,
  applyFilters,
  industryDetail,
  detailLoading,
  fetchIndustryDetail,
  clearIndustryDetail,
} = useIndustryMarket();

// 本地 UI 状态（通过 v-model 传给 IndustryMarketToolbar）
const filters = ref({
  query: '',
  statusFilter: 'all' as IndustryStatusFilter,
  sourceFilter: 'all' as IndustrySourceFilter,
  view: 'grid' as 'grid' | 'table',
});

const openKey = ref<string | null>(null);

const filtered = computed<Industry[]>(() =>
  applyFilters({
    query: filters.value.query,
    status: filters.value.statusFilter,
    source: filters.value.sourceFilter,
  }),
);

const drawerOpen = computed({
  get: () => openKey.value !== null,
  set: (v: boolean) => {
    if (!v) openKey.value = null;
  },
});

const activeIndustry = computed<Industry | null>(() => {
  if (!openKey.value) return null;
  return industries.value.find((i) => i.key === openKey.value) ?? null;
});

function refresh() {
  fetchIndustries();
}

function onInstall(_ind: Industry) {
  // TODO: 接入安装流程（独立 change）
  drawerOpen.value = false;
}

function onViewPrompts(_ind: Industry) {
  // TODO: 接入查看 Prompt 流程
  drawerOpen.value = false;
}

// 当 drawer 打开时，拉取行业详情（部门+岗位）
watch(openKey, (key) => {
  if (key) {
    fetchIndustryDetail(key);
  } else {
    clearIndustryDetail();
  }
});

onMounted(() => {
  fetchIndustries();
});
</script>

<style lang="sass" scoped>
.industry-market-page
  position: relative

/* Empty state */
.industry-market-page__empty
  border: 1px dashed var(--color-border-soft, rgba(141, 110, 99, 0.12))
  border-radius: 18px
  padding: 48px 24px
  text-align: center
  background: var(--glass-surface, rgba(255, 253, 245, 0.65))

  h4
    margin: 0 0 4px
    font-size: 15px
    font-weight: 600

  p
    margin: 0
    color: var(--color-text-tertiary, #5A6A7E)
    font-size: 13px

/* Grid view */
.industry-market-page__grid
  display: grid
  grid-template-columns: repeat(3, 1fr)
  gap: 14px

  @media (max-width: 1024px)
    grid-template-columns: repeat(2, 1fr)

  @media (max-width: 640px)
    grid-template-columns: 1fr

.industry-market-page__cta
  display: flex
  align-items: center
  justify-content: center
  flex-direction: column
  gap: 8px
  min-height: 232px
  padding: 18px 20px
  border: 1.5px dashed var(--color-border-soft, rgba(141, 110, 99, 0.12))
  border-radius: 18px
  background: transparent
  color: var(--color-text-tertiary, #5A6A7E)
  text-align: center
  cursor: pointer
  transition: color 0.18s, border-color 0.18s

  &:hover
    color: var(--color-accent, #DCA03E)
    border-color: rgba(220, 160, 62, 0.45)

.industry-market-page__cta-plus
  width: 32px
  height: 32px
  border-radius: 50%
  display: grid
  place-items: center
  border: 1.5px solid currentColor
  font-size: 18px
  line-height: 1

.industry-market-page__cta-title
  font-size: 14px
  font-weight: 600
  color: var(--color-text-primary, #2C2218)

.industry-market-page__cta-sub
  font-size: 11.5px
  color: var(--color-text-tertiary, #5A6A7E)
  line-height: 1.5

/* Table view */
.industry-market-table
  border: 1px solid var(--glass-border, rgba(235, 220, 200, 0.7))
  border-radius: 16px
  background: var(--glass-surface, rgba(255, 253, 245, 0.65))
  backdrop-filter: blur(18px)
  -webkit-backdrop-filter: blur(18px)
  overflow: hidden
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.45)

  table
    width: 100%
    border-collapse: collapse

  th
    text-align: left
    font-size: 11px
    font-weight: 600
    letter-spacing: 0.06em
    text-transform: uppercase
    color: var(--color-text-tertiary, #5A6A7E)
    padding: 12px 16px
    background: rgba(255, 253, 245, 0.5)
    border-bottom: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))

  .num
    text-align: right
    font-variant-numeric: tabular-nums

/* Signature */
.industry-market-page__signature
  margin-top: 36px
  padding: 20px 0
  border-top: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
  display: flex
  align-items: center
  gap: 14px
  color: var(--color-text-tertiary, #5A6A7E)
  font-size: 12.5px
  flex-wrap: wrap

.industry-market-page__signature-rule
  width: 32px
  height: 1px
  background: var(--color-accent, #DCA03E)
  flex-shrink: 0

.industry-market-page__signature-quote
  color: var(--color-text-secondary, #6B5B4D)

.industry-market-page__signature-quote--em
  font-style: italic

.industry-market-page__signature-caption
  color: var(--color-text-tertiary, #5A6A7E)
  font-size: 12px
</style>
