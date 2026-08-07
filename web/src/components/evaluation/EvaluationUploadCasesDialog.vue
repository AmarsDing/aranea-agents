<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="text-h6">
        {{ $t('evaluationPage.uploadCases') }}{{ datasetName ? ` · ${datasetName}` : '' }}
      </q-card-section>
      <q-card-section class="app-dialog-body q-gutter-md q-pt-none">
        <q-input
          :model-value="text"
          dense
          outlined
          type="textarea"
          rows="10"
          :label="$t('evaluationPage.uploadJsonLabel')"
          :placeholder="$t('evaluationPage.uploadJsonPlaceholder')"
          @update:model-value="$emit('update:text', String($event ?? ''))"
        />
        <div class="text-caption text-grey-7">{{ $t('evaluationPage.uploadHint') }}</div>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps :label="$t('common.cancel')" @click="$emit('update:open', false)" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="$t('evaluationPage.uploadSubmit')"
          :loading="loading"
          :disable="!text?.trim()"
          @click="$emit('submit')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
defineProps<{ open: boolean; text: string; loading: boolean; datasetName?: string }>();
defineEmits<{
  'update:open': [value: boolean];
  'update:text': [value: string];
  submit: [];
}>();
</script>
