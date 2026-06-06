<template>
  <tr class="industry-market-table-row" @click="emit('select', industry)">
    <td>
      <div class="industry-market-table-row__primary">
        <div class="industry-market-table-row__mono" :style="{ background: monoBg }">
          {{ monoLetters }}
        </div>
        <div>
          <div class="industry-market-table-row__name">{{ industry.name }}</div>
          <div class="industry-market-table-row__key app-mono">{{ industry.key }}</div>
        </div>
      </div>
    </td>
    <td class="industry-market-table-row__desc">{{ industry.description }}</td>
    <td class="num app-mono">{{ industry.deptCount ?? 0 }}</td>
    <td class="num app-mono">{{ industry.posCount ?? 0 }}</td>
    <td class="num app-mono">{{ industry.agentCount ?? industry.posCount ?? 0 }}</td>
    <td class="num app-mono">{{ industry.installed ?? 0 }}</td>
    <td>
      <span :class="['industry-market-table-row__status', industry.enabled ? 'is-on' : 'is-off']">
        <span class="industry-market-table-row__status-dot" />
        {{ industry.enabled ? t('industries.market.statusEnabled') : t('industries.market.statusDisabled') }}
      </span>
    </td>
    <td class="industry-market-table-row__source">{{ t('industries.market.sourceSystem') }}</td>
  </tr>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Industry } from '../../features/industries/types';
import { monoBgForKey, monoLettersForKey } from '../../features/industries/industryMonogram';

const props = defineProps<{ industry: Industry }>();
const emit = defineEmits<{ select: [industry: Industry] }>();
const { t } = useI18n();

const monoBg = computed(() => monoBgForKey(props.industry.key));
const monoLetters = computed(() => monoLettersForKey(props.industry.key, props.industry.name));
</script>

<style lang="sass" scoped>
.industry-market-table-row
  transition: background 0.1s
  cursor: pointer

  &:hover
    background: var(--interaction-surface-hover, #FDF6E8)

  td
    padding: 14px 16px
    font-size: 13px
    border-bottom: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))
    vertical-align: middle

  &:last-child td
    border-bottom: 0

  .num
    text-align: right
    font-variant-numeric: tabular-nums

.industry-market-table-row__primary
  display: flex
  align-items: center
  gap: 10px

.industry-market-table-row__mono
  width: 32px
  height: 32px
  border-radius: 8px
  display: grid
  place-items: center
  font-weight: 700
  font-size: 12px
  color: var(--color-on-accent, #fff)
  flex-shrink: 0

.industry-market-table-row__name
  font-weight: 600

.industry-market-table-row__key
  font-size: 11px
  color: var(--color-text-tertiary, #5A6A7E)

.industry-market-table-row__desc
  color: var(--color-text-secondary, #6B5B4D)
  max-width: 28ch
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.industry-market-table-row__status
  display: inline-flex
  align-items: center
  gap: 4px
  font-size: 11.5px
  padding: 3px 8px
  border-radius: 999px

  &.is-on
    background: var(--color-success-soft, #ECFDF3)
    color: var(--color-accent-green, #2D6A4F)

  &.is-off
    background: rgba(229, 92, 92, 0.1)
    color: var(--color-danger, #B13939)

.industry-market-table-row__status-dot
  width: 6px
  height: 6px
  border-radius: 50%
  background: currentColor

.industry-market-table-row__source
  color: var(--color-text-tertiary, #5A6A7E)
  font-size: 11.5px
  letter-spacing: 0.04em
  text-transform: uppercase

.app-mono
  font-family: 'JetBrains Mono', 'SF Mono', Menlo, monospace
  font-feature-settings: 'tnum' 1
</style>
