<template>
  <div v-if="status !== 'normal'" class="context-indicator" :class="statusClass">
    <q-icon :name="statusIcon" size="16px" class="q-mr-xs" />
    <span class="text-caption">{{ statusLabel }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { CompressStatus } from '../../features/session/types'

const props = defineProps<{
  status: CompressStatus
}>()

const statusClass = computed(() => `context-indicator--${props.status}`)

const statusIcon = computed(() => {
  switch (props.status) {
    case 'optimizing': return 'schedule'
    case 'compressing': return 'compress'
    case 'optimized': return 'check_circle'
    default: return 'info'
  }
})

const statusLabel = computed(() => {
  switch (props.status) {
    case 'optimizing': return '正在优化上下文...'
    case 'compressing': return '正在压缩上下文...'
    case 'optimized': return '上下文已优化'
    default: return ''
  }
})
</script>

<style scoped lang="scss">
.context-indicator {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 4px;
  margin: 4px 0;

  backdrop-filter: blur(var(--glass-blur-default, 8px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 8px));

  &--optimizing {
    background: color-mix(in srgb, var(--color-warning) 10%, transparent);
    color: var(--color-warning);
  }

  &--compressing {
    background: color-mix(in srgb, var(--color-danger) 10%, transparent);
    color: var(--color-danger);
  }

  &--optimized {
    background: color-mix(in srgb, var(--color-info) 10%, transparent);
    color: var(--color-info);
  }
}
</style>
