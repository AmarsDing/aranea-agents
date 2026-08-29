<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">{{ t('pluginsPage.runs.detailTitle') }}</div>
        <q-btn v-close-popup flat dense round icon="close" :aria-label="t('pluginsPage.detail.close')" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body q-pt-none">
        <pre class="plugin-run-detail app-code-block">{{ text }}</pre>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat rounded no-caps :label="t('common.close')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import JsonCodeViewer from '../common/JsonCodeViewer.vue';

defineProps<{
  open: boolean;
  text: string;
}>();

defineEmits<{
  'update:open': [value: boolean];
}>();

const { t } = useI18n();
</script>
