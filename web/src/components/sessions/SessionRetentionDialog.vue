<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="sessions-dialog-card app-dialog-card app-dialog-card--sm">
      <q-card-section>
        <div class="text-h6 text-weight-bold">{{ title }}</div>
        <div class="text-caption q-mt-xs" style="color: var(--color-text-secondary)">{{ subtitle }}</div>
        <q-input
          v-model.number="days"
          type="number"
          dense
          outlined
          class="q-mt-md sessions-field"
          :label="t('sessionsPage.retention.keepDaysLabel')"
          :min="1"
        />
        <q-checkbox
          v-if="mode === 'delete'"
          v-model="includeArchived"
          dense
          class="q-mt-sm"
          :label="t('sessionsPage.retention.includeArchived')"
        />
        <div v-if="preview" class="q-mt-md q-pa-sm app-glass-inset-preview">
          <div>
            {{ t('sessionsPage.retention.willAction', { action: actionVerb }) }}
            <strong>{{ preview.matched }}</strong>
            {{ t('sessionsPage.retention.sessionsCountSuffix') }}
          </div>
          <div class="text-caption q-mt-xs">
            {{ t('sessionsPage.retention.keepDays', { days }) }}
            <span v-if="preview.skipped_running">
              {{ t('sessionsPage.retention.skippedRunning', { count: preview.skipped_running }) }}
            </span>
            <span v-if="preview.skipped_not_found">
              {{ t('sessionsPage.retention.skippedNotFound', { count: preview.skipped_not_found }) }}
            </span>
            <span v-if="preview.truncated">{{ t('sessionsPage.retention.truncated') }}</span>
          </div>
        </div>
        <div v-else-if="previewLoading" class="q-mt-md row items-center q-gutter-sm">
          <q-spinner size="20px" color="primary" />
          <span class="text-caption">{{ t('sessionsPage.retention.previewing') }}</span>
        </div>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar q-pa-md q-pt-none">
        <div v-if="!preview || preview.matched === 0" class="text-caption text-grey-6 q-mr-auto">
          {{ t('sessionsPage.retention.previewHint', { action: actionVerb }) }}
        </div>
        <q-btn
          flat
          rounded
          :label="t('sessionsPage.retention.cancel')"
          class="sessions-btn-ghost"
          @click="$emit('update:modelValue', false)"
        />
        <q-btn
          flat
          rounded
          :label="t('sessionsPage.retention.preview')"
          class="sessions-btn-ghost"
          :loading="previewLoading"
          @click="$emit('preview', { days: Math.max(1, Number(days) || 30), includeArchived })"
        />
        <q-btn
          unelevated
          rounded
          :color="mode === 'delete' ? 'negative' : 'primary'"
          :label="confirmLabel"
          :disable="!preview || preview.matched === 0"
          :loading="loading"
          @click="$emit('confirm', { days: Math.max(1, Number(days) || 30), includeArchived })"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { BatchPreviewResult, RetentionDialogMode } from '../../features/session/types';

const { t } = useI18n();

const props = defineProps<{
  modelValue: boolean;
  mode: RetentionDialogMode;
  preview: BatchPreviewResult | null;
  previewLoading?: boolean;
  loading?: boolean;
}>();

defineEmits<{
  'update:modelValue': [v: boolean];
  preview: [payload: { days: number; includeArchived: boolean }];
  confirm: [payload: { days: number; includeArchived: boolean }];
}>();

const days = ref(30);
const includeArchived = ref(false);

const title = computed(() =>
  props.mode === 'archive' ? t('sessionsPage.retention.titleArchive') : t('sessionsPage.retention.titleDelete'),
);
const subtitle = computed(() =>
  props.mode === 'archive' ? t('sessionsPage.retention.subtitleArchive') : t('sessionsPage.retention.subtitleDelete'),
);
const actionVerb = computed(() =>
  props.mode === 'archive' ? t('sessionsPage.retention.actionArchive') : t('sessionsPage.retention.actionDelete'),
);
const confirmLabel = computed(() =>
  props.mode === 'archive' ? t('sessionsPage.retention.confirmArchive') : t('sessionsPage.retention.confirmDelete'),
);

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      days.value = 30;
      includeArchived.value = false;
    }
  },
);
</script>
