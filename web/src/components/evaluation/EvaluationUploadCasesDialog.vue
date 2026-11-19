<template>
  <AppDialog
    :model-value="open"
    :title="datasetName ? $t('evaluationPage.uploadCases') + ' · ' + datasetName : $t('evaluationPage.uploadCases')"
    size="md"
    persistent
    :loading="loading"
    @update:model-value="$emit('update:open', $event)"
  >
    <div class="q-gutter-md">
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
    </div>
    <template #actions>
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
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import AppDialog from '../common/AppDialog.vue';

defineProps<{ open: boolean; text: string; loading: boolean; datasetName?: string }>();
defineEmits<{
  'update:open': [value: boolean];
  'update:text': [value: string];
  submit: [];
}>();
</script>
