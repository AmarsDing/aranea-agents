<template>
  <div class="conversation-turn">
    <UserMessageBubble v-if="turn.userMessage" :message="turn.userMessage" />
    <AgentWorkPanel
      :agent-work="turn.agentWork"
      @confirm="(id, approved) => $emit('confirm', id, approved)"
      @error-retry="(e) => $emit('error-retry', e)"
      @error-switch-model="(e) => $emit('error-switch-model', e)"
      @error-rephrase="(e) => $emit('error-rephrase', e)"
      @error-check-config="(e) => $emit('error-check-config', e)"
      @error-remove-attachment="(e) => $emit('error-remove-attachment', e)"
      @error-relogin="(e) => $emit('error-relogin', e)"
    />
  </div>
</template>

<script setup lang="ts">
import type { ConversationTurn } from '../../features/chat/activityTimelineTypes';
import type { ErrorEvent } from '../../features/chat/streamEventTypes';
import UserMessageBubble from './UserMessageBubble.vue';
import AgentWorkPanel from './AgentWorkPanel.vue';

defineProps<{
  turn: ConversationTurn;
}>();

defineEmits<{
  confirm: [activityId: string, approved: boolean];
  'error-retry': [event: ErrorEvent];
  'error-switch-model': [event: ErrorEvent];
  'error-rephrase': [event: ErrorEvent];
  'error-check-config': [event: ErrorEvent];
  'error-remove-attachment': [event: ErrorEvent];
  'error-relogin': [event: ErrorEvent];
}>();
</script>

<style lang="sass" scoped>
.conversation-turn
  margin-bottom: 16px
</style>
