<template>
  <div class="industry-market-toolbar">
    <div class="industry-market-toolbar__search">
      <span class="industry-market-toolbar__search-icon">⌕</span>
      <input
        ref="searchInput"
        v-model="queryModel"
        type="text"
        class="industry-market-toolbar__search-input"
        :placeholder="t('industries.market.searchPlaceholder')"
        @keydown.meta.k.prevent="searchInput?.focus()"
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
      >
        ▦ {{ t('industries.market.viewGrid') }}
      </button>
      <button
        type="button"
        :class="['industry-market-toolbar__view-btn', { 'is-active': view === 'table' }]"
        @click="view = 'table'"
      >
        ≡ {{ t('industries.market.viewTable') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type {
  IndustryStatusFilter,
  IndustrySourceFilter,
  IndustrySummary,
} from '../../features/industries/useIndustryMarket';

const props = defineProps<{
  modelValue: {
    query: string;
    statusFilter: IndustryStatusFilter;
    sourceFilter: IndustrySourceFilter;
    view: 'grid' | 'table';
  };
  counts: IndustrySummary;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: typeof props.modelValue];
}>();

const { t } = useI18n();
const searchInput = ref<HTMLInputElement | null>(null);

const queryModel = computed({
  get: () => props.modelValue.query,
  set: (v: string) => emit('update:modelValue', { ...props.modelValue, query: v }),
});

const statusFilter = computed({
  get: () => props.modelValue.statusFilter,
  set: (v: IndustryStatusFilter) => emit('update:modelValue', { ...props.modelValue, statusFilter: v }),
});

const sourceFilter = computed({
  get: () => props.modelValue.sourceFilter,
  set: (v: IndustrySourceFilter) => emit('update:modelValue', { ...props.modelValue, sourceFilter: v }),
});

const view = computed({
  get: () => props.modelValue.view,
  set: (v: 'grid' | 'table') => emit('update:modelValue', { ...props.modelValue, view: v }),
});

const statusChips = computed(() => [
  { key: 'all' as const, label: t('industries.market.statusAll'), count: props.counts.total },
  { key: 'enabled' as const, label: t('industries.market.statusEnabled'), count: props.counts.enabled },
  { key: 'disabled' as const, label: t('industries.market.statusDisabled'), count: props.counts.disabled },
]);

const sourceChips = computed(() => [
  { key: 'all' as const, label: t('industries.market.sourceAll') },
  { key: 'system' as const, label: t('industries.market.sourceSystem') },
  { key: 'custom' as const, label: t('industries.market.sourceCustom') },
]);
</script>

<style lang="sass" scoped>
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
  color: var(--color-warning, #8a6014)

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

.app-mono
  font-family: 'JetBrains Mono', 'SF Mono', Menlo, monospace
  font-feature-settings: 'tnum' 1
</style>
