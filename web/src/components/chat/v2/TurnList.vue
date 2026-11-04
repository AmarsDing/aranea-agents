<!-- web/src/components/chat/v2/TurnList.vue -->
<template>
  <div class="turn-list">
    <TurnContainer
      v-for="turn in turns"
      :key="turn.ID"
      :turn="turn"
      @confirm-step="(p) => $emit('confirm-step', p)"
      @fork-turn="(t) => $emit('fork-turn', t)"
      @error-retry="(s) => $emit('error-retry', s)"
      @error-switch-model="(s) => $emit('error-switch-model', s)"
    />
  </div>
</template>

<script setup lang="ts">
import type { Step, Turn } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload } from '../../../features/chat/types';
import TurnContainer from './TurnContainer.vue';

defineProps<{ turns: Turn[] }>();
defineEmits<{
  'confirm-step': [payload: ConfirmStepPayload];
  'fork-turn': [turn: Turn];
  'error-retry': [step: Step];
  'error-switch-model': [step: Step];
}>();
</script>
