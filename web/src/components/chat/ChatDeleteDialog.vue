<template>
  <q-dialog :model-value="modelValue" :persistent="deleting" @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="text-h6 text-negative">{{ title }}</q-card-section>
      <q-card-section class="app-dialog-body q-pt-none">
        <q-banner v-if="blockedBusy" class="bg-negative text-white q-mb-md" rounded>
          {{ t('chat.deleteBlockedBusy') }}
        </q-banner>
        <template v-if="kind !== 'all' && kind !== 'session' && !blockedBusy">
          <p class="text-body2 q-mb-sm app-text-secondary">
            {{ t('chat.deleteConfirmHint') }} <strong>{{ expectedName }}</strong>
          </p>
          <q-input
            :model-value="nameInput"
            class="app-field-md"
            dense
            outlined
            :disable="blockedBusy"
            @update:model-value="$emit('update:nameInput', String($event ?? ''))"
          />
          <p v-if="hasNameError" class="text-negative text-caption q-mt-sm">
            {{ t('chat.deleteNameMismatch') }}
          </p>
        </template>
        <p v-else-if="kind === 'session'" class="text-body2 q-mb-none app-text-secondary">
          {{ t('chat.deleteSessionConfirm') }} <strong>{{ expectedName }}</strong>
        </p>
        <p v-else-if="kind === 'all'" class="text-body2 q-mb-none app-text-secondary">
          {{ t('chat.deleteAllConfirm') }}
        </p>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn v-close-popup flat no-caps :label="t('chat.cancel')" :disable="deleting" />
        <q-btn
          v-if="!blockedBusy"
          unelevated
          no-caps
          color="negative"
          :label="t('chat.confirmDelete')"
          :disable="!canConfirm"
          :loading="deleting"
          @click="$emit('confirm')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { DeleteKind } from './types';

defineProps<{
  modelValue: boolean;
  title: string;
  kind: DeleteKind;
  expectedName: string;
  nameInput: string;
  blockedBusy: boolean;
  canConfirm: boolean;
  hasNameError: boolean;
  deleting?: boolean;
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
  'update:nameInput': [value: string];
  confirm: [];
}>();

const { t } = useI18n();
</script>
