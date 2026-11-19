<template>
  <app-dialog
    :model-value="open"
    :title="$t('agentSettings.evolution.rejectDialogTitle')"
    :subtitle="suggestionTitle"
    icon="block"
    size="sm"
    persistent
    :loading="loading"
    @update:model-value="$emit('update:open', $event)"
  >
    <q-input
      :model-value="reason"
      dense
      outlined
      type="textarea"
      autogrow
      :label="$t('agentSettings.evolution.rejectReasonLabel')"
      :placeholder="$t('agentSettings.evolution.rejectReasonPlaceholder')"
      @update:model-value="(v: string | number | null) => $emit('update:reason', String(v ?? ''))"
    />
    <template #actions>
      <q-btn flat no-caps :label="$t('agentSettings.evolution.cancel')" @click="$emit('update:open', false)" />
      <q-btn
        color="negative"
        unelevated
        no-caps
        :label="$t('agentSettings.evolution.rejectConfirm')"
        :loading="loading"
        @click="$emit('confirm')"
      />
    </template>
  </app-dialog>
</template>

<script setup lang="ts">
import AppDialog from '../common/AppDialog.vue';

defineProps<{
  open: boolean;
  suggestionTitle: string;
  reason: string;
  loading: boolean;
}>();

defineEmits<{
  'update:open': [value: boolean];
  'update:reason': [value: string];
  confirm: [];
}>();
</script>
