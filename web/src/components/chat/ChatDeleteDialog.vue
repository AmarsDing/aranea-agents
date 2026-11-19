<template>
  <app-dialog
    :model-value="modelValue"
    :title="title"
    icon="delete_outline"
    size="sm"
    :loading="deleting"
    @update:model-value="$emit('update:modelValue', $event)"
  >
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

    <template #actions>
      <q-btn flat no-caps :label="t('chat.cancel')" :disable="deleting" @click="$emit('update:modelValue', false)" />
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
    </template>
  </app-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
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
