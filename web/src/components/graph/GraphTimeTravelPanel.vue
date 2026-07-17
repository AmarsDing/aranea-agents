<template>
  <div class="graph-time-travel-panel q-mt-md">
    <div class="graph-time-travel-panel__title q-mb-sm">{{ t('graphs.timeTravelTitle') }}</div>

    <div class="row q-col-gutter-sm items-end q-mb-md">
      <q-input
        :model-value="stepIndex"
        class="col"
        dense
        outlined
        type="number"
        :label="t('graphs.timeTravelStepIndex')"
        @update:model-value="$emit('update:stepIndex', Number($event))"
      />
      <q-btn
        color="primary"
        outline
        dense
        :label="t('graphs.timeTravelButton')"
        :loading="timeTravelLoading"
        @click="$emit('timeTravel')"
      />
    </div>

    <q-slider
      :model-value="stepIndex"
      :min="0"
      :max="maxStep"
      :step="1"
      label
      label-always
      color="primary"
      class="q-mb-md"
      @update:model-value="$emit('update:stepIndex', Number($event))"
    />

    <template v-if="selectedCheckpoint">
      <div class="text-caption app-text-secondary q-mb-xs">{{ t('graphs.timeTravelSelectedCheckpoint') }}</div>
      <div class="graph-time-travel-panel__mono q-mb-sm">{{ selectedCheckpoint.checkpointId }}</div>
    </template>

    <q-spinner v-if="snapshotLoading" color="primary" size="28px" />
    <template v-else-if="statePatchJson">
      <q-input
        :model-value="statePatchJson"
        dense
        outlined
        autogrow
        type="textarea"
        class="app-field-long"
        :label="t('graphs.timeTravelSnapshotLabel')"
        @update:model-value="$emit('update:statePatchJson', String($event ?? ''))"
      />
      <div class="row q-gutter-sm q-mt-sm">
        <q-btn
          color="primary"
          flat
          dense
          :label="t('graphs.timeTravelApplyEdit')"
          :loading="editLoading"
          @click="$emit('applyEdit')"
        />
      </div>
    </template>
    <div v-else class="text-caption app-text-secondary">{{ t('graphs.timeTravelEmptyHint') }}</div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { CheckpointInfo } from '../../features/graph/types';

defineProps<{
  selectedCheckpoint: CheckpointInfo | null;
  statePatchJson: string;
  snapshotLoading: boolean;
  editLoading: boolean;
  timeTravelLoading: boolean;
  stepIndex: number;
  maxStep?: number;
}>();

defineEmits<{
  'update:statePatchJson': [value: string];
  'update:stepIndex': [value: number];
  timeTravel: [];
  applyEdit: [];
}>();

const { t } = useI18n();
</script>
