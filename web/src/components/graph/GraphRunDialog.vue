<template>
  <AppDialog
    :model-value="open"
    :title="t('graphs.runDialogTitle')"
    :subtitle="graphName ? t('graphs.runDialogSubtitle', { name: graphName }) : ''"
    size="sm"
    persistent
    :loading="loading"
    @update:model-value="open = $event"
  >
    <div class="q-gutter-md">
      <q-input
        v-model="sessionId"
        class="app-field-md"
        dense
        outlined
        :label="t('graphs.runDialogSessionId')"
        :hint="t('graphs.runDialogSessionIdHint')"
      />
      <q-input
        v-model="initialState"
        class="app-field-long"
        dense
        outlined
        autogrow
        type="textarea"
        :label="t('graphs.runDialogInitialState')"
        :hint="t('graphs.runDialogInitialStateHint')"
      />
    </div>
    <template #actions>
      <q-btn flat rounded :label="t('graphs.runDialogCancel')" @click="open = false" />
      <q-btn
        color="primary"
        rounded
        unelevated
        :label="t('graphs.runDialogExecute')"
        :loading="loading"
        @click="$emit('submit')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';

const { t } = useI18n();

const open = defineModel<boolean>({ required: true });
const sessionId = defineModel<string>('sessionId', { required: true });
const initialState = defineModel<string>('initialState', { required: true });

defineProps<{
  graphName?: string;
  loading: boolean;
}>();

defineEmits<{
  submit: [];
}>();
</script>
