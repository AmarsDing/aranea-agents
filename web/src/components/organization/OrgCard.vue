<template>
  <article
    :class="['org-market-card', { 'is-open': isOpen }]"
    role="button"
    tabindex="0"
    :aria-pressed="isOpen"
    @click="emit('select', company)"
    @keydown.enter.prevent="emit('select', company)"
    @keydown.space.prevent="emit('select', company)"
  >
    <header class="org-market-card__head">
      <div class="org-market-card__mono" :style="{ background: monoBg }" :aria-label="`公司 ${company.name}`">
        <span>{{ monoLetters }}</span>
      </div>
      <div class="org-market-card__title">
        <h3 class="org-market-card__name">{{ company.name }}</h3>
        <div class="org-market-card__key">
          <span class="org-market-card__key-prefix">key</span>
          <span class="app-mono">{{ company.key }}</span>
        </div>
      </div>
      <span :class="['org-market-card__status', company.enabled ? 'is-on' : 'is-off']">
        <span class="org-market-card__status-dot" />
        {{ company.enabled ? t('organization.market.statusEnabled') : t('organization.market.statusDisabled') }}
      </span>
    </header>

    <p class="org-market-card__desc">{{ company.description }}</p>

    <hr class="org-market-card__divider" />

    <div class="org-market-card__metrics">
      <div class="org-market-card__metric">
        <span class="org-market-card__metric-value app-mono">{{ company.deptCount ?? 0 }}</span>
        <span class="org-market-card__metric-label">{{ t('organization.market.metricDept') }}</span>
      </div>
      <div class="org-market-card__metric">
        <span class="org-market-card__metric-value app-mono">{{ company.posCount ?? 0 }}</span>
        <span class="org-market-card__metric-label">{{ t('organization.market.metricPos') }}</span>
      </div>
      <div class="org-market-card__metric">
        <span class="org-market-card__metric-value app-mono">{{
          company.agentCount ?? company.posCount ?? 0
        }}</span>
        <span class="org-market-card__metric-label">{{ t('organization.market.metricAgent') }}</span>
      </div>
      <div class="org-market-card__metric">
        <span class="org-market-card__metric-value app-mono">{{ company.installed ?? 0 }}</span>
        <span class="org-market-card__metric-label">{{ t('organization.market.metricInstalled') }}</span>
      </div>
    </div>

    <footer class="org-market-card__foot">
      <button
        type="button"
        class="org-market-card__action org-market-card__action--primary"
        @click.stop="emit('select', company)"
      >
        {{ t('organization.market.cardViewDepts') }} →
      </button>
      <span class="org-market-card__foot-spacer" />
      <span class="org-market-card__source">{{ t('organization.market.sourceSystem') }}</span>
    </footer>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Company } from '../../features/organization/types';
import { monoBgForKey, monoLettersForKey } from '../../features/organization/orgMonogram';

const props = defineProps<{ company: Company; isOpen?: boolean }>();
const emit = defineEmits<{ select: [company: Company] }>();
const { t } = useI18n();

const monoBg = computed(() => monoBgForKey(props.company.key));
const monoLetters = computed(() => monoLettersForKey(props.company.key, props.company.name));
</script>

<style lang="sass" scoped>
.org-market-card
  position: relative
  display: flex
  flex-direction: column
  padding: 18px 20px 16px
  border: 1px solid var(--glass-border, rgba(235, 220, 200, 0.7))
  border-radius: 18px
  background: var(--glass-surface, rgba(255, 253, 245, 0.65))
  backdrop-filter: blur(18px)
  -webkit-backdrop-filter: blur(18px)
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.45)
  cursor: pointer
  transition: transform 0.18s ease, border-color 0.18s, box-shadow 0.18s
  outline: none

  &:hover, &:focus-visible
    transform: translateY(-2px)
    border-color: rgba(220, 160, 62, 0.45)
    box-shadow: 0 8px 24px rgba(93, 64, 55, 0.08), inset 0 1px 0 rgba(255, 255, 255, 0.45)

  &.is-open
    border-color: var(--color-accent, #DCA03E)
    box-shadow: 0 0 0 2px rgba(220, 160, 62, 0.18), 0 8px 24px rgba(93, 64, 55, 0.08), inset 0 1px 0 rgba(255, 255, 255, 0.45)

.org-market-card__head
  display: flex
  align-items: flex-start
  gap: 12px
  margin-bottom: 12px

.org-market-card__mono
  width: 40px
  height: 40px
  flex-shrink: 0
  border-radius: 10px
  display: grid
  place-items: center
  font-weight: 700
  font-size: 14px
  letter-spacing: 0.02em
  color: var(--color-on-accent, #fff)
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.3), 0 2px 6px rgba(0, 0, 0, 0.06)
  user-select: none

.org-market-card__title
  flex: 1
  min-width: 0

.org-market-card__name
  margin: 0
  font-size: 15px
  font-weight: 600
  letter-spacing: -0.01em
  color: var(--color-text-primary, #2C2218)

.org-market-card__key
  font-size: 11.5px
  color: var(--color-text-tertiary, #5A6A7E)
  margin-top: 2px
  display: flex
  gap: 4px

.org-market-card__key-prefix
  color: var(--color-text-tertiary, #5A6A7E)

.org-market-card__status
  flex-shrink: 0
  display: flex
  align-items: center
  gap: 4px
  font-size: 11.5px
  padding: 3px 8px
  border-radius: 999px
  white-space: nowrap

  &.is-on
    background: var(--color-success-soft, #ECFDF3)
    color: var(--color-accent-green, #2D6A4F)

  &.is-off
    background: rgba(229, 92, 92, 0.1)
    color: var(--color-danger, #B13939)

.org-market-card__status-dot
  width: 6px
  height: 6px
  border-radius: 50%
  background: currentColor

.org-market-card__desc
  font-size: 12.5px
  color: var(--color-text-secondary, #6B5B4D)
  margin: 0 0 14px
  min-height: 36px
  line-height: 1.5
  display: -webkit-box
  -webkit-line-clamp: 2
  -webkit-box-orient: vertical
  overflow: hidden

.org-market-card__divider
  border: 0
  border-top: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
  margin: 12px 0

.org-market-card__metrics
  display: grid
  grid-template-columns: repeat(4, 1fr)
  gap: 4px

.org-market-card__metric
  display: flex
  flex-direction: column
  gap: 2px

.org-market-card__metric-value
  font-size: 18px
  font-weight: 600
  letter-spacing: -0.01em
  line-height: 1.1
  color: var(--color-text-primary, #2C2218)

.org-market-card__metric-label
  font-size: 10.5px
  color: var(--color-text-tertiary, #5A6A7E)
  letter-spacing: 0.02em

.org-market-card__foot
  display: flex
  align-items: center
  gap: 6px
  margin-top: 14px

.org-market-card__action
  display: inline-flex
  align-items: center
  gap: 4px
  padding: 5px 10px
  border-radius: 8px
  font-size: 12px
  font-weight: 500
  background: transparent
  border: 0
  cursor: pointer
  color: var(--color-text-secondary, #6B5B4D)
  transition: background 0.12s, color 0.12s

  &:hover
    background: var(--interaction-surface-hover, #FDF6E8)
    color: var(--color-text-primary, #2C2218)

  &--primary
    color: var(--color-warning, #8a6014)

    &:hover
      background: rgba(220, 160, 62, 0.12)

.org-market-card__foot-spacer
  flex: 1

.org-market-card__source
  font-size: 10.5px
  color: var(--color-text-tertiary, #5A6A7E)
  letter-spacing: 0.04em
  text-transform: uppercase

.app-mono
  font-family: 'JetBrains Mono', 'SF Mono', Menlo, monospace
  font-feature-settings: 'tnum' 1
</style>
