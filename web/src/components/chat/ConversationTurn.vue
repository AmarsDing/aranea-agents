<template>
  <div class="conversation-turn">
    <UserMessageBubble v-if="turn.userMessage" :message="turn.userMessage" />
    <AgentWorkPanel :agent-work="turn.agentWork" @confirm="(id, approved) => $emit('confirm', id, approved)" />
  </div>
</template>

<script setup lang="ts">
import type { ConversationTurn } from '../../features/chat/activityTimelineTypes';
import UserMessageBubble from './UserMessageBubble.vue';
import AgentWorkPanel from './AgentWorkPanel.vue';

defineProps<{
  turn: ConversationTurn;
}>();

defineEmits<{
  confirm: [activityId: string, approved: boolean];
}>();
</script>

<style lang="sass" scoped>
.conversation-turn
  margin-bottom: 16px
</style>
