<template>
  <q-badge
    :class="{ 'agent-status-label--animated': config.animated }"
    class="agent-status-label"
    :style="{ color: config.color, borderColor: config.color }"
    outline
  >
    <template #default>
      <q-icon :name="config.icon" size="12px" class="q-mr-xs" :style="{ color: config.color }" />
      <span>{{ config.text }}</span>
    </template>
  </q-badge>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { AgentNodeStatusLabel } from '../../features/spirit/spiritUi';
import { STATUS_LABEL_CONFIG } from '../../features/spirit/spiritUi';

const props = defineProps<{
  label: AgentNodeStatusLabel;
}>();

const config = computed(() => STATUS_LABEL_CONFIG[props.label] ?? STATUS_LABEL_CONFIG.queued);
</script>

<style lang="sass" scoped>
.agent-status-label
  min-width: 72px
  max-width: 88px
  justify-content: center
  font-size: var(--text-xs)
  padding: 2px 6px
  border-radius: 4px

.agent-status-label--animated
  animation: agent-status-breathe 2s ease-in-out infinite

@keyframes agent-status-breathe
  0%, 100%
    opacity: 1
  50%
    opacity: 0.6
</style>
