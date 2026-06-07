<template>
  <q-page class="org-market-page">
    <AppPageHero
      :kicker="t('organization.market.kicker')"
      :title="t('organization.market.title')"
      :subtitle="t('organization.market.subtitle')"
    >
      <template #actions>
        <q-btn flat rounded no-caps :label="`↻ ${t('organization.market.actionRefresh')}`" @click="refresh" />
        <q-btn flat rounded no-caps :label="`↥ ${t('organization.market.actionExport')}`" />
        <q-btn unelevated rounded no-caps color="primary" :label="`+ ${t('organization.market.actionRequestNew')}`" />
      </template>
    </AppPageHero>

    <!-- Metrics strip -->
    <OrgMetricStrip :summary="summary" />

    <!-- Toolbar -->
    <OrgMarketToolbar v-model="filters" :counts="summary" />

    <!-- Content: empty / grid / table -->
    <div v-if="filtered.length === 0" class="org-market-page__empty">
      <h4>{{ t('organization.market.emptyTitle') }}</h4>
      <p>{{ t('organization.market.emptyHint') }}</p>
    </div>

    <div v-else-if="filters.view === 'grid'" class="org-market-page__grid">
      <OrgCard
        v-for="comp in filtered"
        :key="comp.key"
        :company="comp"
        :is-open="openKey === comp.key"
        @select="openKey = openKey === comp.key ? null : comp.key"
      />
      <div class="org-market-page__cta">
        <div class="org-market-page__cta-plus">+</div>
        <div class="org-market-page__cta-title">{{ t('organization.market.ctaTitle') }}</div>
        <small class="org-market-page__cta-sub" v-html="t('organization.market.ctaSubtitle')" />
      </div>
    </div>

    <div v-else class="org-market-table">
      <table>
        <thead>
          <tr>
            <th>{{ t('organization.market.title') }}</th>
            <th>{{ t('organization.market.tableDesc') }}</th>
            <th class="num">{{ t('organization.market.metricDept') }}</th>
            <th class="num">{{ t('organization.market.metricPos') }}</th>
            <th class="num">{{ t('organization.market.metricAgent') }}</th>
            <th class="num">{{ t('organization.market.metricInstalled') }}</th>
            <th>{{ t('organization.market.tableStatus') }}</th>
            <th>{{ t('organization.market.tableSource') }}</th>
          </tr>
        </thead>
        <tbody>
          <OrgTableRow v-for="comp in filtered" :key="comp.key" :company="comp" @select="openKey = comp.key" />
        </tbody>
      </table>
    </div>

    <!-- Signature quote -->
    <div class="org-market-page__signature">
      <span class="org-market-page__signature-rule" />
      <span class="org-market-page__signature-quote org-market-page__signature-quote--em">
        {{ t('organization.market.signatureQuote') }}
      </span>
      <span class="org-market-page__signature-caption">
        {{ t('organization.market.signatureCaption') }}
      </span>
    </div>

    <!-- Drawer -->
    <OrgDrawer
      v-model="drawerOpen"
      :company="activeCompany"
      :detail="orgDetail"
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
import OrgMetricStrip from '../../components/organization/OrgMetricStrip.vue';
import OrgMarketToolbar from '../../components/organization/OrgMarketToolbar.vue';
import OrgCard from '../../components/organization/OrgCard.vue';
import OrgTableRow from '../../components/organization/OrgTableRow.vue';
import OrgDrawer from '../../components/organization/OrgDrawer.vue';
import {
  useOrgMarket,
  type OrgStatusFilter,
  type OrgSourceFilter,
} from '../../features/organization/useOrgMarket';
import type { Company } from '../../features/organization/types';

const { t } = useI18n();
const {
  companies,
  loading,
  error,
  summary,
  fetchCompanies,
  applyFilters,
  orgDetail,
  detailLoading,
  fetchOrgDetail,
  clearOrgDetail,
} = useOrgMarket();

const filters = ref({
  query: '',
  statusFilter: 'all' as OrgStatusFilter,
  sourceFilter: 'all' as OrgSourceFilter,
  view: 'grid' as 'grid' | 'table',
});

const openKey = ref<string | null>(null);

const filtered = computed<Company[]>(() =>
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

const activeCompany = computed<Company | null>(() => {
  if (!openKey.value) return null;
  return companies.value.find((c) => c.key === openKey.value) ?? null;
});

function refresh() {
  fetchCompanies();
}

function onInstall(_comp: Company) {
  drawerOpen.value = false;
}

function onViewPrompts(_comp: Company) {
  drawerOpen.value = false;
}

watch(openKey, (key) => {
  if (key) {
    fetchOrgDetail(key);
  } else {
    clearOrgDetail();
  }
});

onMounted(() => {
  fetchCompanies();
});
</script>

<style lang="sass" scoped>
.org-market-page
  position: relative

.org-market-page__empty
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

.org-market-page__grid
  display: grid
  grid-template-columns: repeat(3, 1fr)
  gap: 14px

  @media (max-width: 1024px)
    grid-template-columns: repeat(2, 1fr)

  @media (max-width: 640px)
    grid-template-columns: 1fr

.org-market-page__cta
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

.org-market-page__cta-plus
  width: 32px
  height: 32px
  border-radius: 50%
  display: grid
  place-items: center
  border: 1.5px solid currentColor
  font-size: 18px
  line-height: 1

.org-market-page__cta-title
  font-size: 14px
  font-weight: 600
  color: var(--color-text-primary, #2C2218)

.org-market-page__cta-sub
  font-size: 11.5px
  color: var(--color-text-tertiary, #5A6A7E)
  line-height: 1.5

.org-market-table
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

.org-market-page__signature
  margin-top: 36px
  padding: 20px 0
  border-top: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
  display: flex
  align-items: center
  gap: 14px
  color: var(--color-text-tertiary, #5A6A7E)
  font-size: 12.5px
  flex-wrap: wrap

.org-market-page__signature-rule
  width: 32px
  height: 1px
  background: var(--color-accent, #DCA03E)
  flex-shrink: 0

.org-market-page__signature-quote
  color: var(--color-text-secondary, #6B5B4D)

.org-market-page__signature-quote--em
  font-style: italic

.org-market-page__signature-caption
  color: var(--color-text-tertiary, #5A6A7E)
  font-size: 12px
</style>
