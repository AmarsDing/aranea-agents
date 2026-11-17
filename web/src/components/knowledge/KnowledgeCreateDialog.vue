<template>
  <AppDialog
    :model-value="open"
    :title="t('knowledgePage.createDialogTitle')"
    size="sm"
    persistent
    :loading="loading"
    @update:model-value="$emit('update:open', $event)"
  >
    <div class="q-gutter-md">
      <q-select
            :model-value="vaultBackend || 'team'"
            emit-value
            map-options
            outlined
            dense
            :label="t('knowledgeWorkflow.storageMode')"
            :options="[
              { label: t('knowledgeWorkflow.managedVault'), value: 'team' },
              { label: t('knowledgeWorkflow.localVault'), value: 'local' },
            ]"
            @update:model-value="$emit('update:vaultBackend', $event)"
          />
          <p v-if="vaultBackend !== 'local'" class="text-caption">{{ t('knowledgeWorkflow.managedHint') }}</p>
          <q-input
            :model-value="name"
            class="app-field-md"
            dense
            outlined
            required
            :label="t('knowledgePage.createNameLabel')"
            @update:model-value="$emit('update:name', String($event ?? ''))"
          />
          <q-input
            v-if="vaultBackend === 'local'"
            :model-value="rootPath"
            class="app-field-long"
            dense
            outlined
            required
            :label="t('knowledgePage.createRootPathLabel')"
            :hint="t('knowledgePage.createRootPathHint')"
            @update:model-value="$emit('update:rootPath', String($event ?? ''))"
          />
          <q-input
            :model-value="description"
            class="app-field-long"
            dense
            outlined
            :label="t('knowledgePage.createDescLabel')"
            @update:model-value="$emit('update:description', String($event ?? ''))"
          />
          <q-expansion-item
            dense
            switch-toggle-side
            :label="t('knowledgePage.createAdvanced')"
            :caption="t('knowledgePage.createEmbeddingHint')"
          >
            <q-input
              :model-value="embeddingModel"
              class="app-field-md q-mt-sm"
              dense
              outlined
              :label="t('knowledgePage.createEmbeddingLabel')"
              :hint="t('knowledgePage.createEmbeddingHint')"
              @update:model-value="$emit('update:embeddingModel', String($event ?? ''))"
            />
          </q-expansion-item>
    </div>
    <template #actions>
      <q-btn flat no-caps :label="t('common.cancel')" @click="$emit('update:open', false)" />
      <q-btn
        color="primary"
        unelevated
        no-caps
        :label="t('common.create')"
        :loading="loading"
        @click="$emit('submit')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../../components/common/AppDialog.vue';

defineProps<{
  open: boolean;
  name: string;
  rootPath: string;
  vaultBackend?: 'team' | 'local';
  description: string;
  embeddingModel: string;
  loading: boolean;
}>();
defineEmits<{
  'update:open': [value: boolean];
  'update:name': [value: string];
  'update:rootPath': [value: string];
  'update:vaultBackend': [value: 'team' | 'local'];
  'update:description': [value: string];
  'update:embeddingModel': [value: string];
  submit: [];
}>();

const { t } = useI18n();
</script>
