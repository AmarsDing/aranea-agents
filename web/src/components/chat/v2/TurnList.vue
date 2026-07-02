<!-- web/src/components/chat/v2/TurnList.vue -->
<template>
  <div class="turn-list">
    <TurnContainer v-for="turn in turns" :key="turn.ID" :turn="turn" />
  </div>
</template>

<script setup lang="ts">
import type { Turn } from '../../../features/chat/v2Types';
import TurnContainer from './TurnContainer.vue';

defineProps<{ turns: Turn[] }>();
</script>
