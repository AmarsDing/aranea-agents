<!-- web/src/components/chat/v2/TurnContainer.vue -->
<template>
  <div class="turn-container" :data-turn-id="turn.ID">
    <template v-for="step in visibleSteps" :key="step.ID">
      <ThinkingBlock v-if="step.Kind === 'thinking'" :step="step" />
      <ActionBlock v-else-if="step.Kind === 'action'" :step="step" />
      <ReplyBlock v-else-if="step.Kind === 'reply'" :step="step" />
      <NoticeBlock v-else-if="step.Kind === 'notice'" :step="step" />
      <ConfirmBlock v-else-if="step.Kind === 'confirm'" :step="step" />
      <ErrorBlock v-else-if="step.Kind === 'error'" :step="step" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Turn } from '../../../features/chat/v2Types';
import { isSystemInternalNotice } from '../../../features/chat/noticeFilter';
import ThinkingBlock from '../ThinkingBlock.vue';
import ActionBlock from '../ActionBlock.vue';
import ReplyBlock from '../ReplyBlock.vue';
import NoticeBlock from '../NoticeBlock.vue';
import ConfirmBlock from '../ConfirmBlock.vue';
import ErrorBlock from '../ErrorBlock.vue';

const props = defineProps<{ turn: Turn }>();
const store = useChatActivityStore();
const visibleSteps = computed(() =>
  store.getTurnSteps(props.turn.ID).filter((s) => !isSystemInternalNotice(s.Kind, s.NoticeType)),
);
</script>
