<!-- web/src/components/chat/v2/PlanStepDetailPanel.vue -->
<template>
  <div v-if="step" class="plan-step-detail">
    <div class="detail-header">
      <span class="detail-label">{{ step.Label }}</span>
      <q-badge :color="statusColor">{{ step.Status }}</q-badge>
    </div>
    <p v-if="step.Description">{{ step.Description }}</p>
    <div v-if="step.Result" class="detail-result">
      <h4>结果</h4>
      <pre>{{ step.Result.Output }}</pre>
    </div>
    <div v-if="step.Error" class="detail-error">
      <h4>错误</h4>
      <p>{{ step.Error.Message }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { PlanStep } from '../../../features/chat/v2Types';

const props = defineProps<{ step: PlanStep | null }>();
const statusColor = computed(() => ({
  pending: 'grey', running: 'blue', completed: 'green',
  failed: 'red', skipped: 'orange', partial_failure: 'red-7',
}[props.step?.Status || 'pending'] || 'grey'));
</script>
