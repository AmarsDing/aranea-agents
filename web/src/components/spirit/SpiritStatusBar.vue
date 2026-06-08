<template>
  <div v-if="visible" class="spirit-status-bar">
    <div class="row items-center no-wrap q-gutter-sm spirit-status-bar__inner">
      <div v-if="runningTeamCount > 0" class="spirit-status-bar__item">
        <q-icon name="bolt" size="14px" :style="{ color: 'var(--color-accent)' }" />
        <span>{{ runningTeamCount }} 运行中</span>
      </div>
      <div v-if="interruptedTeamCount > 0" class="spirit-status-bar__item">
        <q-icon name="pause_circle" size="14px" :style="{ color: 'var(--color-warning)' }" />
        <span>{{ interruptedTeamCount }} 已中断</span>
      </div>
      <div v-if="quotaMax > 0" class="spirit-status-bar__item spirit-status-bar__item--hide-sm">
        <q-icon name="bar_chart" size="14px" :style="{ color: 'var(--color-text-tertiary)' }" />
        <span>{{ quotaUsed }}/{{ quotaMax }} 配额</span>
      </div>
      <div v-if="tokenUsage" class="spirit-status-bar__item spirit-status-bar__item--hide-sm">
        <q-icon name="data_usage" size="14px" :style="{ color: 'var(--color-text-tertiary)' }" />
        <span>{{ tokenLabel }}</span>
      </div>
      <div v-if="lastEvent" class="spirit-status-bar__item spirit-status-bar__last-event">
        <q-icon
          :name="lastEvent.type === 'completed' ? 'check_circle' : 'error'"
          :style="{ color: lastEvent.type === 'completed' ? 'var(--color-success)' : 'var(--color-danger)' }"
          size="14px"
        />
        <span class="ellipsis">{{ lastEvent.teamName }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  runningTeamCount: number;
  interruptedTeamCount: number;
  quotaUsed: number;
  quotaMax: number;
  tokenUsage?: { in: number; out: number } | null;
  lastEvent?: { type: 'completed' | 'failed'; teamName: string } | null;
}>();

const visible = computed(() => props.runningTeamCount > 0 || props.interruptedTeamCount > 0 || props.quotaMax > 0 || !!props.lastEvent);

const tokenLabel = computed(() => {
  if (!props.tokenUsage) return '';
  const total = (props.tokenUsage.in + props.tokenUsage.out) / 1000;
  return `${total.toFixed(1)}k Token`;
});
</script>

<style scoped lang="sass">
.spirit-status-bar
  height: 24px
  flex-shrink: 0
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.spirit-status-bar__inner
  height: 24px
  padding: 0 var(--space-3)
  font-size: 11px
  color: var(--color-text-secondary)
  overflow: hidden

.spirit-status-bar__item
  display: flex
  align-items: center
  gap: 3px
  white-space: nowrap
  flex-shrink: 0

.spirit-status-bar__last-event
  margin-left: auto
  max-width: 160px

.spirit-status-bar__item--hide-sm
  @media (max-width: 600px)
    display: none
</style>
