<template>
  <div v-if="steps.length" class="chat-react-steps q-mb-sm">
    <div v-for="(step, idx) in steps" :key="`${step.kind}-${idx}`" class="chat-react-step">
      <div class="chat-react-step__head">
        <q-icon :name="iconFor(step.kind)" size="18px" class="q-mr-xs" />
        <span class="text-caption text-weight-bold">{{ step.title }}</span>
      </div>
      <div v-if="step.body" class="chat-react-step__body text-body2" v-html="renderBody(step.body)" />
      <div v-if="step.kind === 'action' && step.linkedTools?.length" class="chat-react-step__tools q-mt-sm">
        <ChatExecutionCard v-for="tool in step.linkedTools" :key="tool.id" :event="tool" class="q-mb-xs" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import ChatExecutionCard from './ChatExecutionCard.vue';
import type { ReactStepKind } from '../../features/chat/reactPlannerTypes';
import type { ReactStepWithTools } from '../../features/chat/types';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';

defineProps<{
  steps: ReactStepWithTools[];
  isDark?: boolean;
}>();

function iconFor(kind: ReactStepKind): string {
  switch (kind) {
    case 'planning':
      return 'map';
    case 'reasoning':
      return 'psychology';
    case 'action':
      return 'build';
    case 'replanning':
      return 'refresh';
    default:
      return 'chevron_right';
  }
}

function renderBody(body: string): string {
  return renderChatMarkdown(body.trim());
}
</script>

<style scoped>
.chat-react-steps {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.chat-react-step {
  padding: var(--space-3);
  border-radius: 14px;
  border: 1px solid var(--glass-border);
  background: var(--glass-elevated);
}

.chat-react-step__head {
  display: flex;
  align-items: center;
  color: var(--color-text-secondary);
  margin-bottom: var(--space-1);
}

.chat-react-step__body :deep(p:last-child) {
  margin-bottom: 0;
}

.chat-react-step__tools :deep(.chat-execution-card) {
  border-radius: 10px;
}
</style>
