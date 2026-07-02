<!-- web/src/components/chat/v2/TurnContainer.vue -->
<template>
  <div class="turn-container" :data-turn-id="turn.ID">
    <template v-for="step in steps" :key="step.ID">
      <ThinkingBlock v-if="step.Kind === 'thinking'" :step="step" />
      <ActionBlock v-else-if="step.Kind === 'action'" :step="step" />
      <ReplyBlock v-else-if="step.Kind === 'reply'" :step="step" />
      <NoticeBlock v-else-if="step.Kind === 'notice'" :activity="step as any" />
      <ConfirmBlock v-else-if="step.Kind === 'confirm'" :activity="step as any" />
      <ErrorBlock v-else-if="step.Kind === 'error'" :event="step as any" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Turn } from '../../../features/chat/v2Types';
import ThinkingBlock from '../ThinkingBlock.vue';
import ActionBlock from '../ActionBlock.vue';
import ReplyBlock from '../ReplyBlock.vue';
import NoticeBlock from '../NoticeBlock.vue';
import ConfirmBlock from '../ConfirmBlock.vue';
import ErrorBlock from '../ErrorBlock.vue';

const props = defineProps<{ turn: Turn }>();
const store = useChatActivityStore();
const steps = computed(() => store.getTurnSteps(props.turn.ID));
</script>
