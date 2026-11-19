<template>
  <AppDialog
    :model-value="open"
    :title="$t('evaluationPage.addCaseTitle')"
    size="md"
    persistent
    :loading="submitting"
    @update:model-value="$emit('update:open', $event)"
  >
    <div class="q-gutter-md">
      <q-btn-toggle
          :model-value="mode"
          dense
          no-caps
          unelevated
          toggle-color="primary"
          :options="[
            { label: $t('evaluationPage.addCaseModeExisting'), value: 'existing', disable: !datasetOptions.length },
            { label: $t('evaluationPage.addCaseModeNew'), value: 'new' },
          ]"
          @update:model-value="$emit('update:mode', $event)"
        />
        <q-select
          v-if="mode === 'existing'"
          :model-value="datasetId"
          dense
          outlined
          emit-value
          map-options
          :options="datasetOptions"
          :loading="datasetsLoading"
          :label="$t('evaluationPage.addCaseDatasetExisting')"
          @update:model-value="$emit('update:datasetId', String($event ?? ''))"
        />
        <q-input
          v-else
          :model-value="newDatasetName"
          class="app-field-md"
          dense
          outlined
          :label="$t('evaluationPage.addCaseDatasetNew')"
          @update:model-value="$emit('update:newDatasetName', String($event ?? ''))"
        />
        <q-input
          :model-value="input"
          dense
          outlined
          type="textarea"
          rows="4"
          :label="$t('evaluationPage.addCaseInput')"
          @update:model-value="$emit('update:input', String($event ?? ''))"
        />
        <q-input
          :model-value="expectedOutput"
          dense
          outlined
          type="textarea"
          rows="6"
          :label="$t('evaluationPage.addCaseExpected')"
          :hint="$t('evaluationPage.addCaseExpectedHint')"
          @update:model-value="$emit('update:expectedOutput', String($event ?? ''))"
        />
        <q-input
          :model-value="rubric"
          dense
          outlined
          type="textarea"
          rows="3"
          :label="$t('evaluationPage.addCaseRubric')"
          :hint="$t('evaluationPage.addCaseRubricHint')"
          @update:model-value="$emit('update:rubric', String($event ?? ''))"
        />
    </div>
    <template #actions>
      <q-btn flat no-caps :label="$t('common.cancel')" @click="$emit('update:open', false)" />
      <q-btn
        color="primary"
        unelevated
        no-caps
        :label="$t('evaluationPage.addCaseSubmit')"
        :loading="submitting"
        :disable="!input?.trim()"
        @click="$emit('submit')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import AppDialog from '../../components/common/AppDialog.vue';

defineProps<{
  open: boolean;
  mode: 'existing' | 'new';
  datasetId: string;
  datasetOptions: { label: string; value: string }[];
  datasetsLoading: boolean;
  newDatasetName: string;
  input: string;
  expectedOutput: string;
  rubric: string;
  submitting: boolean;
}>();
defineEmits<{
  'update:open': [value: boolean];
  'update:mode': [value: 'existing' | 'new'];
  'update:datasetId': [value: string];
  'update:newDatasetName': [value: string];
  'update:input': [value: string];
  'update:expectedOutput': [value: string];
  'update:rubric': [value: string];
  submit: [];
}>();
</script>
