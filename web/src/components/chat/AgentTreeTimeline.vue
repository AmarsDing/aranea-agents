<template>
  <div class="agent-tree-timeline">
    <!-- Global expand/collapse controls -->
    <div v-if="agentBlocks.length > 0" class="agent-tree-timeline__toolbar">
      <button class="toolbar-btn" @click="expandAll">{{ t('chat.expandAll') }}</button>
      <button class="toolbar-btn" @click="collapseAll">{{ t('chat.collapseAll') }}</button>
    </div>

    <div v-for="block in agentBlocks" :key="block.id" class="agent-tree-timeline__turn">
      <!-- User message bubble (right-aligned) -->
      <div v-if="block.task" class="user-bubble">
        <div class="user-bubble__content">{{ block.task }}</div>
      </div>

      <!-- Root Agent Block -->
      <AgentBlock
        :block="block"
        :is-root="true"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { AgentBlock as AgentBlockType } from '../../features/chat/agentTreeTypes';
import AgentBlock from './AgentBlock.vue';

const { t } = useI18n();

defineProps<{
  agentBlocks: AgentBlockType[];
}>();

function expandAll() {
  // Dispatch custom event that AgentBlock components can listen to
  window.dispatchEvent(new CustomEvent('agent-tree-expand-all'));
}

function collapseAll() {
  window.dispatchEvent(new CustomEvent('agent-tree-collapse-all'));
}
</script>

<style scoped lang="sass">
.agent-tree-timeline
  padding: 0 4px

.agent-tree-timeline__toolbar
  display: flex
  gap: 8px
  justify-content: flex-end
  margin-bottom: 12px

.toolbar-btn
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  color: var(--color-text-secondary)
  padding: 4px 10px
  border-radius: 6px
  font-size: 12px
  cursor: pointer
  transition: all 0.15s

  &:hover
    background: var(--glass-surface-hover)
    border-color: var(--glass-border-hover)
    color: var(--color-text-primary)

.agent-tree-timeline__turn
  margin-bottom: 20px

.user-bubble
  display: flex
  justify-content: flex-end
  margin-bottom: 16px

.user-bubble__content
  background: color-mix(in srgb, var(--color-accent) 14%, transparent)
  border: 1px solid color-mix(in srgb, var(--color-accent) 38%, transparent)
  color: var(--color-text-primary)
  padding: 10px 16px
  border-radius: 16px 16px 4px 16px
  max-width: 70%
  font-size: 14px
  line-height: 1.55
</style>
