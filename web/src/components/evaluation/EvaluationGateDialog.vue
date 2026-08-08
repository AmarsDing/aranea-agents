<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="text-h6">{{ $t('evaluationPage.gateTitle') }}</q-card-section>
      <q-card-section class="app-dialog-body q-gutter-md q-pt-none">
        <div class="text-caption text-grey-7">{{ $t('evaluationPage.gateHint') }}</div>
        <q-toggle
          :model-value="enabled"
          :label="$t('evaluationPage.gateEnabled')"
          @update:model-value="$emit('update:enabled', Boolean($event))"
        />
        <q-select
          :model-value="agentId"
          class="app-field-md"
          dense
          outlined
          emit-value
          map-options
          label="Agent"
          :options="agentOptions"
          :disable="!enabled"
          @update:model-value="$emit('update:agentId', String($event ?? ''))"
        />
        <q-select
          :model-value="datasetId"
          class="app-field-md"
          dense
          outlined
          emit-value
          map-options
          :label="$t('evaluationPage.gateDataset')"
          :options="datasetOptions"
          :disable="!enabled"
          @update:model-value="$emit('update:datasetId', String($event ?? ''))"
        />
        <q-select
          :model-value="metric"
          class="app-field-md"
          dense
          outlined
          emit-value
          map-options
          :label="$t('evaluationPage.gateMetric')"
          :options="metricOptions"
          :disable="!enabled"
          @update:model-value="$emit('update:metric', String($event ?? ''))"
        />
        <q-input
          :model-value="minScore"
          class="app-field-sm"
          dense
          outlined
          type="number"
          min="0"
          max="1"
          step="0.05"
          :label="$t('evaluationPage.gateMinScore')"
          :hint="$t('evaluationPage.gateMinScoreHint')"
          :disable="!enabled"
          @update:model-value="$emit('update:minScore', Number($event) || 0)"
        />
        <q-input
          :model-value="maxDrop"
          class="app-field-sm"
          dense
          outlined
          type="number"
          min="0"
          max="1"
          step="0.05"
          :label="$t('evaluationPage.gateMaxDrop')"
          :hint="$t('evaluationPage.gateMaxDropHint')"
          :disable="!enabled"
          @update:model-value="$emit('update:maxDrop', Number($event) || 0)"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps :label="$t('common.cancel')" @click="$emit('update:open', false)" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="$t('common.save')"
          :loading="saving"
          @click="$emit('submit')"
        />
      </q-card-actions>
      <q-inner-loading :showing="loading" />
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

defineProps<{
  open: boolean;
  enabled: boolean;
  agentId: string;
  datasetId: string;
  metric: string;
  minScore: number;
  maxDrop: number;
  loading: boolean;
  saving: boolean;
  agentOptions: { label: string; value: string }[];
  datasetOptions: { label: string; value: string }[];
}>();

defineEmits<{
  'update:open': [value: boolean];
  'update:enabled': [value: boolean];
  'update:agentId': [value: string];
  'update:datasetId': [value: string];
  'update:metric': [value: string];
  'update:minScore': [value: number];
  'update:maxDrop': [value: number];
  submit: [];
}>();

const { t } = useI18n();

const metricOptions = computed(() => [
  { label: 'exact_match', value: 'exact_match' },
  { label: 'contains_match', value: 'contains_match' },
  { label: 'llm_as_judge', value: 'llm_as_judge' },
  { label: 'tool_call_accuracy', value: 'tool_call_accuracy' },
  { label: t('evaluationPage.gateMetricRedteam'), value: 'redteam_attack_success_rate' },
]);
</script>
