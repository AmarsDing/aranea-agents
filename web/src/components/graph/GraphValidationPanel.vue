<template>
  <div v-if="issues.length" class="graph-validation-panel q-mt-md">
    <div class="graph-validation-panel__title row items-center q-gutter-xs">
      <q-icon :name="valid ? 'warning' : 'error'" :color="valid ? 'warning' : 'negative'" size="16px" />
      <span>{{ valid ? t('graphs.validationWarnings') : t('graphs.validationErrors') }}</span>
    </div>
    <q-list dense bordered separator class="rounded-borders q-mt-sm">
      <q-item
        v-for="(issue, idx) in issues"
        :key="`${issue.code}-${issue.nodeId}-${idx}`"
        clickable
        @click="$emit('selectNode', issue.nodeId || null)"
      >
        <q-item-section>
          <q-item-label>{{ issue.message || issue.code }}</q-item-label>
          <q-item-label v-if="issue.nodeId" caption>{{ issue.nodeId }} · {{ issue.field }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <q-badge :color="issue.warning ? 'warning' : 'negative'" rounded>
            {{ issue.warning ? t('graphs.validationWarn') : t('graphs.validationErr') }}
          </q-badge>
        </q-item-section>
      </q-item>
    </q-list>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { ValidationError, ValidationWarning } from '../../features/graph/types';

type ValidationIssue = (ValidationError & { warning?: false }) | (ValidationWarning & { warning: true });

defineProps<{
  issues: ValidationIssue[];
  valid: boolean;
}>();

defineEmits<{
  selectNode: [nodeId: string | null];
}>();

const { t } = useI18n();
</script>
