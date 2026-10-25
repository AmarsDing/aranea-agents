<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head">
        <div class="app-glass-dialog__title">{{ t('knowledgeWorkflow.sharing') }}</div>
        <div class="text-caption">{{ title }}</div>
      </q-card-section>
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <q-linear-progress v-if="loading" indeterminate />
          <q-banner v-if="error" class="app-banner-warning">{{ error }}</q-banner>
          <q-select
            :model-value="visibility"
            :disable="loading"
            emit-value
            map-options
            outlined
            dense
            :label="t('knowledgeWorkflow.visibility')"
            :options="[
              { label: t('knowledgeWorkflow.private'), value: 'private' },
              { label: t('knowledgeWorkflow.collection'), value: 'collection' },
              { label: t('knowledgeWorkflow.shared'), value: 'shared' },
            ]"
            @update:model-value="$emit('update:visibility', $event)"
          />
          <template v-if="visibility === 'shared'">
            <p class="text-caption">{{ t('knowledgeWorkflow.agentShareHint') }}</p>
            <q-select
              :model-value="sharedWith"
              :options="options"
              multiple
              use-chips
              emit-value
              map-options
              outlined
              dense
              :disable="loading || !!error"
              :label="t('knowledgeWorkflow.shareTargets')"
              @update:model-value="$emit('update:sharedWith', $event)"
            />
          </template>
        </q-card-section>
      </div>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn v-close-popup flat :label="t('common.cancel')" />
        <q-btn
          color="primary"
          :label="t('common.save')"
          :loading="saving"
          :disable="loading || !!error || (visibility === 'shared' && !sharedWith.length)"
          @click="$emit('save')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n';
defineProps<{
  open: boolean;
  loading: boolean;
  saving: boolean;
  error: string;
  title: string;
  visibility: 'collection' | 'private' | 'shared';
  sharedWith: string[];
  options: Array<{ label: string; value: string }>;
}>();
defineEmits<{
  'update:open': [value: boolean];
  'update:visibility': [value: 'collection' | 'private' | 'shared'];
  'update:sharedWith': [value: string[]];
  save: [];
}>();
const { t } = useI18n();
</script>
