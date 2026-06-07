<template>
  <tr class="org-table-row" @click="emit('select', company)">
    <td>
      <div class="org-table-row__name-cell">
        <div class="org-table-row__mono" :style="{ background: monoBg }">
          <span>{{ monoLetters }}</span>
        </div>
        <span>{{ company.name }}</span>
      </div>
    </td>
    <td>{{ company.description }}</td>
    <td class="num app-mono">{{ company.deptCount ?? 0 }}</td>
    <td class="num app-mono">{{ company.posCount ?? 0 }}</td>
    <td class="num app-mono">{{ company.agentCount ?? company.posCount ?? 0 }}</td>
    <td class="num app-mono">{{ company.installed ?? 0 }}</td>
    <td>
      <span :class="['org-table-row__status', company.enabled ? 'is-on' : 'is-off']">
        {{ company.enabled ? '启用' : '停用' }}
      </span>
    </td>
    <td>系统</td>
  </tr>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Company } from '../../features/organization/types';
import { monoBgForKey, monoLettersForKey } from '../../features/organization/orgMonogram';

const props = defineProps<{ company: Company }>();
const emit = defineEmits<{ select: [company: Company] }>();

const monoBg = computed(() => monoBgForKey(props.company.key));
const monoLetters = computed(() => monoLettersForKey(props.company.key, props.company.name));
</script>

<style lang="sass" scoped>
.org-table-row
  cursor: pointer
  transition: background 0.12s

  &:hover
    background: var(--interaction-surface-hover, #FDF6E8)

  td
    padding: 12px 16px
    font-size: 13px
    color: var(--color-text-primary, #2C2218)
    border-bottom: 1px solid var(--color-border-soft, rgba(141, 110, 99, 0.12))

  .num
    text-align: right
    font-variant-numeric: tabular-nums

.org-table-row__name-cell
  display: flex
  align-items: center
  gap: 8px

.org-table-row__mono
  width: 28px
  height: 28px
  border-radius: 6px
  display: grid
  place-items: center
  font-weight: 700
  font-size: 10px
  color: var(--color-on-accent, #fff)
  flex-shrink: 0

.org-table-row__status
  font-size: 11.5px
  padding: 2px 8px
  border-radius: 999px

  &.is-on
    background: var(--color-success-soft, #ECFDF3)
    color: var(--color-accent-green, #2D6A4F)

  &.is-off
    background: var(--color-danger-surface)
    color: var(--color-danger, #B13939)

.app-mono
  font-family: 'JetBrains Mono', 'SF Mono', Menlo, monospace
  font-feature-settings: 'tnum' 1
</style>
