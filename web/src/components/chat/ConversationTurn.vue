<template>
  <div class="conversation-turn">
    <UserMessageBubble :message="turn.userMessage" />
    <AgentWorkPanel :agent-work="turn.agentWork" />
  </div>
</template>

<script setup lang="ts">
import type { ConversationTurn } from '../../features/chat/activityTimelineTypes';
import UserMessageBubble from './UserMessageBubble.vue';
import AgentWorkPanel from './AgentWorkPanel.vue';

defineProps<{
  turn: ConversationTurn;
}>();
</script>

<style lang="sass" scoped>
.conversation-turn
  margin-bottom: 16px
</style>
