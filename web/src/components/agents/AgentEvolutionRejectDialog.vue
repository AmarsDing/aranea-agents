<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">{{ $t('agentSettings.evolution.rejectDialogTitle') }}</div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <div v-if="suggestionTitle" class="text-body2">{{ suggestionTitle }}</div>
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
        </q-card-section>
      </div>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat no-caps :label="$t('agentSettings.evolution.cancel')" />
        <q-btn
          color="negative"
          unelevated
          no-caps
          :label="$t('agentSettings.evolution.rejectConfirm')"
          :loading="loading"
          @click="$emit('confirm')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
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
