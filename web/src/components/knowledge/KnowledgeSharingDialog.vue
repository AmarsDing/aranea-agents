<template>
  <AppDialog
    :model-value="open"
    :title="t('knowledgeWorkflow.sharing')"
    :subtitle="title"
    size="sm"
    :loading="saving"
    @update:model-value="$emit('update:open', $event)"
  >
    <div class="q-gutter-md">
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
    </div>
    <template #actions>
      <q-btn flat :label="t('common.cancel')" @click="$emit('update:open', false)" />
      <q-btn
        color="primary"
        :label="t('common.save')"
        :loading="saving"
        :disable="loading || !!error || (visibility === 'shared' && !sharedWith.length)"
        @click="$emit('save')"
      />
    </template>
  </AppDialog>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../../components/common/AppDialog.vue';
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
