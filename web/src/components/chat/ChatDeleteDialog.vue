<template>
  <q-dialog :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="q-pa-md" style="min-width: 320px">
      <q-card-section class="text-h6 text-negative">{{ title }}</q-card-section>
      <q-banner v-if="blockedBusy" class="bg-negative text-white q-mb-md" rounded>
        {{ t("chat.deleteBlockedBusy") }}
      </q-banner>
      <q-card-section v-if="kind !== 'all' && kind !== 'session' && !blockedBusy" class="q-pt-none">
        <p class="text-cream-muted text-body2 q-mb-sm">
          {{ t("chat.deleteConfirmHint") }} <strong>{{ expectedName }}</strong>
        </p>
        <q-input
          :model-value="nameInput"
          dense
          outlined
          :disable="blockedBusy"
          @update:model-value="$emit('update:nameInput', String($event ?? ''))"
        />
        <p v-if="hasNameError" class="text-negative text-caption q-mt-sm">
          {{ t("chat.deleteNameMismatch") }}
        </p>
      </q-card-section>
      <q-card-section v-else-if="kind === 'session'" class="q-pt-none text-cream-muted text-body2">
        {{ t("chat.deleteSessionConfirm") }} <strong>{{ expectedName }}</strong>
      </q-card-section>
      <q-card-section v-else-if="kind === 'all'" class="q-pt-none text-cream-muted text-body2">
        {{ t("chat.deleteAllConfirm") }}
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat :label="t('chat.cancel')" :disable="deleting" v-close-popup />
        <q-btn
          v-if="!blockedBusy"
          unelevated
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
import { useI18n } from "vue-i18n";
import type { DeleteKind } from "./types";

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
  "update:modelValue": [value: boolean];
  "update:nameInput": [value: string];
  confirm: [];
}>();

const { t } = useI18n();
</script>
