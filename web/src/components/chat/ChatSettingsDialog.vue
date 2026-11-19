<template>
  <app-dialog
    :model-value="modelValue"
    :title="title"
    size="sm"
    :loading="Boolean(saving)"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <div v-if="mode === 'agent'" class="q-gutter-y-sm">
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
    </div>
    <div v-else-if="mode === 'team'">
      <q-input
        :model-value="name"
        class="app-field-md"
        :label="t('chat.nameField')"
        outlined
        dense
        @update:model-value="$emit('update:name', String($event ?? ''))"
      />
    </div>
    <template #actions>
      <q-btn flat no-caps :label="t('chat.cancel')" :disable="saving" @click="$emit('update:modelValue', false)" />
      <q-btn unelevated no-caps color="accent" :label="t('chat.save')" :loading="saving" @click="$emit('save')" />
    </template>
  </app-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
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
