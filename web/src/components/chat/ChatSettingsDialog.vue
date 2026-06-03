<template>
  <q-dialog :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm">
      <q-card-section class="text-h6">{{ title }}</q-card-section>
      <q-card-section v-if="mode === 'agent'" class="app-dialog-body q-gutter-y-sm q-pt-none">
        <q-input
          :model-value="name"
          class="app-field-md"
          :label="t('chat.nameField')"
          outlined
          dense
          @update:model-value="$emit('update:name', String($event ?? ''))"
        />
        <q-input :model-value="agentKey" class="app-field-md" :label="t('chat.keyField')" outlined dense readonly />
        <q-input
          :model-value="provider"
          class="app-field-sm"
          :label="t('chat.providerField')"
          outlined
          dense
          @update:model-value="$emit('update:provider', String($event ?? ''))"
        />
        <q-input
          :model-value="model"
          class="app-field-md"
          :label="t('chat.modelField')"
          outlined
          dense
          @update:model-value="$emit('update:model', String($event ?? ''))"
        />
      </q-card-section>
      <q-card-section v-else-if="mode === 'team'" class="app-dialog-body q-pt-none">
        <q-input
          :model-value="name"
          class="app-field-md"
          :label="t('chat.nameField')"
          outlined
          dense
          @update:model-value="$emit('update:name', String($event ?? ''))"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn v-close-popup flat no-caps :label="t('chat.cancel')" :disable="saving" />
        <q-btn unelevated no-caps color="primary" :label="t('chat.save')" :loading="saving" @click="$emit('save')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { ChatEntityKind } from './types';

defineProps<{
  modelValue: boolean;
  title: string;
  mode: ChatEntityKind | null;
  name: string;
  agentKey: string;
  provider: string;
  model: string;
  saving?: boolean;
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
  'update:name': [value: string];
  'update:provider': [value: string];
  'update:model': [value: string];
  save: [];
}>();

const { t } = useI18n();
</script>
