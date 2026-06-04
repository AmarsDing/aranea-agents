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
        <q-btn
          unelevated
          rounded
          no-caps
          color="primary"
          :label="`+ ${t('industries.market.actionRequestNew')}`"
        />
      </template>
    </AppPageHero>

    <!-- Metrics strip -->
    <div class="app-metrics-grid industry-market-page__metrics">
      <div class="app-metrics-card">
        <div class="app-metrics-card__label">{{ t('industries.market.metricEnabled') }}</div>
        <span class="app-metrics-card__value app-mono">{{ summary.enabled }}</span>
        <div class="app-metrics-card__foot">
          <span class="industry-market-page__delta-up">{{ t('industries.market.metricEnabledDelta') }}</span>
          {{ t('industries.market.metricEnabledFootNoDelta') }}
        </div>
      </div>
      <div class="app-metrics-card">
        <div class="app-metrics-card__label">{{ t('industries.market.metricDepartments') }}</div>
        <span class="app-metrics-card__value app-mono">{{ summary.departments }}</span>
        <div class="app-metrics-card__foot">{{ t('industries.market.metricDepartmentsFoot') }}</div>
      </div>
      <div class="app-metrics-card">
        <div class="app-metrics-card__label">{{ t('industries.market.metricPositions') }}</div>
        <span class="app-metrics-card__value app-mono">{{ summary.positions }}</span>
        <div class="app-metrics-card__foot">
          {{ t('industries.market.metricPositionsFoot', { ratio: avgAgentsPerPosition }) }}
        </div>
      </div>
      <div class="app-metrics-card">
        <div class="app-metrics-card__label">{{ t('industries.market.metricAgents') }}</div>
        <span class="app-metrics-card__value app-mono">{{ summary.agents }}</span>
        <div class="app-metrics-card__foot">
          {{ t('industries.market.metricAgentsFoot', { installed: summary.installed }) }}
        </div>
      </div>
    </div>

    <!-- Toolbar -->
    <div class="industry-market-toolbar">
      <div class="industry-market-toolbar__search">
        <span class="industry-market-toolbar__search-icon">⌕</span>
        <input
          v-model="query"
          type="text"
          class="industry-market-toolbar__search-input"
          :placeholder="t('industries.market.searchPlaceholder')"
        />
        <span class="industry-market-toolbar__search-kbd">{{ t('industries.market.searchKbd') }}</span>
      </div>

      <div class="industry-market-toolbar__chips">
        <span class="industry-market-toolbar__chips-label">状态</span>
        <button
          v-for="c in statusChips"
          :key="c.key"
          type="button"
          :class="['industry-market-chip', { 'is-active': statusFilter === c.key }]"
          @click="statusFilter = c.key"
        >
          {{ c.label }}<span class="industry-market-chip__count app-mono">{{ c.count }}</span>
        </button>
      </div>

      <div class="industry-market-toolbar__chips">
        <span class="industry-market-toolbar__chips-label">来源</span>
        <button
          v-for="c in sourceChips"
          :key="c.key"
          type="button"
          :class="['industry-market-chip', { 'is-active': sourceFilter === c.key }]"
          @click="sourceFilter = c.key"
        >
          {{ c.label }}
        </button>
      </div>

      <div class="industry-market-toolbar__spacer" />

      <div class="industry-market-toolbar__view-toggle" role="tablist">
        <button
          type="button"
          :class="['industry-market-toolbar__view-btn', { 'is-active': view === 'grid' }]"
          @click="view = 'grid'"
        >▦ {{ t('industries.market.viewGrid') }}</button>
        <button
          type="button"
          :class="['industry-market-toolbar__view-btn', { 'is-active': view === 'table' }]"
          @click="view = 'table'"
        >≡ {{ t('industries.market.viewTable') }}</button>
      </div>
    </div>

    <!-- Content: empty / grid / table -->
    <div v-if="filtered.length === 0" class="industry-market-page__empty">
      <h4>{{ t('industries.market.emptyTitle') }}</h4>
      <p>{{ t('industries.market.emptyHint') }}</p>
    </div>

    <div v-else-if="view === 'grid'" class="industry-market-page__grid">
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
            <th>描述</th>
            <th class="num">{{ t('industries.market.metricDept') }}</th>
            <th class="num">{{ t('industries.market.metricPos') }}</th>
            <th class="num">{{ t('industries.market.metricAgent') }}</th>
            <th class="num">{{ t('industries.market.metricInstalled') }}</th>
            <th>状态</th>
            <th>来源</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="ind in filtered" :key="ind.key" @click="openKey = ind.key">
            <td>
              <div class="industry-market-table__cell-primary">
                <div class="industry-market-table__mono" :style="{ background: monoBgFor(ind.key) }">
                  {{ monoFor(ind.key) }}
                </div>
                <div>
                  <div class="industry-market-table__name">{{ ind.name }}</div>
                  <div class="industry-market-table__key app-mono">{{ ind.key }}</div>
                </div>
              </div>
            </td>
            <td class="industry-market-table__desc">{{ ind.description }}</td>
            <td class="num app-mono">{{ ind.deptCount ?? 0 }}</td>
            <td class="num app-mono">{{ ind.posCount ?? 0 }}</td>
            <td class="num app-mono">{{ ind.agentCount ?? ind.posCount ?? 0 }}</td>
            <td class="num app-mono">{{ ind.installed ?? 0 }}</td>
            <td>
              <span :class="['industry-market-table__status', ind.enabled ? 'is-on' : 'is-off']">
                <span class="industry-market-table__status-dot" />
                {{ ind.enabled ? t('industries.market.statusEnabled') : t('industries.market.statusDisabled') }}
              </span>
            </td>
            <td class="industry-market-table__source">{{ t('industries.market.sourceSystem') }}</td>
          </tr>
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
      @install="onInstall"
      @view-prompts="onViewPrompts"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../../components/layout/AppPageHero.vue';
import IndustryCard from '../../components/industries/IndustryCard.vue';
import IndustryDrawer from '../../components/industries/IndustryDrawer.vue';
import { useIndustryMarket, type IndustryStatusFilter, type IndustrySourceFilter } from '../../features/industries/useIndustryMarket';
import type { Industry } from '../../features/industries/types';

const { t } = useI18n();
const { industries, loading, error, summary, fetchIndustries, applyFilters } = useIndustryMarket();

// 本地 UI 状态
const query = ref('');
const statusFilter = ref<IndustryStatusFilter>('all');
const sourceFilter = ref<IndustrySourceFilter>('all');
const view = ref<'grid' | 'table'>('grid');
const openKey = ref<string | null>(null);

const filtered = computed<Industry[]>(() =>
  applyFilters({
    query: query.value,
    status: statusFilter.value,
    source: sourceFilter.value,
  }),
);

const statusChips = computed(() => [
  { key: 'all' as const, label: t('industries.market.statusAll'), count: summary.value.total },
  { key: 'enabled' as const, label: t('industries.market.statusEnabled'), count: summary.value.enabled },
  { key: 'disabled' as const, label: t('industries.market.statusDisabled'), count: summary.value.disabled },
]);

const sourceChips = computed(() => [
  { key: 'all' as const, label: t('industries.market.sourceAll') },
  { key: 'system' as const, label: t('industries.market.sourceSystem') },
  { key: 'custom' as const, label: t('industries.market.sourceCustom') },
]);

const avgAgentsPerPosition = computed(() => {
  if (summary.value.positions === 0) return '0';
  return (summary.value.agents / summary.value.positions).toFixed(1);
});

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

// monogram 工具（与 IndustryCard 同步 — 行业颜色板）
const PALETTES = [
  'linear-gradient(135deg, #4F46E5 0%, #312E81 100%)',
  'linear-gradient(135deg, #E55C5C 0%, #9B2226 100%)',
  'linear-gradient(135deg, #0EA5E9 0%, #075985 100%)',
  'linear-gradient(135deg, #10B981 0%, #065F46 100%)',
  'linear-gradient(135deg, #F59E0B 0%, #92400E 100%)',
  'linear-gradient(135deg, #8B5CF6 0%, #4C1D95 100%)',
];

function monoBgFor(key: string): string {
  let h = 0;
  for (let i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) | 0;
  return PALETTES[Math.abs(h) % PALETTES.length];
}

function monoFor(key: string): string {
  const cleaned = key.replace(/[^a-zA-Z]/g, '').toUpperCase();
  if (cleaned.length >= 2) return cleaned.slice(0, 2);
  if (cleaned.length === 1) return cleaned + cleaned;
  return key.slice(0, 2);
}

onMounted(() => {
  fetchIndustries();
});
</script>

<style lang="sass" scoped>
.industry-market-page
  // 父 q-page 已提供 padding
  position: relative

.industry-market-page__metrics
  margin: 20px 0

.industry-market-page__delta-up
  color: var(--color-success, #4CAF7C)
  font-weight: 600

/* Toolbar */
.industry-market-toolbar
  display: flex
  align-items: center
  gap: 12px
  flex-wrap: wrap
  padding: 12px 14px
  border: 1px solid var(--glass-border, rgba(235, 220, 200, 0.7))
  border-radius: 16px
  background: var(--glass-surface, rgba(255, 253, 245, 0.65))
  backdrop-filter: blur(18px)
  -webkit-backdrop-filter: blur(18px)
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.45)
  margin-bottom: 16px

.industry-market-toolbar__search
  display: flex
  align-items: center
  gap: 8px
  background: var(--color-surface-solid, #FFFFFF)
  border: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
  border-radius: 10px
  padding: 7px 10px
  min-width: 280px
  transition: border-color 0.12s, box-shadow 0.12s

  &:focus-within
    border-color: var(--color-accent, #DCA03E)
    box-shadow: 0 0 0 3px rgba(220, 160, 62, 0.15)

.industry-market-toolbar__search-icon
  color: var(--color-icon-muted, #A89580)
  font-size: 14px

.industry-market-toolbar__search-input
  flex: 1
  border: 0
  outline: 0
  background: transparent
  font-size: 13px
  color: var(--color-text-primary, #2C2218)

  &::placeholder
    color: var(--color-icon-muted, #A89580)

.industry-market-toolbar__search-kbd
  font-size: 11px
  color: var(--color-text-tertiary, #5A6A7E)
  border: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
  border-radius: 4px
  padding: 1px 5px
  background: var(--canvas-base, #FEFBF4)

.industry-market-toolbar__chips
  display: flex
  align-items: center
  gap: 6px
  flex-wrap: wrap

.industry-market-toolbar__chips-label
  font-size: 11px
  color: var(--color-text-tertiary, #5A6A7E)
  margin-right: 4px

.industry-market-chip
  display: inline-flex
  align-items: center
  gap: 6px
  padding: 5px 10px
  border-radius: 999px
  font-size: 12.5px
  font-weight: 500
  background: transparent
  color: var(--color-text-secondary, #6B5B4D)
  border: 1px solid transparent
  cursor: pointer
  transition: all 0.12s

  &:hover
    background: var(--interaction-surface-hover, #FDF6E8)

  &.is-active
    background: var(--color-surface-solid, #FFFFFF)
    border-color: var(--color-accent, #DCA03E)
    color: var(--color-text-primary, #2C2218)
    box-shadow: 0 0 0 1px var(--color-accent, #DCA03E)

.industry-market-chip__count
  font-size: 11px
  padding: 0 5px
  border-radius: 6px
  background: rgba(141, 110, 99, 0.1)
  color: var(--color-text-secondary, #6B5B4D)

.is-active .industry-market-chip__count
  background: rgba(220, 160, 62, 0.18)
  color: #8a6014

.industry-market-toolbar__spacer
  flex: 1

.industry-market-toolbar__view-toggle
  display: flex
  background: var(--color-surface-solid, #FFFFFF)
  border: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
  border-radius: 10px
  padding: 2px

.industry-market-toolbar__view-btn
  padding: 5px 10px
  border-radius: 8px
  font-size: 12.5px
  color: var(--color-text-secondary, #6B5B4D)
  background: transparent
  border: 0
  cursor: pointer
  display: inline-flex
  align-items: center
  gap: 4px

  &.is-active
    background: var(--interaction-surface-hover, #FDF6E8)
    color: var(--color-text-primary, #2C2218)

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

  td
    padding: 14px 16px
    font-size: 13px
    border-bottom: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
    vertical-align: middle

  tbody tr
    transition: background 0.1s
    cursor: pointer

    &:hover
      background: var(--interaction-surface-hover, #FDF6E8)

    &:last-child td
      border-bottom: 0

  .num
    text-align: right
    font-variant-numeric: tabular-nums

.industry-market-table__cell-primary
  display: flex
  align-items: center
  gap: 10px

.industry-market-table__mono
  width: 32px
  height: 32px
  border-radius: 8px
  display: grid
  place-items: center
  font-weight: 700
  font-size: 12px
  color: #fff
  flex-shrink: 0

.industry-market-table__name
  font-weight: 600

.industry-market-table__key
  font-size: 11px
  color: var(--color-text-tertiary, #5A6A7E)

.industry-market-table__desc
  color: var(--color-text-secondary, #6B5B4D)
  max-width: 28ch
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.industry-market-table__status
  display: inline-flex
  align-items: center
  gap: 4px
  font-size: 11.5px
  padding: 3px 8px
  border-radius: 999px

  &.is-on
    background: var(--color-success-soft, #ECFDF3)
    color: #2D6A4F

  &.is-off
    background: rgba(229, 92, 92, 0.1)
    color: #B13939

.industry-market-table__status-dot
  width: 6px
  height: 6px
  border-radius: 50%
  background: currentColor

.industry-market-table__source
  color: var(--color-text-tertiary, #5A6A7E)
  font-size: 11.5px
  letter-spacing: 0.04em
  text-transform: uppercase

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
