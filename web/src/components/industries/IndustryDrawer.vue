<template>
  <Teleport to="body">
    <Transition name="industry-drawer-fade">
      <div v-if="modelValue" class="industry-market-drawer-mask" @click="close" />
    </Transition>
    <Transition name="industry-drawer-slide">
      <aside
        v-if="modelValue && industry"
        class="industry-market-drawer"
        :aria-label="`行业详情 ${industry.name}`"
        role="dialog"
        tabindex="-1"
        @keydown.esc="close"
      >
        <header class="industry-market-drawer__head">
          <div class="industry-market-drawer__head-main">
            <div class="industry-market-drawer__mono" :style="{ background: monoBg }">{{ monoLetters }}</div>
            <div class="industry-market-drawer__title">
              <h2 class="industry-market-drawer__name">{{ industry.name }}</h2>
              <div class="industry-market-drawer__meta">
                <span class="industry-market-drawer__key app-mono">{{ industry.key }}</span>
                <span class="industry-market-drawer__sep">·</span>
                <span class="industry-market-drawer__source">{{ t('industries.market.sourceSystem') }}</span>
              </div>
            </div>
          </div>
          <button
            type="button"
            class="industry-market-drawer__close"
            :aria-label="t('industries.market.drawerClose')"
            @click="close"
          >
            ✕
          </button>
        </header>

        <div class="industry-market-drawer__body">
          <p class="industry-market-drawer__desc">{{ industry.description }}</p>

          <div class="industry-market-drawer__metrics">
            <div class="industry-market-drawer__metric">
              <div class="industry-market-drawer__metric-value app-mono">{{ industry.deptCount ?? 0 }}</div>
              <div class="industry-market-drawer__metric-label">{{ t('industries.market.metricDept') }}</div>
            </div>
            <div class="industry-market-drawer__metric">
              <div class="industry-market-drawer__metric-value app-mono">{{ industry.posCount ?? 0 }}</div>
              <div class="industry-market-drawer__metric-label">{{ t('industries.market.metricPos') }}</div>
            </div>
            <div class="industry-market-drawer__metric">
              <div class="industry-market-drawer__metric-value app-mono">{{ industry.agentCount ?? industry.posCount ?? 0 }}</div>
              <div class="industry-market-drawer__metric-label">{{ t('industries.market.metricAgent') }}</div>
            </div>
          </div>

          <div class="industry-market-drawer__section-title">
            {{ t('industries.market.drawerSectionDepts') }}
          </div>

          <div v-if="loadingDepartments" class="industry-market-drawer__loading">
            {{ t('industries.market.drawerLoadingDepts') }}
          </div>
          <div v-else-if="departments.length === 0" class="industry-market-drawer__empty">
            {{ t('industries.market.drawerNoDepts') }}
          </div>
          <div v-else class="industry-market-drawer__dept-list">
            <div v-for="dep in departments" :key="dep.key" class="industry-market-drawer__dept">
              <div class="industry-market-drawer__dept-head">
                <span class="industry-market-drawer__dept-name">{{ dep.name }}</span>
                <span class="industry-market-drawer__dept-count app-mono">{{ positionsByDept[dep.key]?.length ?? 0 }} {{ t('industries.market.metricPos') }}</span>
              </div>
              <div v-if="(positionsByDept[dep.key]?.length ?? 0) > 0" class="industry-market-drawer__dept-pos">
                <div
                  v-for="pos in positionsByDept[dep.key] ?? []"
                  :key="pos.key"
                  class="industry-market-drawer__pos-row"
                >
                  <span class="industry-market-drawer__pos-dot" />
                  <span class="industry-market-drawer__pos-name">{{ pos.name }}</span>
                  <span v-if="pos.seniority_level" class="industry-market-drawer__pos-seniority app-mono">{{ pos.seniority_level }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <footer class="industry-market-drawer__foot">
          <button type="button" class="app-btn app-btn--ghost" @click="emit('view-prompts', industry)">
            {{ t('industries.market.drawerActionViewPrompts') }}
          </button>
          <button type="button" class="app-btn app-btn--primary" @click="emit('install', industry)">
            {{ t('industries.market.drawerActionInstall') }} →
          </button>
        </footer>
      </aside>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Industry, Department, Position } from '../../features/industries/types';
import type { IndustryDetail } from '../../features/industries/useIndustryMarket';
import { monoBgForKey, monoLettersForKey } from '../../features/industries/industryMonogram';

const props = defineProps<{
  modelValue: boolean;
  industry: Industry | null;
  detail: IndustryDetail;
  detailLoading?: boolean;
}>();
const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  install: [industry: Industry];
  'view-prompts': [industry: Industry];
}>();

const { t } = useI18n();

const departments = computed(() => props.detail.departments);
const positionsByDept = computed(() => props.detail.positionsByDept);
const loadingDepartments = computed(() => props.detailLoading ?? false);

const monoBg = computed(() => {
  if (!props.industry) return monoBgForKey('default');
  return monoBgForKey(props.industry.key);
});

const monoLetters = computed(() => {
  if (!props.industry) return '·';
  return monoLettersForKey(props.industry.key, props.industry.name);
});

function close() {
  emit('update:modelValue', false);
}
</script>

<style lang="sass" scoped>
.industry-market-drawer-mask
  position: fixed
  inset: 0
  background: rgba(44, 34, 24, 0.5)
  backdrop-filter: blur(3px)
  -webkit-backdrop-filter: blur(3px)
  z-index: 2000

.industry-market-drawer
  position: fixed
  top: 0
  right: 0
  bottom: 0
  width: 480px
  max-width: 100%
  background: var(--canvas-base, #FEFBF4)
  border-left: 1px solid var(--glass-border, rgba(235, 220, 200, 0.7))
  box-shadow: -20px 0 60px rgba(93, 64, 55, 0.15)
  z-index: 2001
  display: flex
  flex-direction: column
  outline: none

.industry-market-drawer__head
  display: flex
  align-items: flex-start
  justify-content: space-between
  gap: 12px
  padding: 24px 28px 18px
  border-bottom: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))

.industry-market-drawer__head-main
  display: flex
  align-items: flex-start
  gap: 12px
  min-width: 0

.industry-market-drawer__mono
  width: 44px
  height: 44px
  border-radius: 10px
  display: grid
  place-items: center
  font-weight: 700
  font-size: 16px
  color: #fff
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.3), 0 2px 6px rgba(0, 0, 0, 0.06)
  flex-shrink: 0

.industry-market-drawer__title
  min-width: 0
  flex: 1

.industry-market-drawer__name
  margin: 0
  font-size: 18px
  font-weight: 700
  letter-spacing: -0.02em
  color: var(--color-text-primary, #2C2218)

.industry-market-drawer__meta
  display: flex
  gap: 6px
  align-items: center
  font-size: 11.5px
  color: var(--color-text-tertiary, #5A6A7E)
  margin-top: 2px

.industry-market-drawer__sep
  opacity: 0.6

.industry-market-drawer__close
  width: 28px
  height: 28px
  border-radius: 8px
  background: transparent
  border: 0
  cursor: pointer
  display: grid
  place-items: center
  color: var(--color-text-tertiary, #5A6A7E)
  font-size: 14px
  transition: background 0.12s, color 0.12s

  &:hover
    background: var(--interaction-surface-hover, #FDF6E8)
    color: var(--color-text-primary, #2C2218)

.industry-market-drawer__body
  flex: 1
  overflow-y: auto
  padding: 20px 28px 28px

.industry-market-drawer__desc
  margin: 0 0 12px
  color: var(--color-text-secondary, #6B5B4D)
  font-size: 13px
  line-height: 1.6

.industry-market-drawer__metrics
  display: grid
  grid-template-columns: repeat(3, 1fr)
  gap: 8px
  margin-bottom: 8px

.industry-market-drawer__metric
  border: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
  border-radius: 10px
  padding: 10px 12px
  background: var(--color-surface-solid, #FFFFFF)

.industry-market-drawer__metric-value
  font-size: 20px
  font-weight: 600
  line-height: 1.1
  color: var(--color-text-primary, #2C2218)

.industry-market-drawer__metric-label
  font-size: 10.5px
  color: var(--color-text-tertiary, #5A6A7E)
  text-transform: uppercase
  letter-spacing: 0.04em
  margin-top: 2px

.industry-market-drawer__section-title
  font-size: 11px
  font-weight: 700
  letter-spacing: 0.08em
  text-transform: uppercase
  color: var(--color-text-tertiary, #5A6A7E)
  margin: 18px 0 10px

.industry-market-drawer__loading, .industry-market-drawer__empty
  padding: 16px
  border: 1px dashed var(--color-border-soft, rgba(141, 110, 99, 0.12))
  border-radius: 10px
  text-align: center
  color: var(--color-text-tertiary, #5A6A7E)
  font-size: 12.5px

.industry-market-drawer__dept-list
  display: flex
  flex-direction: column
  gap: 8px

.industry-market-drawer__dept
  border: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
  border-radius: 12px
  background: var(--color-surface-solid, #FFFFFF)
  padding: 12px 14px

.industry-market-drawer__dept-head
  display: flex
  align-items: baseline
  justify-content: space-between
  gap: 8px

.industry-market-drawer__dept-name
  font-weight: 600
  font-size: 13.5px
  color: var(--color-text-primary, #2C2218)

.industry-market-drawer__dept-count
  font-size: 11px
  color: var(--color-text-tertiary, #5A6A7E)

.industry-market-drawer__dept-pos
  margin-top: 10px
  display: flex
  flex-direction: column
  gap: 4px

.industry-market-drawer__pos-row
  display: flex
  align-items: center
  gap: 8px
  padding: 6px 8px
  border-radius: 6px
  font-size: 12.5px
  color: var(--color-text-secondary, #6B5B4D)

.industry-market-drawer__pos-dot
  width: 4px
  height: 4px
  border-radius: 50%
  background: var(--color-icon-muted, #A89580)
  flex-shrink: 0

.industry-market-drawer__pos-name
  flex: 1
  min-width: 0

.industry-market-drawer__pos-seniority
  font-size: 10.5px
  color: var(--color-text-tertiary, #5A6A7E)
  padding: 1px 6px
  border-radius: 4px
  background: var(--canvas-base, #FEFBF4)
  border: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))

.industry-market-drawer__foot
  padding: 16px 28px
  border-top: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
  display: flex
  gap: 8px
  background: var(--canvas-base, #FEFBF4)

.app-btn
  flex: 1
  display: inline-flex
  align-items: center
  justify-content: center
  gap: 6px
  padding: 10px 14px
  border-radius: 10px
  font-size: 13px
  font-weight: 500
  cursor: pointer
  transition: background 0.12s, color 0.12s, border-color 0.12s

.app-btn--ghost
  background: var(--color-surface-solid, #FFFFFF)
  border: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
  color: var(--color-text-primary, #2C2218)

  &:hover
    background: var(--interaction-surface-hover, #FDF6E8)
    border-color: var(--glass-border, rgba(235, 220, 200, 0.7))

.app-btn--primary
  background: var(--color-accent, #DCA03E)
  color: #fff
  border: 1px solid var(--color-accent, #DCA03E)
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.25), 0 1px 2px rgba(220, 160, 62, 0.2)

  &:hover
    background: var(--color-accent-hover, #C48C28)
    border-color: var(--color-accent-hover, #C48C28)

.app-mono
  font-family: 'JetBrains Mono', 'SF Mono', Menlo, monospace
  font-feature-settings: 'tnum' 1

/* Transitions */
.industry-drawer-fade-enter-active, .industry-drawer-fade-leave-active
  transition: opacity 0.18s ease

.industry-drawer-fade-enter-from, .industry-drawer-fade-leave-to
  opacity: 0

.industry-drawer-slide-enter-active, .industry-drawer-slide-leave-active
  transition: transform 0.24s cubic-bezier(0.2, 0.8, 0.2, 1), opacity 0.18s

.industry-drawer-slide-enter-from, .industry-drawer-slide-leave-to
  transform: translateX(20px)
  opacity: 0
</style>
