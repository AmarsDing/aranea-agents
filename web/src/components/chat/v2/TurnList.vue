<!-- web/src/components/chat/v2/TurnList.vue -->
<template>
  <div class="turn-list">
    <TurnContainer
      v-for="turn in turns"
      :key="turn.ID"
      :turn="turn"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
    />
  </div>
</template>

<script setup lang="ts">
import type { Turn } from '../../../features/chat/v2Types';
import TurnContainer from './TurnContainer.vue';

defineProps<{ turns: Turn[] }>();
defineEmits<{
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
}>();
</script>
