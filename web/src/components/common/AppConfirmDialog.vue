<template>
  <AppDialog
    :model-value="modelValue"
    :title="title"
    size="sm"
    :loading="loading"
    persistent
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="app-confirm-dialog__body">
      <div class="app-confirm-dialog__icon" :class="`app-confirm-dialog__icon--${tone}`" aria-hidden="true">
        <q-icon :name="resolvedIcon" size="24px" />
      </div>
      <p v-if="message" class="app-confirm-dialog__message">{{ message }}</p>
    </div>
    <template #actions>
      <q-btn
        flat
        no-caps
        :label="cancelLabel || t('shell.common.cancel')"
        :disable="loading"
        @click="emit('update:modelValue', false)"
      />
      <q-btn
        unelevated
        no-caps
        :color="tone === 'danger' ? 'negative' : 'primary'"
        :label="confirmLabel || t('shell.common.confirm')"
        :loading="loading"
        @click="emit('confirm')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from './AppDialog.vue';

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    title: string;
    message?: string;
    confirmLabel?: string;
    cancelLabel?: string;
    tone?: 'danger' | 'primary';
    loading?: boolean;
    icon?: string;
  }>(),
  { message: '', confirmLabel: '', cancelLabel: '', tone: 'danger', loading: false, icon: '' },
);

const emit = defineEmits<{ 'update:modelValue': [value: boolean]; confirm: [] }>();
const { t } = useI18n();
const resolvedIcon = computed(() => props.icon || (props.tone === 'danger' ? 'warning_amber' : 'help_outline'));
</script>
