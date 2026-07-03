<!-- web/src/components/chat/v2/PlanStepDetailPanel.vue -->
<template>
  <div v-if="step" class="plan-step-detail">
    <div class="detail-header">
      <span class="detail-label">{{ step.Label }}</span>
      <q-badge :color="statusColor">{{ step.Status }}</q-badge>
    </div>
    <p v-if="step.Description">{{ step.Description }}</p>
    <div v-if="step.Result" class="detail-result">
      <h4>{{ t('chat.v2.resultLabel') }}</h4>
      <pre>{{ step.Result.Output }}</pre>
    </div>
    <div v-if="step.Error" class="detail-error">
      <h4>{{ t('chat.v2.errorLabel') }}</h4>
      <p>{{ step.Error.Message }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PlanStep } from '../../../features/chat/v2Types';

// Safe i18n wrapper — falls back to the key when the i18n plugin isn't
// installed (e.g., during unit tests without app.use(i18n)).
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ step: PlanStep | null }>();
const { t } = useSafeI18n();
const statusColor = computed(
  () =>
    ({
      pending: 'grey',
      running: 'blue',
      completed: 'green',
      failed: 'red',
      skipped: 'orange',
      partial_failure: 'red-7',
    })[props.step?.Status || 'pending'] || 'grey',
);
</script>
