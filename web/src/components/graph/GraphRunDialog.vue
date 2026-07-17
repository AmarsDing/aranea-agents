<template>
  <q-dialog v-model="open" persistent>
    <q-card class="graph-run-dialog app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head">
        <div class="app-glass-dialog__title">{{ t('graphs.runDialogTitle') }}</div>
        <div v-if="graphName" class="app-glass-dialog__subtitle">
          {{ t('graphs.runDialogSubtitle', { name: graphName }) }}
        </div>
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
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
      </q-card-section>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn flat rounded :label="t('graphs.runDialogCancel')" @click="open = false" />
        <q-btn
          color="primary"
          rounded
          unelevated
          :label="t('graphs.runDialogExecute')"
          :loading="loading"
          @click="$emit('submit')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

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
