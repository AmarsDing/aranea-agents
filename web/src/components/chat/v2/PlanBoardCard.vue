<!-- web/src/components/chat/v2/PlanBoardCard.vue -->
<template>
  <div class="plan-board-card">
    <div class="plan-header">
      <span>{{ t('chat.v2.planBoardTitle') }}</span>
      <q-badge :color="statusColor">{{ planBoard.Status }}</q-badge>
    </div>
    <PlanDAG :steps="planBoard.Steps" />
    <PlanStepDetailPanel :step="selectedStep" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PlanBoard } from '../../../features/chat/v2Types';
import PlanDAG from './PlanDAG.vue';
import PlanStepDetailPanel from './PlanStepDetailPanel.vue';

// Safe i18n wrapper — falls back to the key when the i18n plugin isn't
// installed (e.g., during unit tests without app.use(i18n)).
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ planBoard: PlanBoard }>();
const { t } = useSafeI18n();
const selectedStepId = ref<string | null>(null);
const selectedStep = computed(() =>
  props.planBoard.Steps.find(s => s.ID === selectedStepId.value) || null
);
const statusColor = computed(() => ({
  planning: 'orange', executing: 'blue', completed: 'green',
  failed: 'red', partial_failure: 'orange-8',
}[props.planBoard.Status] || 'grey'));
</script>
