<!-- web/src/components/chat/v2/PlanBoardCard.vue -->
<template>
  <div class="plan-board-card">
    <div class="plan-header">
      <span>执行计划</span>
      <q-badge :color="statusColor">{{ planBoard.Status }}</q-badge>
    </div>
    <PlanDAG :steps="planBoard.Steps" />
    <PlanStepDetailPanel :step="selectedStep" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import type { PlanBoard } from '../../../features/chat/v2Types';
import PlanDAG from './PlanDAG.vue';
import PlanStepDetailPanel from './PlanStepDetailPanel.vue';

const props = defineProps<{ planBoard: PlanBoard }>();
const selectedStepId = ref<string | null>(null);
const selectedStep = computed(() =>
  props.planBoard.Steps.find(s => s.ID === selectedStepId.value) || null
);
const statusColor = computed(() => ({
  planning: 'orange', executing: 'blue', completed: 'green',
  failed: 'red', partial_failure: 'orange-8',
}[props.planBoard.Status] || 'grey'));
</script>
