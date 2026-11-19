<template>
  <AppDialog
    :model-value="open"
    :title="t('pluginsPage.runs.detailTitle')"
    size="sm"
    @update:model-value="$emit('update:open', $event)"
  >
    <JsonCodeViewer :text="text" />
    <template #actions>
      <q-btn flat rounded no-caps :label="t('common.close')" @click="$emit('update:open', false)" />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
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
